/**
 * @file mpvwidget.cpp
 * @brief Реализация виджета видеоплеера на базе libmpv с OpenGL рендерингом
 */

#include "mpvwidget.h"

#ifndef NO_OPENGL
#include <QOpenGLContext>
#include <QOpenGLFramebufferObject>
#endif
#include <QWindow>
#include <QGuiApplication>
#include <QMetaObject>

// Таймер для обработки событий mpv
static const int MPV_EVENT_TIMER_MS = 30;

// Задержка debounce для перемотки (мс)
static const int SEEK_DEBOUNCE_MS = 300;

#ifdef HAS_MPV_RENDER
void* MpvWidget::mpvGetProcAddress(void *ctx, const char *name)
{
#ifndef NO_OPENGL
    QOpenGLContext *glctx = QOpenGLContext::currentContext();
    if (!glctx) return nullptr;
    return reinterpret_cast<void *>(glctx->getProcAddress(name));
#else
    return nullptr;
#endif
}
#endif // HAS_MPV_RENDER

#ifndef NO_OPENGL
MpvWidget::MpvWidget(QWidget *parent)
    : QOpenGLWidget(parent)
#else
MpvWidget::MpvWidget(QWidget *parent)
    : QWidget(parent)
#endif
{
    // Устанавливаем политику фокуса
    setFocusPolicy(Qt::StrongFocus);

    // Создаём debounce таймер для перемотки
    m_seekDebounceTimer = new QTimer(this);
    m_seekDebounceTimer->setSingleShot(true);
    m_seekDebounceTimer->setInterval(SEEK_DEBOUNCE_MS);
    connect(m_seekDebounceTimer, &QTimer::timeout, this, &MpvWidget::onSeekDebounceTimeout);
}

MpvWidget::~MpvWidget()
{
#ifdef HAS_MPV
    // Set destroying flag immediately to protect against concurrent render callback
    m_destroying.storeRelaxed(1);

    // Step 1: Deregister the render callback (prevents any new callbacks)
    if (m_mpvGL) {
        mpv_render_context_set_update_callback(m_mpvGL, nullptr, nullptr);
    }

    // Step 2: Stop timers
    if (m_seekDebounceTimer) {
        m_seekDebounceTimer->stop();
    }
    if (m_eventTimer) {
        m_eventTimer->stop();
    }

    // Step 3: Free mpv resources under mutex
    {
        QMutexLocker locker(&m_mutex);
        if (m_mpvGL) {
            mpv_render_context_free(m_mpvGL);
            m_mpvGL = nullptr;
        }
        if (m_mpv) {
            mpv_terminate_destroy(m_mpv);
            m_mpv = nullptr;
        }
        m_initialized.storeRelaxed(0);
    }
#else
    m_destroying.storeRelaxed(1);
    if (m_seekDebounceTimer) {
        m_seekDebounceTimer->stop();
    }
    if (m_eventTimer) {
        m_eventTimer->stop();
    }
#endif
}

// All GL functions must be defined when QOpenGLWidget is the base class
#ifndef NO_OPENGL
void MpvWidget::initializeGL()
{
#ifdef HAS_MPV_RENDER
    QOpenGLWidget::initializeGL();

    makeCurrent();

    // Create render context if mpv is initialized
    if (m_mpv && !m_mpvGL) {
        mpv_opengl_init_params gl_init_params = {
            mpvGetProcAddress,
            nullptr
        };

        char apiType[] = MPV_RENDER_API_TYPE_OPENGL;
        mpv_render_param params[] = {
            {MPV_RENDER_PARAM_API_TYPE, apiType},
            {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init_params},
            {MPV_RENDER_PARAM_INVALID, nullptr}
        };

        int err = mpv_render_context_create(&m_mpvGL, m_mpv, params);
        if (err < 0) {
            qWarning() << "Failed to create mpv render context:" << mpv_error_string(err);
        } else {
            qDebug() << "MpvWidget: OpenGL render context initialized";
        }
    }

    doneCurrent();
#endif // HAS_MPV_RENDER
}

void MpvWidget::resizeGL(int w, int h)
{
#ifdef HAS_MPV_RENDER
    Q_UNUSED(w);
    Q_UNUSED(h);
    // mpv handles scaling automatically via OPENGL_FBO in paintGL
    // No explicit resize needed with newer mpv versions
#endif // HAS_MPV_RENDER
}

void MpvWidget::paintGL()
{
#ifdef HAS_MPV_RENDER
    QOpenGLWidget::paintGL();

    // Render video frame if mpv is initialized
    if (m_mpv && m_mpvGL && !m_destroying.loadRelaxed()) {
        makeCurrent();

        int flip = 1;
        mpv_render_param params[] = {
            {MPV_RENDER_PARAM_FLIP_Y, &flip},
            {MPV_RENDER_PARAM_INVALID, nullptr}
        };

        mpv_render_context_render(m_mpvGL, params);

        doneCurrent();
    }
#endif // HAS_MPV_RENDER
}
#endif // NO_OPENGL

#ifdef HAS_MPV
bool MpvWidget::initializeMpv()
{
    // Check if already initialized
    if (m_initialized.loadRelaxed()) {
        return m_mpv != nullptr;
    }
    
    // Check if destroying
    if (m_destroying.loadRelaxed()) {
        return false;
    }

    QMutexLocker locker(&m_mutex);

    // Double-check under lock
    if (m_initialized.loadRelaxed() || m_mpv != nullptr) {
        locker.unlock();
        return m_mpv != nullptr;
    }

    // Создаём экземпляр mpv
    m_mpv = mpv_create();
    if (!m_mpv) {
        MpvEventData event;
        event.type = MpvEventData::Error;
        event.message = tr("Не удалось создать mpv экземпляр");
        m_eventBuffer.append(event);
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
        return false;
    }

    // Настройки mpv
    auto setOpt = [](mpv_handle *mpv, const char *name, const char *val) {
        int err = mpv_set_option_string(mpv, name, val);
        if (err < 0)
            qWarning() << "mpv option failed:" << name << mpv_error_string(err);
    };
    setOpt(m_mpv, "vo", "libmpv");
    setOpt(m_mpv, "hwdec", "auto");
    setOpt(m_mpv, "cache", "yes");
    setOpt(m_mpv, "cache-secs", "30");
    setOpt(m_mpv, "demuxer-max-bytes", "150M");
    setOpt(m_mpv, "demuxer-max-back-bytes", "50M");
    setOpt(m_mpv, "ao", "auto");

    int err = mpv_initialize(m_mpv);
    if (err < 0) {
        MpvEventData event;
        event.type = MpvEventData::Error;
        event.message = tr("Ошибка инициализации mpv: %1").arg(mpv_error_string(err));
        m_eventBuffer.append(event);
        mpv_terminate_destroy(m_mpv);
        m_mpv = nullptr;
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
        return false;
    }

    // Подписка на изменение свойств mpv. Без неё mpv не генерирует
    // MPV_EVENT_PROPERTY_CHANGE, поэтому processMpvEvent() никогда не получает
    // события: m_position и m_duration остаются 0, сигналы positionChanged и
    // durationChanged не эмитятся, и перемотка в UI не работает.
    // Список свойств совпадает с ветками MPV_EVENT_PROPERTY_CHANGE в
    // processMpvEvent(); наблюдение свойства без обработчика было бы no-op.
    auto observe = [](mpv_handle *handle, const char *name, mpv_format format) {
        int rc = mpv_observe_property(handle, 0, name, format);
        if (rc < 0)
            qWarning() << "mpv observe failed:" << name << mpv_error_string(rc);
    };
    observe(m_mpv, "time-pos", MPV_FORMAT_DOUBLE);
    observe(m_mpv, "duration", MPV_FORMAT_DOUBLE);
    observe(m_mpv, "pause", MPV_FORMAT_FLAG);
    observe(m_mpv, "eof-reached", MPV_FORMAT_FLAG);

    if (!m_eventTimer) {
        m_eventTimer = new QTimer(this);
        connect(m_eventTimer, &QTimer::timeout, this, &MpvWidget::onMpvEvents);
    }
    m_eventTimer->start(MPV_EVENT_TIMER_MS);

    // Mark as initialized atomically before releasing lock
    m_initialized.storeRelaxed(1);

    MpvEventData event;
    event.type = MpvEventData::Ready;
    m_eventBuffer.append(event);
    QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);

    qDebug() << "MpvWidget: mpv успешно инициализирован";
    return true;
}
#else // !HAS_MPV
bool MpvWidget::initializeMpv()
{
    qDebug() << "MpvWidget: mpv не поддерживается (собрано без HAS_MPV)";
    return false;
}
#endif // HAS_MPV

void MpvWidget::play(const QString &url)
{
#ifdef HAS_MPV
    // Check initialization flag first without lock to avoid locking every call
    if (!m_initialized.loadRelaxed()) {
        // Need initialization - use mutex to protect the initialization
        QMutexLocker locker(&m_mutex);
        if (!m_mpv && !m_initialized.loadRelaxed()) {
            locker.unlock();
            // Initialize without holding the lock
            initializeMpv();
            locker.relock();
        }
    }

    if (!m_mpv) {
        return; // Initialization failed
    }

    const QByteArray urlData = url.toUtf8();
    const char *args[] = {"loadfile", urlData.constData(), "replace", nullptr};

    int err = mpv_command_async(m_mpv, 0, args);
    if (err < 0) {
        MpvEventData event;
        event.type = MpvEventData::Error;
        event.message = tr("Ошибка загрузки файла: %1").arg(mpv_error_string(err));
        m_eventBuffer.append(event);
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
        return;
    }

    m_paused = false;
    qDebug() << "MpvWidget: воспроизведение" << url;
#else
    Q_UNUSED(url);
    qDebug() << "MpvWidget: воспроизведение невозможно (собрано без HAS_MPV)";
#endif // HAS_MPV
}

void MpvWidget::pause()
{
#ifdef HAS_MPV
    if (!m_mpv) return;

    QMutexLocker locker(&m_mutex);

    int value = 1;
    mpv_set_property_async(m_mpv, 0, "pause", MPV_FORMAT_FLAG, &value);
    m_paused = true;

    qDebug() << "MpvWidget: пауза";
#else
    qDebug() << "MpvWidget: пауза невозможна (собрано без HAS_MPV)";
#endif // HAS_MPV
}

void MpvWidget::resume()
{
#ifdef HAS_MPV
    if (!m_mpv) return;

    QMutexLocker locker(&m_mutex);

    int value = 0;
    mpv_set_property_async(m_mpv, 0, "pause", MPV_FORMAT_FLAG, &value);
    m_paused = false;

    qDebug() << "MpvWidget: возобновление";
#else
    qDebug() << "MpvWidget: возобновление невозможно (собрано без HAS_MPV)";
#endif // HAS_MPV
}

void MpvWidget::seek(double position)
{
#ifdef HAS_MPV
    if (!m_mpv) return;

    // Ограничиваем позицию допустимыми значениями
    if (position < 0) position = 0;

    QMutexLocker locker(&m_mutex);

    if (m_duration > 0 && position > m_duration) {
        position = m_duration;
    }

    // Сохраняем позицию для отложенного выполнения
    m_pendingSeekPosition = position;
    // Команда seek уйдёт в mpv через SEEK_DEBOUNCE_MS, а файл к тому моменту
    // может быть ещё не открыт (loadfile асинхронный, а поток из торрента
    // открывается медленно). Помечаем позицию как неприменённую, чтобы
    // MPV_EVENT_FILE_LOADED повторил её.
    m_seekApplied = false;

    // Сбрасываем предыдущий таймер и запускаем новый (debounce)
    // Это предотвращает утечку памяти при быстрой перемотке
    m_seekDebounceTimer->stop();
    m_seekDebounceTimer->start();

    qDebug() << "MpvWidget: перемотка запрошена на" << position << "(debounce " << SEEK_DEBOUNCE_MS << "мс)";
#else
    Q_UNUSED(position);
    qDebug() << "MpvWidget: перемотка невозможна (собрано без HAS_MPV)";
#endif // HAS_MPV
}

void MpvWidget::onSeekDebounceTimeout()
{
#ifdef HAS_MPV
    if (!m_mpv) return;

    QMutexLocker locker(&m_mutex);

    double position = m_pendingSeekPosition;

    QByteArray posStr = QByteArray::number(position, 'f', 2);
    const char *args[] = {"seek", posStr.constData(), "absolute", nullptr};

    int err = mpv_command_async(m_mpv, 0, args);
    if (err < 0) {
        MpvEventData event;
        event.type = MpvEventData::Error;
        event.message = tr("Ошибка перемотки: %1").arg(mpv_error_string(err));
        m_eventBuffer.append(event);
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
        return;
    }
    // Команда отправлена. Если файл ещё не загружен, mpv её проигнорирует —
    // тогда повторим по MPV_EVENT_FILE_LOADED.
    m_seekApplied = true;

    qDebug() << "MpvWidget: перемотка выполнена на" << position;
#else
    qDebug() << "MpvWidget: перемотка невозможна (собрано без HAS_MPV)";
#endif // HAS_MPV
}

double MpvWidget::position() const
{
#ifdef HAS_MPV
    if (!m_mpv) return 0.0;

    // Используем кэшированное значение под защитой мьютекса
    // для избежания гонки данных с потоком событий mpv
    QMutexLocker locker(&m_mutex);
    return m_position;
#else
    return 0.0;
#endif // HAS_MPV
}

double MpvWidget::duration() const
{
#ifdef HAS_MPV
    if (!m_mpv) return 0.0;

    // Используем кэшированное значение под защитой мьютекса
    // для избежания гонки данных с потоком событий mpv
    QMutexLocker locker(&m_mutex);
    return m_duration;
#else
    return 0.0;
#endif // HAS_MPV
}

bool MpvWidget::isPaused() const
{
#ifdef HAS_MPV
    if (!m_mpv) return true;

    QMutexLocker locker(&m_mutex);

    int paused = 0;
    mpv_get_property(m_mpv, "pause", MPV_FORMAT_FLAG, &paused);

    return paused != 0;
#else
    return true;
#endif // HAS_MPV
}

bool MpvWidget::event(QEvent *event)
{
#ifdef HAS_MPV
    // Обрабатываем события mpv
    if (event->type() == QEvent::User) {
        onMpvEvents();
        return true;
    }
#endif

#ifndef NO_OPENGL
    return QOpenGLWidget::event(event);
#else
    return QWidget::event(event);
#endif
}

void MpvWidget::showEvent(QShowEvent *event)
{
#ifndef NO_OPENGL
    QOpenGLWidget::showEvent(event);
#else
    QWidget::showEvent(event);
#endif

#ifdef HAS_MPV
    if (!m_initialized.loadRelaxed()) {
        // Захватываем QPointer, а не сырой this. QTimer::singleShot не
        // отменяется при уничтожении объекта: одноразовый таймер негде хранить,
        // поэтому лямбда с `this` срабатывала уже после удаления виджета и
        // читала m_destroying в освобождённой памяти. QPointer сам обнуляется
        // в деструкторе — это ровно то, чего требовал комментарий ниже.
        QPointer<MpvWidget> guard(this);
        QTimer::singleShot(100, [guard]() {
            if (!guard) return;
            MpvWidget *self = guard.data();
            if (self->m_destroying.loadRelaxed()) return;
            // Мьютекс здесь намеренно не берётся. m_mutex — нерекурсивный QMutex,
            // а initializeMpv() захватывает его сам (см. initializeMpv). Внешний
            // QMutexLocker приводил либо к вечной блокировке, либо к тому, что
            // ранний выход initializeMpv() разлочивал чужой mutex. initializeMpv()
            // делает двойную проверку под мьютексом, поэтому внешняя блокировка
            // не требуется.
            if (!self->m_mpv) {
                (void)self->initializeMpv();
            }
#ifdef HAS_MPV_RENDER
#ifndef NO_OPENGL
            // Reinitialize OpenGL context now that we have a valid surface
            if (self->m_mpv && !self->m_mpvGL) {
                self->initializeGL();
            }
#endif // NO_OPENGL
#endif // HAS_MPV_RENDER
        });
    }
#endif // HAS_MPV
}

#ifdef HAS_MPV
void MpvWidget::onMpvEvents()
{
    QMutexLocker locker(&m_mutex);
    if (!m_mpv || m_destroying.loadRelaxed()) return;

    // Обрабатываем все накопившиеся события
    while (true) {
        mpv_event *event = mpv_wait_event(m_mpv, 0);
        if (event->event_id == MPV_EVENT_NONE) {
            break;
        }

        processMpvEvent(event);
    }

    bool hasEvents = !m_eventBuffer.isEmpty();
    locker.unlock();

    // Эмитируем буферизированные события после разблокировки мьютекса
    if (hasEvents) {
        emitBufferedEvents();
    }
    // Trigger repaint after processing events
    update();
}
#else
void MpvWidget::onMpvEvents() { qDebug() << "MpvWidget: onMpvEvents вызван без поддержки mpv"; }
#endif // HAS_MPV

#ifdef HAS_MPV
void MpvWidget::processMpvEvent(mpv_event *event)
{
    switch (event->event_id) {
    case MPV_EVENT_PROPERTY_CHANGE: {
        mpv_event_property *prop = static_cast<mpv_event_property *>(event->data);

        if (strcmp(prop->name, "time-pos") == 0 && prop->format == MPV_FORMAT_DOUBLE) {
            // FIX: nullptr check for prop->data
            double *posData = static_cast<double *>(prop->data);
            if (posData != nullptr) {
                m_position = *posData;
            } else {
                m_position = 0.0;
            }
            MpvEventData eventData;
            eventData.type = MpvEventData::PositionChanged;
            eventData.value = m_position;
            m_eventBuffer.append(eventData);
        }
        else if (strcmp(prop->name, "duration") == 0 && prop->format == MPV_FORMAT_DOUBLE) {
            // FIX: nullptr check for prop->data
            double *durData = static_cast<double *>(prop->data);
            if (durData != nullptr) {
                m_duration = *durData;
            } else {
                m_duration = 0.0;
            }
            MpvEventData eventData;
            eventData.type = MpvEventData::DurationChanged;
            eventData.value = m_duration;
            m_eventBuffer.append(eventData);
        }
        else if (strcmp(prop->name, "pause") == 0 && prop->format == MPV_FORMAT_FLAG) {
            // FIX: nullptr check for prop->data
            int *pauseData = static_cast<int *>(prop->data);
            if (pauseData != nullptr) {
                m_paused = *pauseData != 0;
            }
        }
        else if (strcmp(prop->name, "eof-reached") == 0 && prop->format == MPV_FORMAT_FLAG) {
            // Конец файла по достижении eof-reached. Основной путь — END_FILE,
            // но eof-reached приходит раньше и позволяет приложению узнать о
            // завершении, не дожидаясь разрыва соединения с потоком.
            int *eofData = static_cast<int *>(prop->data);
            if (eofData != nullptr && *eofData != 0) {
                MpvEventData eventData;
                eventData.type = MpvEventData::PlaybackFinished;
                m_eventBuffer.append(eventData);
            }
        }
        break;
    }

    case MPV_EVENT_END_FILE: {
        mpv_event_end_file *endFile = static_cast<mpv_event_end_file *>(event->data);
        if (endFile && endFile->reason == MPV_END_FILE_REASON_EOF) {
            MpvEventData eventData;
            eventData.type = MpvEventData::PlaybackFinished;
            m_eventBuffer.append(eventData);
        }
        else if (endFile && endFile->reason == MPV_END_FILE_REASON_ERROR) {
            MpvEventData eventData;
            eventData.type = MpvEventData::Error;
            eventData.message = tr("Ошибка воспроизведения: код ошибки %1").arg(endFile ? endFile->error : -1);
            m_eventBuffer.append(eventData);
        }
        break;
    }

    case MPV_EVENT_FILE_LOADED: {
        qDebug() << "MpvWidget: файл успешно загружен";
        // Перемотка могла уйти в mpv до открытия файла и быть отброшена.
        // Теперь файл загружен — применяем ожидающую позицию заново.
        // Это восстанавливает просмотр после смены stream-тикета: mpv
        // перезагружает файл, и без этого воспроизведение начиналось с нуля.
        if (!m_seekApplied && m_pendingSeekPosition > 0.0 && m_seekDebounceTimer) {
            qDebug() << "MpvWidget: применяю отложенную перемотку на"
                     << m_pendingSeekPosition;
            m_seekDebounceTimer->stop();
            m_seekDebounceTimer->start();
        }
        break;
    }

    case MPV_EVENT_LOG_MESSAGE: {
        mpv_event_log_message *msg = static_cast<mpv_event_log_message *>(event->data);
        if (msg && msg->log_level == MPV_LOG_LEVEL_ERROR) {
            qWarning() << "MPV Error:" << msg->prefix << msg->text;
            if (!m_lastErrorEmit.isValid() || m_lastErrorEmit.elapsed() >= 100) {
                m_lastErrorEmit.start();
                MpvEventData eventData;
                eventData.type = MpvEventData::Error;
                eventData.message = tr("MPV: %1").arg(QString::fromUtf8(msg->text));
                m_eventBuffer.append(eventData);
            }
        } else if (msg && msg->log_level == MPV_LOG_LEVEL_WARN) {
            qDebug() << "MPV Warn:" << msg->prefix << msg->text;
        } else if (msg) {
            qDebug() << "MPV Log:" << msg->prefix << msg->text;
        }
        break;
    }

    case MPV_EVENT_SHUTDOWN: {
        qDebug() << "MpvWidget: mpv shutdown";
        break;
    }

    default:
        break;
    }
}
#endif // HAS_MPV

void MpvWidget::emitBufferedEvents()
{
    // Копируем буфер и очищаем его под блокировкой
    QVector<MpvEventData> events;
    {
        QMutexLocker locker(&m_mutex);
        events = m_eventBuffer;
        m_eventBuffer.clear();
    }

    // Эмитируем сигналы без блокировки мьютекса
    for (const MpvEventData &event : events) {
        switch (event.type) {
        case MpvEventData::PositionChanged:
            emit positionChanged(event.value);
            break;
        case MpvEventData::DurationChanged:
            emit durationChanged(event.value);
            break;
        case MpvEventData::PlaybackFinished:
            emit playbackFinished();
            break;
        case MpvEventData::Error:
            emit error(event.message);
            break;
        case MpvEventData::Ready:
            emit ready();
            break;
        }
    }
}

#ifdef HAS_MPV
int MpvWidget::getProperty(const char *name, mpv_format format, void *data)
{
    if (!m_mpv) return -1;

    return mpv_get_property(m_mpv, name, format, data);
}
#endif // HAS_MPV