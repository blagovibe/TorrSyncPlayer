/**
 * @file mpvwidget.h
 * @brief Виджет видеоплеера на базе libmpv
 * 
 * Обеспечивает воспроизведение видео через libmpv с поддержкой:
 * - Воспроизведение URL (HTTP потоки)
 * - Пауза/возобновление
 * - Перемотка
 * - Синхронизация позиции воспроизведения
 */

#ifndef MPVWIDGET_H
#define MPVWIDGET_H

#include <QWidget>
#include <QMutex>
#include <QVector>
#include <QTimer>

// Обёртка для C-заголовка libmpv
#ifdef HAS_MPV
extern "C" {
#include <mpv/client.h>
#include <mpv/render_gl.h>
}
#endif

/**
 * @class MpvWidget
 * @brief Виджет для воспроизведения видео через libmpv
 * 
 * Использует mpv_create() для создания экземпляра плеера
 * и mpv_render_context для рендеринга видео в Qt окне.
 * Потокобезопасен благодаря использованию QMutex.
 */
class MpvWidget : public QWidget
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор виджета mpv
     * Инициализирует mpv экземпляр и контекст рендеринга
     * @param parent Родительский виджет
     */
    explicit MpvWidget(QWidget *parent = nullptr);
    
    /**
     * @brief Деструктор - освобождает mpv ресурсы
     * Закрывает контекст рендеринга и уничтожает mpv экземпляр
     */
    ~MpvWidget();

    /**
     * @brief Начать воспроизведение URL
     * @param url Адрес медиа (http://, file:// и т.д.)
     */
    void play(const QString &url);

    /**
     * @brief Поставить на паузу
     * Приостанавливает воспроизведение без сброса позиции
     */
    void pause();

    /**
     * @brief Возобновить воспроизведение
     * Продолжает воспроизведение с текущей позиции
     */
    void resume();

    /**
     * @brief Перемотать на позицию
     * @param position Позиция в секундах
     */
    void seek(double position);

    /**
     * @brief Получить текущую позицию воспроизведения
     * @return Позиция в секундах
     */
    double position() const;

    /**
     * @brief Получить длительность медиа
     * @return Длительность в секундах
     */
    double duration() const;

    /**
     * @brief Проверить, стоит ли воспроизведение на паузе
     * @return true если на паузе
     */
    bool isPaused() const;

    /**
     * @brief Проверить, инициализирован ли mpv
     * @return true если mpv готов к работе
     */
    bool isInitialized() const { return m_mpv != nullptr; }

signals:
    /**
     * @brief Сигнал изменения позиции воспроизведения
     * Испускается при изменении текущей позиции
     * @param position Новая позиция в секундах
     */
    void positionChanged(double position);

    /**
     * @brief Сигнал изменения длительности медиа
     * Испускается при получении информации о длительности
     * @param duration Новая длительность в секундах
     */
    void durationChanged(double duration);

    /**
     * @brief Сигнал завершения воспроизведения
     * Испускается когда видео доходит до конца
     */
    void playbackFinished();

    /**
     * @brief Сигнал ошибки
     * Испускается при возникновении ошибки воспроизведения
     * @param message Описание ошибки
     */
    void error(const QString &message);

    /**
     * @brief Сигнал готовности mpv к воспроизведению
     * Испускается после успешной инициализации
     */
    void ready();

protected:
    /**
     * @brief Обработка событий виджета
     * Перехватывает события mpv для обработки в основном потоке
     * @param event Событие Qt
     * @return true если событие обработано
     */
    bool event(QEvent *event) override;

    /**
     * @brief Обработка показа виджета
     * Инициализирует контекст рендеринга при первом показе
     * @param event Событие показа
     */
    void showEvent(QShowEvent *event) override;

private slots:
    /**
     * @brief Обработка событий mpv в основном потоке
     * Вызывается таймером для обработки накопленных событий
     */
    void onMpvEvents();

    /**
     * @brief Эмиссия буферизированных событий
     * Вызывается после разблокировки мьютекса для безопасной эмиссии сигналов
     */
    void emitBufferedEvents();

    /**
     * @brief Выполнение отложенной перемотки после debounce
     * Вызывается таймером для предотвращения утечки при быстрой перемотке
     */
    void onSeekDebounceTimeout();

private:
    /**
     * @brief Инициализация mpv экземпляра
     * Создаёт mpv и настраивает параметры воспроизведения
     * @return true если инициализация успешна
     */
    bool initializeMpv();

#ifdef HAS_MPV
    /**
     * @brief Обработка событий от mpv
     * Обрабатывает события изменения позиции, длительности, ошибок
     * @param event Событие mpv
     */
    void processMpvEvent(mpv_event *event);

    /**
     * @brief Получение свойства mpv
     * Запрашивает значение свойства у mpv экземпляра
     * @param name Имя свойства
     * @param format Формат данных
     * @param[out] data Буфер для данных
     * @return Код ошибки mpv
     */
    int getProperty(const char *name, mpv_format format, void *data);

    mpv_handle *m_mpv = nullptr;        ///< Экземпляр mpv
    mpv_render_context *m_mpvGL = nullptr; ///< Контекст рендеринга OpenGL
#else
    void *m_mpv = nullptr;        ///< Заглушка для совместимости
    void *m_mpvGL = nullptr;      ///< Заглушка для совместимости
#endif
    mutable QMutex m_mutex;             ///< Мьютекс для потокобезопасности
    bool m_initialized = false;         ///< Флаг инициализации
    double m_position = 0.0;            ///< Текущая позиция
    double m_duration = 0.0;            ///< Длительность медиа
    bool m_paused = false;              ///< Флаг паузы
    QTimer *m_seekDebounceTimer = nullptr;   ///< Таймер debounce для перемотки
    QTimer *m_eventTimer = nullptr;          ///< Таймер для обработки событий mpv
    double m_pendingSeekPosition = 0.0;      ///< Ожидающая позиция перемотки
    
    // Буфер для событий, ожидающих эмиссии сигналов (защищён m_mutex)
    struct MpvEventData {
        enum Type {
            PositionChanged,
            DurationChanged,
            PlaybackFinished,
            Error,
            Ready
        };
        Type type;
        double value;       ///< для position/duration
        QString message;    ///< для error
    };
    QVector<MpvEventData> m_eventBuffer; ///< Буфер событий для безопасной эмиссии
};

#endif // MPVWIDGET_H
