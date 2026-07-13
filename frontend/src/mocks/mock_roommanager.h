/**
 * @file mock_roommanager.h
 * @brief Mock реализация IRoomManager для gmock тестов
 */

#ifndef MOCK_ROOMMANAGER_H
#define MOCK_ROOMMANAGER_H

#include "../interfaces/iroommanager.h"
#include <gmock/gmock.h>

/**
 * @class MockRoomManager
 * @brief Mock для IRoomManager использующий gmock
 */
class MockRoomManager : public IRoomManager
{
    Q_OBJECT

public:
    MockRoomManager(QObject *parent = nullptr) : IRoomManager(parent) {}
    ~MockRoomManager() override = default;

    MOCK_METHOD(bool, isInRoom, (), (const, override));
    MOCK_METHOD(QString, currentRoomId, (), (const, override));
    MOCK_METHOD(bool, isHost, (), (const, override));
    MOCK_METHOD(void, setHost, (bool host), (override));

    MOCK_METHOD(void, createRoom, (const QString &name, const QString &password), (override));
    MOCK_METHOD(void, joinRoom, (const QString &roomId, const QString &password), (override));
    MOCK_METHOD(void, leaveRoom, (), (override));
    MOCK_METHOD(void, syncPlay, (), (override));
    MOCK_METHOD(void, syncPause, (), (override));
    MOCK_METHOD(void, syncSeek, (double position), (override));
    MOCK_METHOD(void, onRoomEvent, (const QJsonObject &event), (override));
    MOCK_METHOD(void, onSignalReceived, (const QJsonObject &signal), (override));
};

#endif // MOCK_ROOMMANAGER_H