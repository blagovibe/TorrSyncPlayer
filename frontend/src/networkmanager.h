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

/**
 * @struct RetryRequest
 * @brief Структура для хранения информации о повторном запросе
 */
struct RetryRequest {
    QString path;
    QJsonObject body;
    QString method; // "GET", "POST", "DELETE"
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
    void setServerUrl(const QUrl &url) { m_serverUrl = url; }
    
    /**
     * @brief Проверить, подключен ли к комнате
     * @return true если в комнате
     */
    bool isInRoom() const { return !m_currentRoomId.isEmpty(); }
    
    /**
     * @brief Получить ID текущей комнаты
     * @return ID комнаты
     */
    QString currentRoomId() const { return m_currentRoomId; }
    
    /**
     * @brief Проверить доступность сервера
     * @return true если сервер доступен
     */
    bool isServerAvailable() const { return m_serverAvailable; }
    
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
     * @brief Обработка ошибки сети
     * @param code Код ошибки
     */
    void onNetworkError(QNetworkReply::NetworkError code);
    
    /**
     * @brief Обработка SSL ошибок
     * @param reply Ответ с ошибкой
     * @param errors Список SSL ошибок
     */
    void onSslErrors(QNetworkReply *reply, const QList<QSslError> &errors);
    
    /**
     * @brief Повторная отправка запроса
     * Вызывается таймером для retry с экспоненциальным backoff
     */
    void retryRequest();

private:
    /**
     * @brief Отправить GET запрос
     * @param path Путь API (без базового URL)
     */
    void sendGet(const QString &path);
    
    /**
     * @brief Отправить POST запрос
     * @param path Путь API (без базового URL)
     * @param body Тело запроса (JSON)
     */
    void sendPost(const QString &path, const QJsonObject &body);
    
    /**
     * @brief Отправить DELETE запрос
     * @param path Путь API (без базового URL)
     */
    void sendDelete(const QString &path);
    
    /**
     * @brief Отправить запрос с поддержкой retry
     * @param method Метод ("GET", "POST", "DELETE")
     * @param path Путь API
     * @param body Тело запроса (для POST)
     */
    void sendWithRetry(const QString &method, const QString &path, const QJsonObject &body = QJsonObject());
    
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
    QNetworkReply *m_sseReply = nullptr;    ///< Текущий SSE ответ
    QString m_currentRoomId;                ///< ID текущей комнаты
    QMap<QNetworkReply*, QString> m_replyMap; ///< Карта запросов для идентификации
    
    // ── Retry logic ───────────────────────────────────────────────────
    int m_maxRetries = 3;                   ///< Максимальное количество попыток
    int m_retryBaseDelay = 1000;            ///< Базовая задержка retry (мс)
    QTimer *m_retryTimer = nullptr;         ///< Таймер для retry
    RetryRequest m_pendingRetry;            ///< Ожидающий retry запрос
    bool m_serverAvailable = true;          ///< Флаг доступности сервера
};

#endif // NETWORKMANAGER_H
