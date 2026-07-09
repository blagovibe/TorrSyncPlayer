/**
 * @file networkmanager.h
 * @brief HTTP клиент для связи с Go backend
 * 
 * Обеспечивает взаимодействие с API сервера TorrServer:
 * - Управление торрентами (добавление, удаление, список)
 * - Управление комнатами (создание, присоединение, сигналы)
 * - Синхронизация воспроизведения
 * - SSE подписка на события комнаты
 * - Retry logic с экспоненциальным backoff
 * - Автоматическое переподключение SSE
 */

#ifndef NETWORKMANAGER_H
#define NETWORKMANAGER_H

#include "inetworkmanager.h"

#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QUrl>
#include <QJsonObject>
#include <QJsonArray>
#include <QJsonDocument>
#include <QTimer>
#include <QPointer>
#include <QAtomicInt>
#include <QMutex>
#include <QDebug>

/**
 * @struct RetryRequest
 * @brief Структура для хранения информации о повторном запросе
 */
enum class RequestType {
    ListTorrents,
    AddTorrent,
    RemoveTorrent,
    GetFiles,
    SelectFile,
    StreamFile,
    SetBufferPosition,
    GetBufferInfo,
    CreateRoom,
    JoinRoom,
    LeaveRoom,
    Signal,
    RoomEvents,
    SyncPlay,
    SyncPause,
    SyncSeek,
    SyncStatus,
    HealthCheck,
    Version,
    Unknown
};

struct RetryRequest {
    QString path;
    QJsonObject body;
    QString method; // "GET", "POST", "DELETE"
    RequestType type = RequestType::Unknown;
    int attempt = 0;
};

/**
 * @class NetworkManager
 * @brief Менеджер сетевых запросов к Go backend
 *
 * Базовый URL: http://localhost:8889
 * API версия: v1
 * Поддерживает REST API и SSE подписки.
 * Все запросы выполняются асинхронно.
 * Встроенный retry logic с экспоненциальным backoff.
 * Реализует интерфейс INetworkManager для возможности подмены в тестах.
 */
class NetworkManager : public INetworkManager
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор менеджера сети
     * Инициализирует QNetworkAccessManager и базовый URL
     * @param parent Родительский объект
     */
    explicit NetworkManager(QObject *parent = nullptr);
    
    /**
     * @brief Деструктор - отменяет все активные запросы
     * Закрывает SSE соединение и освобождает ресурсы
     */
    ~NetworkManager();

    // ── Torrent API ───────────────────────────────────────────────────
    
    /**
     * @brief Добавить торрент по magnet-ссылке
     * Отправляет POST запрос на /api/v1/torrents
     * @param magnetUri Magnet-ссылка на торрент
     */
    void addTorrent(const QString &magnetUri);

    /**
     * @brief Добавить торрент из файла .torrent
     * Отправляет POST запрос на /api/v1/torrents с base64 закодированным содержимым файла
     * @param torrentData Содержимое файла .torrent (bencoded)
     */
    void addTorrentFile(const QByteArray &torrentData);
    
    /**
     * @brief Удалить торрент
     * Отправляет DELETE запрос на /api/v1/torrents/{id}
     * @param id ID торрента
     */
    void removeTorrent(const QString &id);
    
    /**
     * @brief Получить список торрентов
     * Отправляет GET запрос на /api/v1/torrents
     */
    void listTorrents();
    
    /**
     * @brief Получить список файлов торрента
     * Отправляет GET запрос на /api/v1/torrents/{id}/files
     * @param torrentId ID торрента
     */
    void getFiles(const QString &torrentId);
    
    /**
     * @brief Выбрать файл для воспроизведения
     * Отправляет POST запрос на /api/v1/torrents/{id}/select
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     */
    void selectFile(const QString &torrentId, int fileIndex);

    // ── Room API ──────────────────────────────────────────────────────
    
    /**
     * @brief Создать новую комнату
     * Отправляет POST запрос на /api/v1/rooms
     * @param name Имя комнаты
     * @param password Пароль (опционально)
     */
    void createRoom(const QString &name, const QString &password = "");
    
    /**
     * @brief Присоединиться к комнате
     * Отправляет POST запрос на /api/v1/rooms/join
     * @param roomId ID комнаты
     * @param password Пароль (опционально)
     */
    void joinRoom(const QString &roomId, const QString &password = "");
    
    /**
     * @brief Покинуть текущую комнату
     * Отправляет POST запрос на /api/v1/rooms/leave
     */
    void leaveRoom();
    
    /**
     * @brief Отправить сигнал в комнату
     * Отправляет POST запрос на /api/v1/rooms/signal
     * @param signal JSON объект сигнала WebRTC
     */
    void sendSignal(const QJsonObject &signal);

    // ── Sync API ──────────────────────────────────────────────────────
    
    /**
     * @brief Синхронизировать воспроизведение (play)
     * Отправляет POST запрос на /api/v1/sync/play
     */
    void syncPlay();
    
    /**
     * @brief Синхронизировать паузу
     * Отправляет POST запрос на /api/v1/sync/pause
     */
    void syncPause();
    
    /**
     * @brief Синхронизировать перемотку
     * Отправляет POST запрос на /api/v1/sync/seek
     * @param position Позиция в секундах
     */
    void syncSeek(double position);

    // ── Утилиты ───────────────────────────────────────────────────────
    
    /**
     * @brief Получить URL потока для торрента
     * @param torrentId ID торрента
     * @return Полный URL для воспроизведения
     */
    QString streamUrl(const QString &torrentId) const;
    
    /**
     * @brief Получить базовый URL сервера
     * @return URL сервера
     */
    QString serverUrl() const { return m_serverUrl.toString(); }
    
    /**
     * @brief Установить базовый URL сервера
     * @param url Новый URL
     */
    void setServerUrl(const QUrl &url) {
        if (url.isEmpty()) {
            qWarning() << "NetworkManager: empty URL, keeping default";
            return;
        }
        if (url.scheme() != "https" && url.scheme() != "http") {
            qWarning() << "NetworkManager: unsupported URL scheme, defaulting to HTTPS";
            QUrl fixed(url);
            fixed.setScheme("https");
            if (fixed.port() == -1) {
                fixed.setPort(443);
            }
            m_serverUrl = fixed;
        } else {
            m_serverUrl = url;
        }
    }

    void setAuthToken(const QString &token) {
        QMutexLocker locker(&m_authTokenMutex);
        m_authToken = token;
    }
    void clearAuthToken() {
        QMutexLocker locker(&m_authTokenMutex);
        m_authToken.clear();
    }
    QString authToken() const {
        QMutexLocker locker(&m_authTokenMutex);
        return m_authToken;
    }
    
    /**
     * @brief Проверить, подключен ли к комнате
     * @return true если в комнате
     */
    bool isInRoom() const {
        QMutexLocker locker(&m_roomIdMutex);
        return !m_currentRoomId.isEmpty();
    }
    
    /**
     * @brief Получить ID текущей комнаты (потокобезопасно)
     * @return ID комнаты
     */
    QString currentRoomId() const {
        QMutexLocker locker(&m_roomIdMutex);
        return m_currentRoomId;
    }
    
    /**
     * @brief Проверить доступность сервера
     * @return true если сервер доступен
     */
    bool isServerAvailable() const { return m_serverAvailable.loadRelaxed(); }
    
    /**
     * @brief Режим проверки SSL-сертификатов
     */
    enum class SslMode {
        Strict,     ///< Verify all certificates (production)
        AllowSelfSigned  ///< Accept self-signed certs for localhost (development)
    };

    /**
     * @brief Установить режим проверки SSL-сертификатов
     * @param mode Strict — проверять все сертификаты (production),
     *             AllowSelfSigned — принимать self-signed для localhost (разработка)
     */
    void setSslMode(SslMode mode) { m_sslMode = mode; }

    /**
     * @brief Получить текущий режим проверки SSL
     */
    SslMode sslMode() const { return m_sslMode; }

    /**
     * @brief Получить максимальное количество попыток
     * @return Максимум попыток
     */
    int maxRetries() const { return m_maxRetries; }
    
    /**
     * @brief Установить максимальное количество попыток
     * @param retries Количество попыток (1-10)
     */
    void setMaxRetries(int retries) { m_maxRetries = qBound(1, retries, 10); }
    
    /**
     * @brief Получить базовую задержку для retry (мс)
     * @return Базовая задержка в миллисекундах
     */
    int retryBaseDelay() const { return m_retryBaseDelay; }
    
    /**
     * @brief Установить базовую задержку для retry
     * @param delayMs Задержка в миллисекундах (100-10000)
     */
    void setRetryBaseDelay(int delayMs) { m_retryBaseDelay = qBound(100, delayMs, 10000); }
    
    /**
     * @brief Парсинг JSON ответа
     * @param data Сырые данные
     * @return JSON документ
     */
    QJsonDocument parseJson(const QByteArray &data);

signals:
    // ── Torrent signals ───────────────────────────────────────────────
    
    /**
     * @brief Торрент добавлен
     * Испускается после успешного добавления торрента
     * @param torrent JSON объект с информацией о торренте
     */
    void torrentAdded(const QJsonObject &torrent);
    
    /**
     * @brief Торрент удалён
     * Испускается после успешного удаления торрента
     * @param id ID удалённого торрента
     */
    void torrentRemoved(const QString &id);
    
    /**
     * @brief Получен список торрентов
     * Испускается после получения списка от сервера
     * @param torrents JSON массив торрентов
     */
    void torrentListReceived(const QJsonArray &torrents);
    
    /**
     * @brief Получен список файлов
     * Испускается после получения списка файлов торрента
     * @param torrentId ID торрента
     * @param files JSON массив файлов
     */
    void filesReceived(const QString &torrentId, const QJsonArray &files);

    // ── Room signals ──────────────────────────────────────────────────
    
    /**
     * @brief Комната создана
     * Испускается после успешного создания комнаты
     * @param roomId ID созданной комнаты
     */
    void roomCreated(const QString &roomId);
    
    /**
     * @brief Присоединились к комнате
     * Испускается после успешного присоединения
     * @param roomId ID комнаты
     */
    void roomJoined(const QString &roomId);
    
    /**
     * @brief Покинули комнату
     * Испускается после выхода из комнаты
     */
    void roomLeft();
    
    /**
     * @brief Получено событие комнаты (SSE)
     * Испускается при получении события через SSE
     * @param event JSON объект события
     */
    void roomEvent(const QJsonObject &event);
    
    /**
     * @brief Получен сигнал от пира
     * Испускается при получении WebRTC сигнала
     * @param signal JSON объект сигнала
     */
    void signalReceived(const QJsonObject &signal);

    // ── Sync signals ──────────────────────────────────────────────────
    
    /**
     * @brief Получен статус синхронизации
     * Испускается при получении статуса от сервера
     * @param status JSON объект статуса
     */
    void syncStatusReceived(const QJsonObject &status);

    // ── Error signal ──────────────────────────────────────────────────
    
    /**
     * @brief Ошибка сети или API
     * Испускается при возникновении ошибки
     * @param message Описание ошибки
     */
    void error(const QString &message);
    
    /**
     * @brief Сервер стал недоступен
     * Испускается при потере связи с сервером
     */
    void serverUnavailable();
    
    /**
     * @brief Сервер стал доступен
     * Испускается при восстановлении связи
     */
    void serverAvailable();

private slots:
    /**
     * @brief Обработка завершения запроса
     * Парсит ответ и испускает соответствующий сигнал
     * @param reply Ответ сервера
     */
    void onReplyFinished(QNetworkReply *reply);
    
    /**
     * @brief Обработка SSE данных
     * Читает данные из SSE потока и парсит события
     */
    void onSsEReadyRead();
    
    /**
     * @brief Обработка SSL ошибок
     * @param reply Ответ с ошибкой
     * @param errors Список SSL ошибок
     */
    void onSslErrors(QNetworkReply *reply, const QList<QSslError> &errors);

    void onNetworkError(QNetworkReply::NetworkError code);
    
    /**
     * @brief Повторная отправка запроса
     * Вызывается таймером для retry с экспоненциальным backoff
     */
    void retryRequest();
    
    /**
     * @brief Переподключение SSE
     * Вызывается таймером для автоматического переподключения SSE
     */
    void onSSEReconnect();

private:
    /**
     * @brief Отправить GET запрос
     * @param path Путь API (без базового URL)
     */
    void sendGet(const QString &path, RequestType type);
    
    /**
     * @brief Отправить POST запрос
     * @param path Путь API (без базового URL)
     * @param body Тело запроса (JSON)
     */
    void sendPost(const QString &path, const QJsonObject &body, RequestType type);
    
    /**
     * @brief Отправить DELETE запрос
     * @param path Путь API (без базового URL)
     */
    void sendDelete(const QString &path, RequestType type);
    
    /**
     * @brief Отправить запрос с поддержкой retry
     * @param method Метод ("GET", "POST", "DELETE")
     * @param path Путь API
     * @param body Тело запроса (для POST)
     */
    void sendWithRetry(const QString &method, const QString &path, const QJsonObject &body);
    void sendWithRetry(const QString &method, const QString &path, RequestType type, const QJsonObject &body = QJsonObject());
    
    /**
     * @brief Подключиться к SSE потоку
     * Устанавливает постоянное соединение для получения событий
     * @param path Путь SSE endpoint
     */
    void connectToSSE(const QString &path);
    
    /**
     * @brief Отключиться от SSE потока
     * Закрывает текущее SSE соединение
     */
    void disconnectSSE();

    /**
     * @brief Применить Bearer-авторизацию к запросу
     * @param request Запрос для модификации
     */
    void applyAuthHeader(QNetworkRequest &request);

    /**
     * @brief Обработка ошибки API
     * Парсит ошибку из ответа и испускает сигнал error
     * @param reply Ответ сервера
     */
    void handleApiError(QNetworkReply *reply);
    
    /**
     * @brief Вычислить задержку для retry (экспоненциальный backoff)
     * @param attempt Номер попытки (0-based)
     * @return Задержка в миллисекундах
     */
    int calculateRetryDelay(int attempt) const;

    QNetworkAccessManager *m_network;       ///< Менеджер сетевых запросов
    QUrl m_serverUrl;                       ///< Базовый URL сервера
    QPointer<QNetworkReply> m_sseReply;     ///< Текущий SSE ответ (QPointer для безопасности от nullptr)
    mutable QMutex m_roomIdMutex;
    QString m_currentRoomId;                ///< ID текущей комнаты
    QMap<QNetworkReply*, RequestType> m_replyMap; ///< Карта запросов для идентификации
    mutable QMutex m_replyMutex;            ///< Мьютекс для потокобезопасного доступа к m_replyMap
    
    // ── Retry logic ───────────────────────────────────────────────────
    int m_maxRetries = 3;                   ///< Максимальное количество попыток
    int m_retryBaseDelay = 1000;            ///< Базовая задержка retry (мс)
    QTimer *m_retryTimer = nullptr;         ///< Таймер для retry
    mutable QMutex m_retryMutex;            ///< Mutex for thread-safe retry state access
    RetryRequest m_pendingRetry;            ///< Ожидающий retry запрос
    int m_retrySeq = 0;                     ///< Sequence number for the current pending retry
    int m_pendingRetrySeq = 0;              ///< Sequence number captured at retry scheduling
    QAtomicInt m_serverAvailable{1};        ///< Флаг доступности сервера (потокобезопасный)

    // ── Auth ───────────────────────────────────────────────────────────
    mutable QMutex m_authTokenMutex;        ///< Mutex for thread-safe auth token access
    QString m_authToken;                    ///< JWT Bearer token for API auth
    void fetchCsrfToken();                  ///< Запросить CSRF-токен с сервера
    mutable QMutex m_csrfTokenMutex;
    QString m_csrfToken;
    bool m_csrfReady = false;
    struct PendingCsrfRequest {
        QString method;
        QString path;
        QJsonObject body;
        RequestType type;
    };
    QVector<PendingCsrfRequest> m_csrfPendingQueue;
    void applyCsrfHeader(QNetworkRequest &request);
    void enqueueOrSend(const QString &method, const QString &path, const QJsonObject &body, RequestType type);
    void flushCsrfQueue();

    // ── SSL/TLS mode ───────────────────────────────────────────────────
    SslMode m_sslMode = SslMode::Strict;    ///< Current SSL verification mode

    // ── SSE Reconnect ─────────────────────────────────────────────────
    QTimer *m_sseReconnectTimer = nullptr;  ///< Таймер для SSE переподключения
    QAtomicInt m_sseReconnectAttempts{0};   ///< Счётчик попыток SSE переподключения
    QString m_sseReconnectPath;             ///< Путь для SSE переподключения
    qint64 m_sseTotalBytesRead = 0;         ///< Общий объём данных SSE (защита от DoS)
};

#endif // NETWORKMANAGER_H
