/**
 * @file inetworkmanager.h
 * @brief Абстрактный интерфейс для NetworkManager
 * 
 * Позволяет подменять реализацию для тестирования.
 * Все публичные методы NetworkManager объявлены как чисто виртуальные.
 */

#ifndef INETWORKMANAGER_H
#define INETWORKMANAGER_H

#include <QObject>
#include <QJsonObject>
#include <QJsonArray>
#include <QUrl>
#include <QJsonDocument>

/**
 * @enum SslMode
 * @brief SSL verification mode
 */
enum class SslMode {
    Strict,
    AllowSelfSigned
};

/**
 * @class INetworkManager
 * @brief Абстрактный интерфейс для сетевых операций
 * 
 * Наследуется от QObject для поддержки сигналов/слотов.
 * Реализации должны обеспечивать все методы для работы с API.
 */
class INetworkManager : public QObject
{
    Q_OBJECT

public:
    explicit INetworkManager(QObject *parent = nullptr) : QObject(parent) {}
    virtual ~INetworkManager() = default;

    // ── Torrent API ───────────────────────────────────────────────────
    
    /**
     * @brief Добавить торрент по magnet-ссылке
     * @param magnetUri Magnet-ссылка на торрент
     */
    virtual void addTorrent(const QString &magnetUri) = 0;

    /**
     * @brief Добавить торрент из .torrent файла (base64)
     * @param torrentData Содержимое .torrent файла в base64
     */
    virtual void addTorrentFile(const QByteArray &torrentData) = 0;
    
    /**
     * @brief Удалить торрент
     * @param id ID торрента
     */
    virtual void removeTorrent(const QString &id) = 0;
    
    /**
     * @brief Получить список торрентов
     */
    virtual void listTorrents() = 0;
    
    /**
     * @brief Получить список файлов торрента
     * @param torrentId ID торрента
     */
    virtual void getFiles(const QString &torrentId) = 0;
    
    /**
     * @brief Выбрать файл для воспроизведения
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     */
    virtual void selectFile(const QString &torrentId, int fileIndex) = 0;

    /**
     * @brief Обновить позицию буферизации
     */
    virtual void setBufferPosition(const QString &torrentId, qint64 positionBytes) = 0;

    /**
     * @brief Запросить информацию о буферизации
     */
    virtual void getBufferInfo(const QString &torrentId) = 0;

    // ── Room API ──────────────────────────────────────────────────────
    
    /**
     * @brief Создать новую комнату
     * @param name Имя комнаты
     * @param password Пароль (опционально)
     */
    virtual void createRoom(const QString &name, const QString &password = "") = 0;
    
    /**
     * @brief Присоединиться к комнате
     * @param roomId ID комнаты
     * @param password Пароль (опционально)
     */
    virtual void joinRoom(const QString &roomId, const QString &password = "") = 0;
    
    /**
     * @brief Покинуть текущую комнату
     */
    virtual void leaveRoom() = 0;
    
    /**
     * @brief Отправить сигнал в комнату
     * @param signal JSON объект сигнала WebRTC
     */
    virtual void sendSignal(const QJsonObject &signal) = 0;

    // ── Sync API ──────────────────────────────────────────────────────
    
    /**
     * @brief Синхронизировать воспроизведение (play)
     */
    virtual void syncPlay() = 0;
    
    /**
     * @brief Синхронизировать паузу
     */
    virtual void syncPause() = 0;
    
    /**
     * @brief Синхронизировать перемотку
     * @param position Позиция в секундах
     */
    virtual void syncSeek(double position) = 0;

    // ── Утилиты ───────────────────────────────────────────────────────
    
    /**
     * @brief Получить URL потока для торрента
     * @param torrentId ID торрента
     * @return Полный URL для воспроизведения
     */
    virtual QString streamUrl(const QString &torrentId) const = 0;
    
    /**
     * @brief Получить базовый URL сервера
     * @return URL сервера
     */
    virtual QString serverUrl() const = 0;
    
    /**
     * @brief Установить базовый URL сервера
     * @param url Новый URL
     */
    virtual void setServerUrl(const QUrl &url) = 0;
    
    /**
     * @brief Проверить, подключен ли к комнате
     * @return true если в комнате
     */
    virtual bool isInRoom() const = 0;
    
    /**
     * @brief Получить ID текущей комнаты
     * @return ID комнаты
     */
    virtual QString currentRoomId() const = 0;
    
    /**
     * @brief Проверить доступность сервера
     * @return true если сервер доступен
     */
    virtual bool isServerAvailable() const = 0;

    // ── Authentication ────────────────────────────────────────────────────
    
    virtual void setAuthToken(const QString &token) = 0;
    virtual void clearAuthToken() = 0;
    virtual QString authToken() const = 0;

    // ── Retry configuration ──────────────────────────────────────────────────
    
    virtual void setMaxRetries(int retries) = 0;
    virtual int maxRetries() const = 0;
    virtual void setRetryBaseDelay(int delayMs) = 0;
    virtual int retryBaseDelay() const = 0;
    virtual void setSslMode(SslMode mode) = 0;
    virtual SslMode sslMode() const = 0;

    // ── JSON utilities ──────────────────────────────────────────────────────
    
    virtual QJsonDocument parseJson(const QByteArray &data) = 0;

signals:
    // ── Torrent signals ───────────────────────────────────────────────
    void torrentAdded(const QJsonObject &torrent);
    void torrentRemoved(const QString &id);
    void torrentListReceived(const QJsonArray &torrents);
    void filesReceived(const QString &torrentId, const QJsonArray &files);
    void bufferInfoReceived(const QJsonObject &info);

    // ── Room signals ──────────────────────────────────────────────────
    void roomCreated(const QString &roomId);
    void roomJoined(const QString &roomId);
    void roomLeft();
    void roomEvent(const QJsonObject &event);
    void signalReceived(const QJsonObject &signal);

    // ── Error signals ─────────────────────────────────────────────────
    void error(const QString &message);
    void serverUnavailable();
    void serverAvailable();

    // ── Auth signals ──────────────────────────────────────────────────
    void authenticated(const QString &token);

    // ── Stream ticket signal ──────────────────────────────────────────
    void streamTicketReceived(const QString &torrentId, const QString &ticket);
};

#endif // INETWORKMANAGER_H
