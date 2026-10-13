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

#include "interfaces/inetworkmanager.h"

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
    StreamTicket,
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
    SyncBuffering,
    SyncStatusGet,
    RoomInfoGet,
    Login,
    Register,
    Unknown
};

/**
 * @brief Which of the two servers a request is addressed to
 *
 * Every participant runs their own backend and pulls the media from the swarm
 * themselves; the host's server is a rendezvous for sync commands and the
 * roster only. Sending a request to the wrong one is not a performance
 * question — it decides who actually downloads the film.
 *
 * Declared inside NetworkManager (see below) because nothing outside the class
 * routes requests, and a namespace-scope enum with two users would only invite
 * the third to reach for it.
 */

struct RetryRequest {
    QString path;
    QJsonObject body;
    QString method; // "GET", "POST", "DELETE"
    RequestType type = RequestType::Unknown;
    int attempt = 0;
    int fileIndex = -1; // For SelectFile responses: which file was selected
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

    /**
     * @brief Обновить позицию буферизации
     * Отправляет POST запрос на /api/v1/torrents/{id}/buffer/position
     * @param torrentId ID торрента
     * @param positionBytes Текущая позиция воспроизведения в байтах
     */
    void setBufferPosition(const QString &torrentId, qint64 positionBytes);

    /**
     * @brief Запросить информацию о буферизации
     * Отправляет GET запрос на /api/v1/torrents/{id}/buffer/info
     * @param torrentId ID торрента
     */
    void getBufferInfo(const QString &torrentId);

    // ── Room API ──────────────────────────────────────────────────────
    
    /**
     * @brief Создать новую комнату
     * Отправляет POST запрос на /api/v1/rooms
     *
     * @param name Имя комнаты
     * @param password Пароль (опционально)
     * @param torrentId infohash торрента, к которому привязывается комната.
     *        Обязателен с idea-0046: комната смотрит конкретный торрент, и все
     *        её участники тянут его из роя каждый со своего бэкенда.
     */
    void createRoom(const QString &name, const QString &password, const QString &torrentId);

    /**
     * @brief Присоединиться к комнате
     * Отправляет POST запрос на /api/v1/rooms/join
     * @param roomId ID комнаты
     * @param password Пароль (опционально)
     * @param torrentId infohash торрента комнаты; сервер отклоняет вход, если он
     *        не совпадает с привязанным к комнате.
     */
    void joinRoom(const QString &roomId, const QString &password, const QString &torrentId);
    
    /**
     * @brief Покинуть текущую комнату
     * Отправляет POST запрос на /api/v1/rooms/leave
     */
    void leaveRoom();
    
    /**
     * @brief Отправить сигнал в комнату
     * Отправляет POST запрос на /api/v1/rooms/signal
     * @param signal JSON объект сигнала (ретранслируется сервером через SSE)
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

    /**
     * @brief Сообщить комнате о начале/окончании буферизации
     * Отправляет POST запрос на /api/v1/sync/buffering. Вызывается локально,
     * когда mpv вошёл в core-idle (данных не хватает) или вышел из него —
     * остальные участники паузят/возобновляют вместе с этим пиром, иначе
     * «смотреть вместе» рассыпается на рассинхрон.
     * @param buffering true — началась буферизация, false — закончилась
     */
    void syncBuffering(bool buffering);

    /**
     * @brief Запросить актуальный статус синхронизации
     * GET /api/v1/sync/status — позиция, play/pause, буферизация.
     * Вызывается автоматически при каждом открытии SSE-потока.
     */
    void fetchSyncStatus();

    /**
     * @brief Запросить информацию о текущей комнате
     * GET /api/v1/rooms/info — кто хост и сколько участников. Комната
     * определяется членством вызывающего, а не параметром пути.
     * Вызывается автоматически при каждом открытии SSE-потока.
     */
    void fetchRoomInfo();

    // ── Утилиты ───────────────────────────────────────────────────────
    
    /**
     * @brief Получить URL потока для торрента
     * @param torrentId ID торрента
     * @return Полный URL для воспроизведения
     */
    QString streamUrl(const QString &torrentId) const;

    /**
     * @brief Запросить подписанный stream-ticket для торрента
     * Эндпоинт /stream публичный и аутентифицируется тикетом (libmpv не может
     * прикрепить JWT-заголовок), поэтому тикет запрашивается отдельно и затем
     * добавляется к URL как "?ticket=".
     * @param torrentId ID торрента
     */
    void requestStreamTicket(const QString &torrentId);
    
    /**
     * @brief Базовый URL плоскости управления — сервера, где живёт комната
     *
     * Именно этот адрес имеется в виду под «сервером» во всём прежнем коде:
     * комнаты, ростер, команды синхронизации и SSE. Токен у него тоже свой —
     * чужой, если комната чужая.
     */
    QString serverUrl() const { return m_controlServerUrl.toString(); }
    QUrl controlServerUrl() const { return m_controlServerUrl; }

    /**
     * @brief Базовый URL локального бэкенда — плоскости данных
     *
     * Торренты, файлы, выбор файла, телеметрия буфера и сам медиапоток идут
     * сюда, а не к серверу хоста: каждый участник тянет данные из роя сам.
     */
    QUrl localBackendUrl() const { return m_localBackendUrl; }

    /**
     * @brief Установить базовый URL плоскости управления
     * @param url Новый URL
     */
    void setServerUrl(const QUrl &url) {
        setControlServerUrl(url);
    }

    /**
     * @brief Установить базовый URL локального бэкенда
     * @param url Новый URL
     */
    void setLocalBackendUrl(const QUrl &url) {
        if (url.isEmpty()) {
            qWarning() << "NetworkManager: empty local backend URL, keeping previous";
            return;
        }
        if (url.scheme() != QLatin1String("https") && url.scheme() != QLatin1String("http")) {
            qWarning() << "NetworkManager: unsupported local backend URL scheme" << url.scheme()
                       << "- keeping previous URL";
            return;
        }
        m_localBackendUrl = url;
    }

    /**
     * @brief Установить базовый URL плоскости управления
     * @param url Новый URL
     */
    void setControlServerUrl(const QUrl &url) {
        if (url.isEmpty()) {
            qWarning() << "NetworkManager: empty URL, keeping default";
            return;
        }
        // Only http/https are allowed. Never silently upgrade a custom scheme
        // (e.g. file://, ftp://) to https — that would point the client at an
        // unintended host and could leak the access token set via setAccessToken.
        if (url.scheme() != "https" && url.scheme() != "http") {
            qWarning() << "NetworkManager: unsupported URL scheme" << url.scheme() << ", keeping previous URL";
            return;
        }
        // Warn (but allow) when targeting a non-localhost host over plain http,
        // since credentials/tokens could be sent in cleartext.
        if (url.scheme() == "http" && url.host() != "localhost" && url.host() != "127.0.0.1") {
            qWarning() << "NetworkManager: using plain HTTP to non-localhost host" << url.host()
                       << "- auth tokens will be sent unencrypted";
        }
        m_controlServerUrl = url;
    }

    /**
     * @brief Токен плоскости управления — токен чужого сервера
     *
     * Передаётся командной строкой, потому что задаётся человеком: он
     * передал адрес сервера и токен вместе.
     */
    void setAccessToken(const QString &token) {
        setControlAccessToken(token);
    }
    void setControlAccessToken(const QString &token) {
        QMutexLocker locker(&m_accessTokenMutex);
        m_accessToken = token;
    }
    void clearAccessToken() {
        QMutexLocker locker(&m_accessTokenMutex);
        m_accessToken.clear();
    }
    QString accessToken() const {
        QMutexLocker locker(&m_accessTokenMutex);
        return m_accessToken;
    }

    /**
     * @brief Токен локального бэкенда
     *
     * Другой секрет, чем у плоскости управления, и никогда не вводится
     * человеком: приложение само порождает его при запуске своего бэкенда
     * (bug-0060). Смешивать их нельзя — токен чужого сервера не подойдёт к
     * своему, а свой не откроет чужой.
     */
    void setLocalAccessToken(const QString &token) {
        QMutexLocker locker(&m_localTokenMutex);
        m_localAccessToken = token;
    }
    QString localAccessToken() const {
        QMutexLocker locker(&m_localTokenMutex);
        return m_localAccessToken;
    }

    /**
     * @brief Установить идентификатор клиента для P2P комнат
     * Отправляется как заголовок X-Client-ID и определяет, кто этот игрок
     * в рамках комнаты. Это не учётная запись: идентификатор живёт только
     * в пределах одного запуска и нигде не сохраняется.
     * @param clientId Уникальный идентификатор этого экземпляра плеера
     */
    void setClientId(const QString &clientId);

    /**
     * @brief Узнать собственный идентификатор клиента
     * Нужен, чтобы отличить «я хост» от «хост — кто-то другой»: ответ
     * /rooms/info приходит с hostUserId хоста, а не с флагом «вы хост».
     * @return X-Client-ID этого запуска (пустая строка, если не задан)
     */
    QString clientId() const;
    
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

    /**
     * @brief Файл выбран для воспроизведения (подтверждено сервером)
     * Испускается после успешного ответа /select. MainWindow/TorrentManager
     * используют этот сигнал для запроса stream-тикета и запуска воспроизведения.
     * @param torrentId ID торрента
     * @param fileIndex Индекс выбранного файла
     * @param streamUrl Базовый URL потока (без тикета)
     */
    void fileSelected(const QString &torrentId, int fileIndex, const QString &streamUrl);

    /**
     * @brief Получена информация о буферизации
     * Испускается после успешного запроса /buffer/info
     * @param info JSON объект BufferInfo
     */
    void bufferInfoReceived(const QJsonObject &info);

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
     * Испускается при получении сигнала, ретранслированного сервером через SSE
     * @param signal JSON объект сигнала
     */
    void signalReceived(const QJsonObject &signal);

    /**
     * @brief Получен актуальный статус синхронизации
     * Испускается в ответ на GET /api/v1/sync/status. После реконнекта SSE
     * это единственный способ узнать, где комната находится сейчас: пока
     * соединение рвалось, позиция и play/pause могли поменяться (bug-0012).
     * @param status JSON объект SyncStatus
     */
    void syncStatusReceived(const QJsonObject &status);

    /**
     * @brief Получена информация о текущей комнате
     * Испускается в ответ на GET /api/v1/rooms/info: кто хост и сколько
     * участников. После реконнекта состав комнаты мог измениться (bug-0012).
     * @param info JSON объект RoomInfo
     */
    void roomInfoReceived(const QJsonObject &info);

    /**
     * @brief SSE-поток комнаты открыт
     * Испускается ровно один раз на каждое успешно установленное соединение,
     * в том числе после реконнекта. Это единственный надёжный признак «мы
     * снова в комнате»: счётчик попыток и сам факт connectToSSE возникают
     * ДО того, как сервер принял запрос.
     */
    void sseStreamOpened();

    // ── Sync signals ──────────────────────────────────────────────────

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

    /**
     * @brief Тикет потока получен
     * @param torrentId ID торрента
     * @param ticket Подписанный тикет для /stream
     */
    void streamTicketReceived(const QString &torrentId, const QString &ticket);

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
     * @brief Обработка появления HTTP-ответа SSE-потока
     * Единственный достоверный признак того, что сервер принял поток событий
     * (заголовки 200 получены). Здесь, а не в connectToSSE(), обнуляется
     * счётчик попыток переподключения и запускается сверка состояния комнаты.
     */
    void onSseMetaDataChanged();
    
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
     * @brief Apply access-token authentication to a request
     * Sets X-Access-Token and X-Client-ID headers (no Bearer, no cookies).
     * @param request Request to modify
     */
    void applyAuthHeader(QNetworkRequest &request);
    void applyAuthHeader(QNetworkRequest &request, const QString &token);
    enum class Plane {
        ControlServer,  ///< Rooms, roster, sync commands, SSE
        LocalBackend,   ///< Torrents, files, selection, buffer telemetry, media
    };

    Plane planeFor(RequestType type) const;
    QUrl baseUrlFor(RequestType type) const;
    QString tokenFor(RequestType type) const;

    /**
     * @brief Обработка ошибки API
     * Парсит ошибку из ответа и испускает сигнал error
     * @param reply Ответ сервера
     */
    void handleApiError(QNetworkReply *reply, RequestType type);

    /**
     * @brief Вычислить задержку для retry (экспоненциальный backoff)
     * @param attempt Номер попытки (0-based)
     * @return Задержка в миллисекундах
     */
    int calculateRetryDelay(int attempt) const;

    QNetworkAccessManager *m_network;       ///< Менеджер сетевых запросов
    QUrl m_controlServerUrl;                ///< Базовый URL плоскости управления
    QUrl m_localBackendUrl;                 ///< Базовый URL локального бэкенда (данные)
    mutable QMutex m_localTokenMutex;         ///< Guards the local-backend token
    QString m_localAccessToken;             ///< Токен локального бэкенда
    QPointer<QNetworkReply> m_sseReply;     ///< Текущий SSE ответ (QPointer для безопасности от nullptr)
    mutable QMutex m_roomIdMutex;
    QString m_currentRoomId;                ///< ID текущей комнаты
    /// ID комнаты, отправленный в последнем POST /rooms/join.
    /// Ответ сервера поля id НЕ содержит, поэтому берём его отсюда: иначе
    /// roomId был пустым и SSE-путь собирался как /api/v1/rooms//events.
    QString m_pendingJoinRoomId;
    QMap<QNetworkReply*, RequestType> m_replyMap; ///< Карта запросов для идентификации
    QMap<QString, int> m_selectFileIndexByTorrent; ///< Индекс выбранного файла по torrentId (для ответа /select)
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
    mutable QMutex m_accessTokenMutex;      ///< Guards the control-plane token and the client id
    QString m_accessToken;                  ///< Per-process access token (X-Access-Token)
    QString m_clientId;                     ///< Client identifier for this run (X-Client-ID)

    // ── SSL/TLS mode ───────────────────────────────────────────────────
    SslMode m_sslMode = SslMode::Strict;    ///< Current SSL verification mode

    // ── SSE Reconnect ─────────────────────────────────────────────────
    QTimer *m_sseReconnectTimer = nullptr;  ///< Таймер для SSE переподключения
    QAtomicInt m_sseReconnectAttempts{0};   ///< Счётчик попыток SSE переподключения
    QString m_sseReconnectPath;             ///< Путь для SSE переподключения
    qint64 m_sseTotalBytesRead = 0;         ///< Общий объём данных SSE (защита от DoS)
    QAtomicInt m_sseStreamOpened{0};        ///< 1 — текущий SSE-поток уже открыт (ответ 200 получен);
                                            ///< гарантирует однократную сверку состояния на соединение
};

#endif // NETWORKMANAGER_H
