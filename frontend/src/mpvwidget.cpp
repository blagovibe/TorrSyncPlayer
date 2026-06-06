/**
 * @file mpvwidget.cpp
 * @brief Реализация виджета видеоплеера на базе libmpv
 */

#include "mpvwidget.h"

#include <QDebug>
#include <QTimer>
#include <QOpenGLContext>
#include <QOpenGLFramebufferObject>
#include <QWindow>
#include <QGuiApplication>
#include <QMetaObject>

// Таймер для обработки событий mpv
static const int MPV_EVENT_TIMER_MS = 30;

// Задержка debounce для перемотки (мс)
static const int SEEK_DEBOUNCE_MS = 300;

#ifdef HAS_MPV
// Static callback для mpv OpenGL (C-compatible, без capture)
static void *mpvGetProcAddress(void *ctx, const char *name)
{
    Q_UNUSED(ctx);
    QOpenGLContext *glctx = QOpenGLContext::currentContext();
    if (!glctx) return nullptr;
    return reinterpret_cast<void *>(glctx->getProcAddress(name));
}
#endif

MpvWidget::MpvWidget(QWidget *parent)
    : QWidget(parent)
{
    // Устанавливаем атрибуты для поддержки OpenGL
    setAttribute(Qt::WA_NativeWindow, true);
    setAttribute(Qt::WA_PaintOnScreen, true);
    setAttribute(Qt::WA_NoSystemBackground, true);

    // Устанавливаем политику фокуса
    setFocusPolicy(Qt::StrongFocus);

    // Устанавливаем минимальный размер
    setMinimumSize(400, 300);

    // Устанавливаем чёрный фон
    QPalette pal = palette();
    pal.setColor(QPalette::Window, Qt::black);
    setAutoFillBackground(true);
    setPalette(pal);

    // Создаём debounce таймер для перемотки
    m_seekDebounceTimer = new QTimer(this);
    m_seekDebounceTimer->setSingleShot(true);
    m_seekDebounceTimer->setInterval(SEEK_DEBOUNCE_MS);
    connect(m_seekDebounceTimer, &QTimer::timeout, this, &MpvWidget::onSeekDebounceTimeout);
}

MpvWidget::~MpvWidget()
{
    // Останавливаем debounce таймер
    if (m_seekDebounceTimer) {
        m_seekDebounceTimer->stop();
    }

#ifdef HAS_MPV
    // Освобождаем ресурсы mpv
    if (m_mpvGL) {
        mpv_render_context_free(m_mpvGL);
        m_mpvGL = nullptr;
    }

    if (m_mpv) {
        mpv_terminate_destroy(m_mpv);
        m_mpv = nullptr;
    }
#endif
}

bool MpvWidget::initializeMpv()
{
#ifdef HAS_MPV
    if (m_initialized) return true;
    // Блокируем мьютекс для инициализации
    {
        QMutexLocker locker(&m_mutex);

        // Создаём экземпляр mpv
        m_mpv = mpv_create();
        if (!m_mpv) {
            // Буферизируем событие ошибки для безопасной эмиссии
            MpvEventData event;
            event.type = MpvEventData::Error;
            event.message = tr("Не удалось создать mpv экземпляр");
            m_eventBuffer.append(event);
            QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
            return false;
        }

        // Настройки mpv
        mpv_set_option_string(m_mpv, "vo", "libmpv");  // Используем libmpv для рендеринга
        mpv_set_option_string(m_mpv, "hwdec", "auto");  // Автоматическое аппаратное декодирование
        mpv_set_option_string(m_mpv, "cache", "yes");   // Включаем кэширование
        mpv_set_option_string(m_mpv, "cache-secs", "30"); // Кэш 30 секунд
        mpv_set_option_string(m_mpv, "demuxer-max-bytes", "150M");
        mpv_set_option_string(m_mpv, "demuxer-max-back-bytes", "50M");

        // Отключаем аудио (опционально, можно включить)
        mpv_set_option_string(m_mpv, "ao", "null");

        // Инициализация mpv
        int err = mpv_initialize(m_mpv);
        if (err < 0) {
            // Буферизируем событие ошибки для безопасной эмиссии
            MpvEventData event;
            event.type = MpvEventData::Error;
            event.message = tr("Ошибка инициализации mpv: %1").arg(mpv_error_string(err));
            m_eventBuffer.append(event);
            mpv_terminate_destroy(m_mpv);
            m_mpv = nullptr;
            QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
            return false;
        }

        // Настройка рендеринга OpenGL
        // Используем static функцию для C callback (совместимо с MSVC)
        mpv_opengl_init_params gl_init_params = {
            mpvGetProcAddress,
            nullptr
        };

        mpv_render_param params[] = {
            {MPV_RENDER_PARAM_API_TYPE, const_cast<char *>(MPV_RENDER_API_TYPE_OPENGL)},
            {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init_params},
            {MPV_RENDER_PARAM_INVALID, nullptr}
        };

        // Создаём контекст рендеринга
        err = mpv_render_context_create(&m_mpvGL, m_mpv, params);
        if (err < 0) {
            // Буферизируем событие ошибки для безопасной эмиссии
            MpvEventData event;
            event.type = MpvEventData::Error;
            event.message = tr("Ошибка создания контекста рендеринга: %1").arg(mpv_error_string(err));
            m_eventBuffer.append(event);
            mpv_terminate_destroy(m_mpv);
            m_mpv = nullptr;
            QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
            return false;
        }

        // Устанавливаем callback для обновления
        mpv_render_context_set_update_callback(m_mpvGL,
            [](void *ctx) {
                MpvWidget *widget = static_cast<MpvWidget *>(ctx);
                QTimer::singleShot(0, widget, &MpvWidget::onMpvEvents);
            }, this);

        // Запускаем таймер для обработки событий mpv
        if (!m_eventTimer) {
            m_eventTimer = new QTimer(this);
            connect(m_eventTimer, &QTimer::timeout, this, &MpvWidget::onMpvEvents);
        }
        m_eventTimer->start(MPV_EVENT_TIMER_MS);

        m_initialized = true;

        // Буферизируем событие готовности для безопасной эмиссии
        MpvEventData event;
        event.type = MpvEventData::Ready;
        m_eventBuffer.append(event);
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
    } // Мьютекс разблокирован здесь

    qDebug() << "MpvWidget: mpv успешно инициализирован";
#else
    qDebug() << "MpvWidget: mpv не поддерживается (собрано без HAS_MPV)";
#endif
    return true;
}

void MpvWidget::play(const QString &url)
{
#ifdef HAS_MPV
    if (!m_mpv) {
        if (!initializeMpv()) {
            return;
        }
    }

    QMutexLocker locker(&m_mutex);

    const QByteArray urlData = url.toUtf8();
    const char *args[] = {"loadfile", urlData.constData(), "replace", nullptr};

    int err = mpv_command_async(m_mpv, 0, args);
    if (err < 0) {
        // Буферизируем событие ошибки для безопасной эмиссии
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
#endif
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
#endif
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
#endif
}

void MpvWidget::seek(double position)
{
#ifndef HAS_MPV
    Q_UNUSED(position);
    qWarning() << "mpv not available, seek ignored";
#else
    if (!m_mpv) return;

    // Ограничиваем позицию допустимыми значениями
    if (position < 0) position = 0;

    QMutexLocker locker(&m_mutex);

    if (m_duration > 0 && position > m_duration) {
        position = m_duration;
    }

    // Сохраняем позицию для отложенного выполнения
    m_pendingSeekPosition = position;

    // Сбрасываем предыдущий таймер и запускаем новый (debounce)
    // Это предотвращает утечку памяти при быстрой перемотке
    m_seekDebounceTimer->stop();
    m_seekDebounceTimer->start();

    qDebug() << "MpvWidget: перемотка запрошена на" << position << "(debounce " << SEEK_DEBOUNCE_MS << "мс)";
#endif
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
        // Буферизируем событие ошибки для безопасной эмиссии
        MpvEventData event;
        event.type = MpvEventData::Error;
        event.message = tr("Ошибка перемотки: %1").arg(mpv_error_string(err));
        m_eventBuffer.append(event);
        QMetaObject::invokeMethod(this, &MpvWidget::emitBufferedEvents, Qt::QueuedConnection);
        return;
    }

    qDebug() << "MpvWidget: перемотка выполнена на" << position;
#else
    qDebug() << "MpvWidget: перемотка невозможна (собрано без HAS_MPV)";
#endif
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
#endif
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
#endif
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
#endif
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

    return QWidget::event(event);
}

void MpvWidget::showEvent(QShowEvent *event)
{
    QWidget::showEvent(event);

#ifdef HAS_MPV
    if (!m_initialized) {
        QTimer::singleShot(100, this, [this]() {
            initializeMpv();
        });
    }
#endif
}

void MpvWidget::onMpvEvents()
{
#ifdef HAS_MPV
    if (!m_mpv) return;

    // Обрабатываем события внутри блокировки мьютекса
    bool hasEvents = false;
    {
        QMutexLocker locker(&m_mutex);

        // Обрабатываем все накопившиеся события
        while (true) {
            mpv_event *event = mpv_wait_event(m_mpv, 0);
            if (event->event_id == MPV_EVENT_NONE) {
                break;
            }

            processMpvEvent(event);
        }

        // Проверяем наличие событий под блокировкой
        hasEvents = !m_eventBuffer.isEmpty();
    } // Мьютекс разблокирован здесь

    // Эмитируем буферизированные события после разблокировки мьютекса
    // Это предотвращает race condition и potential deadlock
    if (hasEvents) {
        emitBufferedEvents();
    }
#else
    qDebug() << "MpvWidget: onMpvEvents вызван без поддержки mpv";
#endif
}

#ifdef HAS_MPV
void MpvWidget::processMpvEvent(mpv_event *event)
{
    switch (event->event_id) {
    case MPV_EVENT_PROPERTY_CHANGE: {
        mpv_event_property *prop = static_cast<mpv_event_property *>(event->data);

        if (strcmp(prop->name, "time-pos") == 0 && prop->format == MPV_FORMAT_DOUBLE) {
            m_position = *static_cast<double *>(prop->data);
            // Буферизируем событие вместо прямой эмиссии
            MpvEventData eventData;
            eventData.type = MpvEventData::PositionChanged;
            eventData.value = m_position;
            m_eventBuffer.append(eventData);
        }
        else if (strcmp(prop->name, "duration") == 0 && prop->format == MPV_FORMAT_DOUBLE) {
            m_duration = *static_cast<double *>(prop->data);
            // Буферизируем событие вместо прямой эмиссии
            MpvEventData eventData;
            eventData.type = MpvEventData::DurationChanged;
            eventData.value = m_duration;
            m_eventBuffer.append(eventData);
        }
        else if (strcmp(prop->name, "pause") == 0 && prop->format == MPV_FORMAT_FLAG) {
            m_paused = *static_cast<int *>(prop->data) != 0;
        }
        break;
    }

    case MPV_EVENT_END_FILE: {
        mpv_event_end_file *endFile = static_cast<mpv_event_end_file *>(event->data);
        if (endFile->reason == MPV_END_FILE_REASON_EOF) {
            // Буферизируем событие вместо прямой эмиссии
            MpvEventData eventData;
            eventData.type = MpvEventData::PlaybackFinished;
            m_eventBuffer.append(eventData);
        }
        else if (endFile->reason == MPV_END_FILE_REASON_ERROR) {
            // Обработка ошибки завершения файла
            MpvEventData eventData;
            eventData.type = MpvEventData::Error;
            eventData.message = tr("Ошибка воспроизведения: код ошибки %1").arg(endFile->error);
            m_eventBuffer.append(eventData);
        }
        break;
    }

    // Обработка ошибки загрузки файла (исправление: добавлена обработка ошибок mpv)
    case MPV_EVENT_FILE_LOADED: {
        // Файл успешно загружен — можно сбросить ошибки
        qDebug() << "MpvWidget: файл успешно загружен";
        break;
    }

    case MPV_EVENT_LOG_MESSAGE: {
        mpv_event_log_message *msg = static_cast<mpv_event_log_message *>(event->data);
        // Логируем ошибки и предупреждения mpv
        if (msg->log_level == MPV_LOG_LEVEL_ERROR) {
            qWarning() << "MPV Error:" << msg->prefix << msg->text;
            // Буферизируем событие ошибки для безопасной эмиссии
            MpvEventData eventData;
            eventData.type = MpvEventData::Error;
            eventData.message = tr("MPV: %1").arg(QString::fromUtf8(msg->text));
            m_eventBuffer.append(eventData);
        } else if (msg->log_level == MPV_LOG_LEVEL_WARN) {
            qDebug() << "MPV Warn:" << msg->prefix << msg->text;
        } else {
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
#endif

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
#endif
