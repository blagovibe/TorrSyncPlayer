/**
 * @file roommanager.h
 * @brief Менеджер управления комнатами для TorrPlayer
 * 
 * Вынесен из MainWindow для уменьшения размера класса.
 * Управляет созданием, присоединением и выходом из комнат.
 */

#ifndef ROOMMANAGER_H
#define ROOMMANAGER_H

#include <QObject>
#include <QJsonObject>

// Предварительные объявления
class NetworkManager;

/**
 * @class RoomManager
 * @brief Менеджер операций с комнатами синхронизации
 * 
 * Инкапсулирует логику:
 * - Создание комнаты
 * - Присоединение к комнате
 * - Выход из комнаты
 * - Синхронизация воспроизведения
 * - Обработка событий комнаты
 */
class RoomManager : public QObject
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор менеджера комнат
     * @param network Менеджер сети для API запросов
     * @param parent Родительский объект
     */
    explicit RoomManager(NetworkManager *network, QObject *parent = nullptr);
    
    /**
     * @brief Деструктор
     */
    ~RoomManager();

    /**
     * @brief Проверить, находится ли пользователь в комнате
     * @return true если в комнате
     */
    bool isInRoom() const;
    
    /**
     * @brief Получить ID текущей комнаты
     * @return ID комнаты
     */
    QString currentRoomId() const { return m_currentRoomId; }
    
    /**
     * @brief Проверить, является ли пользователь хостом
     * @return true если хост
     */
    bool isHost() const { return m_isHost; }
    
    /**
     * @brief Установить флаг хоста
     * @param host true если хост
     */
    void setHost(bool host) { m_isHost = host; }

public slots:
    /**
     * @brief Создать новую комнату
     * @param name Имя комнаты
     * @param password Пароль (опционально)
     */
    void createRoom(const QString &name, const QString &password = "");
    
    /**
     * @brief Присоединиться к комнате
     * @param roomId ID комнаты
     * @param password Пароль (опционально)
     */
    void joinRoom(const QString &roomId, const QString &password = "");
    
    /**
     * @brief Покинуть текущую комнату
     */
    void leaveRoom();
    
    /**
     * @brief Синхронизировать воспроизведение (play)
     */
    void syncPlay();
    
    /**
     * @brief Синхронизировать паузу
     */
    void syncPause();
    
    /**
     * @brief Синхронизировать перемотку
     * @param position Позиция в секундах
     */
    void syncSeek(double position);
    
    /**
     * @brief Обработка события комнаты
     * @param event JSON объект события
     */
    void onRoomEvent(const QJsonObject &event);
    
    /**
     * @brief Обработка сигнала от пира
     * @param signal JSON объект сигнала
     */
    void onSignalReceived(const QJsonObject &signal);

signals:
    /**
     * @brief Комната создана
     * @param roomId ID созданной комнаты
     */
    void roomCreated(const QString &roomId);
    
    /**
     * @brief Присоединились к комнате
     * @param roomId ID комнаты
     */
    void roomJoined(const QString &roomId);
    
    /**
     * @brief Покинули комнату
     */
    void roomLeft();
    
    /**
     * @brief Получено событие комнаты
     * @param event JSON объект события
     */
    void roomEvent(const QJsonObject &event);
    
    /**
     * @brief Получен сигнал синхронизации
     * @param action Действие (play/pause/seek)
     * @param position Позиция (для seek)
     */
    void syncAction(const QString &action, double position = 0.0);
    
    /**
     * @brief Пир присоединился к комнате
     * @param peerId ID пира
     */
    void peerJoined(const QString &peerId);
    
    /**
     * @brief Пир покинул комнату
     * @param peerId ID пира
     */
    void peerLeft(const QString &peerId);
    
    /**
     * @brief Ошибка операции с комнатой
     * @param message Описание ошибки
     */
    void error(const QString &message);

private:
    NetworkManager *m_network;      ///< Менеджер сети
    QString m_currentRoomId;        ///< ID текущей комнаты
    bool m_isHost = false;          ///< Флаг хоста комнаты
};

#endif // ROOMMANAGER_H
