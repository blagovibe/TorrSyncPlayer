/**
 * @file iroommanager.h
 * @brief Абстрактный интерфейс для RoomManager
 * 
 * Позволяет подменять реализацию для тестирования через gmock.
 */

#ifndef IROOMMANAGER_H
#define IROOMMANAGER_H

#include <QObject>
#include <QJsonObject>

/**
 * @class IRoomManager
 * @brief Абстрактный интерфейс для управления комнатами синхронизации
 * 
 * Наследуется от QObject для поддержки сигналов/слотов.
 * Реализации должны обеспечивать все методы для работы с комнатами.
 */
class IRoomManager : public QObject
{
    Q_OBJECT

public:
    explicit IRoomManager(QObject *parent = nullptr) : QObject(parent) {}
    virtual ~IRoomManager() = default;

    /**
     * @brief Проверить, находится ли пользователь в комнате
     * @return true если в комнате
     */
    virtual bool isInRoom() const = 0;
    
    /**
     * @brief Получить ID текущей комнаты
     * @return ID комнаты
     */
    virtual QString currentRoomId() const = 0;
    
    /**
     * @brief Проверить, является ли пользователь хостом
     * @return true если хост
     */
    virtual bool isHost() const = 0;
    
    /**
     * @brief Установить флаг хоста
     * @param host true если хост
     */
    virtual void setHost(bool host) = 0;

public slots:
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
    
    /**
     * @brief Обработка события комнаты
     * @param event JSON объект события
     */
    virtual void onRoomEvent(const QJsonObject &event) = 0;
    
    /**
     * @brief Обработка сигнала от пира
     * @param signal JSON объект сигнала
     */
    virtual void onSignalReceived(const QJsonObject &signal) = 0;

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
};

#endif // IROOMMANAGER_H