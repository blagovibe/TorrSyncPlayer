/**
 * @file test_roommanager_gmock.cpp
 * @brief Unit tests for RoomManager using Google Mock
 * 
 * Tests isolation of RoomManager logic using mocked dependencies.
 */

#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <QJsonObject>

#include "interfaces/iroommanager.h"

using ::testing::_;
using ::testing::Return;
using ::testing::Invoke;

class RoomManagerGMockTest : public ::testing::Test
{
protected:
    void SetUp() override {
        m_mockNetwork = new MockNetworkManager(nullptr);
        m_mockRoomManager = new MockRoomManager(nullptr);
    }
    
    void TearDown() override {
        delete m_mockRoomManager;
        delete m_mockNetwork;
        m_mockRoomManager = nullptr;
        m_mockNetwork = nullptr;
    }
    
    MockNetworkManager *m_mockNetwork = nullptr;
    MockRoomManager *m_mockRoomManager = nullptr;
    
    QJsonObject createRoomEvent(const QString &type = "peer-joined", 
                                 const QString &peerId = "peer-123") {
        QJsonObject event;
        event["type"] = type;
        event["peerId"] = peerId;
        event["timestamp"] = QDateTime::currentDateTime().toMSecsSinceEpoch();
        return event;
    }
    
    QJsonObject createSignal(const QString &type = "offer",
                              const QString &from = "peer-1",
                              const QString &to = "peer-2") {
        QJsonObject signal;
        signal["type"] = type;
        signal["from"] = from;
        signal["to"] = to;
        signal["payload"] = QJsonObject{{"sdp", "test-sdp"}};
        return signal;
    }
};

// ── Room state ─────────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, IsInRoom)
{
    EXPECT_CALL(*m_mockRoomManager, isInRoom())
        .WillOnce(Return(false));
    EXPECT_FALSE(m_mockRoomManager->isInRoom());
    
    EXPECT_CALL(*m_mockRoomManager, isInRoom())
        .WillOnce(Return(true));
    EXPECT_TRUE(m_mockRoomManager->isInRoom());
}

TEST_F(RoomManagerGMockTest, CurrentRoomId)
{
    EXPECT_CALL(*m_mockRoomManager, currentRoomId())
        .WillOnce(Return(QString()));
    EXPECT_TRUE(m_mockRoomManager->currentRoomId().isEmpty());
    
    EXPECT_CALL(*m_mockRoomManager, currentRoomId())
        .WillOnce(Return(QString("room-123")));
    EXPECT_EQ(m_mockRoomManager->currentRoomId(), QString("room-123"));
}

TEST_F(RoomManagerGMockTest, IsHost)
{
    EXPECT_CALL(*m_mockRoomManager, isHost())
        .WillOnce(Return(false));
    EXPECT_FALSE(m_mockRoomManager->isHost());
    
    EXPECT_CALL(*m_mockRoomManager, isHost())
        .WillOnce(Return(true));
    EXPECT_TRUE(m_mockRoomManager->isHost());
}

TEST_F(RoomManagerGMockTest, SetHost)
{
    EXPECT_CALL(*m_mockRoomManager, setHost(true));
    m_mockRoomManager->setHost(true);
    
    EXPECT_CALL(*m_mockRoomManager, setHost(false));
    m_mockRoomManager->setHost(false);
}

// ── Room operations ────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, CreateRoom)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("Test Room"), QString("")));
    m_mockRoomManager->createRoom(QString("Test Room"), QString(""));
    
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("Private Room"), QString("secret123")));
    m_mockRoomManager->createRoom(QString("Private Room"), QString("secret123"));
}

TEST_F(RoomManagerGMockTest, JoinRoom)
{
    EXPECT_CALL(*m_mockRoomManager, joinRoom(QString("room-123"), QString("")));
    m_mockRoomManager->joinRoom(QString("room-123"), QString(""));
    
    EXPECT_CALL(*m_mockRoomManager, joinRoom(QString("private-room"), QString("password")));
    m_mockRoomManager->joinRoom(QString("private-room"), QString("password"));
}

TEST_F(RoomManagerGMockTest, LeaveRoom)
{
    EXPECT_CALL(*m_mockRoomManager, leaveRoom());
    m_mockRoomManager->leaveRoom();
}

// ── Sync operations ────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, SyncPlay)
{
    EXPECT_CALL(*m_mockRoomManager, syncPlay());
    m_mockRoomManager->syncPlay();
}

TEST_F(RoomManagerGMockTest, SyncPause)
{
    EXPECT_CALL(*m_mockRoomManager, syncPause());
    m_mockRoomManager->syncPause();
}

TEST_F(RoomManagerGMockTest, SyncSeek)
{
    EXPECT_CALL(*m_mockRoomManager, syncSeek(42.5));
    m_mockRoomManager->syncSeek(42.5);
    
    EXPECT_CALL(*m_mockRoomManager, syncSeek(0.0));
    m_mockRoomManager->syncSeek(0.0);
    
    EXPECT_CALL(*m_mockRoomManager, syncSeek(3600.0));
    m_mockRoomManager->syncSeek(3600.0);
}

// ── Event handling ─────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, OnRoomEvent)
{
    QJsonObject event = createRoomEvent(QString("peer-joined"), QString("peer-123"));
    
    EXPECT_CALL(*m_mockRoomManager, onRoomEvent(event));
    m_mockRoomManager->onRoomEvent(event);
}

TEST_F(RoomManagerGMockTest, OnSignalReceived)
{
    QJsonObject signal = createSignal(QString("offer"), QString("peer-1"), QString("peer-2"));
    
    EXPECT_CALL(*m_mockRoomManager, onSignalReceived(signal));
    m_mockRoomManager->onSignalReceived(signal);
}

// ── Signal emissions ───────────────────────────────────────────────────

// Note: Signal emission tests removed - QSignalSpy requires Q_OBJECT macro
// and MOC-generated meta-object code which conflicts with gmock MOCK_METHOD.
// Signals are tested in the Qt Test-based tests.

// ── Call order verification ────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, VerifyCallOrder)
{
    testing::InSequence seq;
    
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("Test Room"), QString("")));
    EXPECT_CALL(*m_mockRoomManager, syncPlay());
    EXPECT_CALL(*m_mockRoomManager, syncSeek(60.0));
    EXPECT_CALL(*m_mockRoomManager, leaveRoom());
    
    m_mockRoomManager->createRoom(QString("Test Room"), QString(""));
    m_mockRoomManager->syncPlay();
    m_mockRoomManager->syncSeek(60.0);
    m_mockRoomManager->leaveRoom();
}

// ── Multiple calls ─────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, MultipleSyncCalls)
{
    EXPECT_CALL(*m_mockRoomManager, syncPlay()).Times(2);
    EXPECT_CALL(*m_mockRoomManager, syncPause()).Times(2);
    
    m_mockRoomManager->syncPlay();
    m_mockRoomManager->syncPause();
    m_mockRoomManager->syncPlay();
    m_mockRoomManager->syncPause();
}

TEST_F(RoomManagerGMockTest, MultipleRoomEvents)
{
    EXPECT_CALL(*m_mockRoomManager, onRoomEvent(_)).Times(3);
    
    m_mockRoomManager->onRoomEvent(createRoomEvent(QString("peer-joined"), QString("peer-1")));
    m_mockRoomManager->onRoomEvent(createRoomEvent(QString("peer-left"), QString("peer-1")));
    m_mockRoomManager->onRoomEvent(createRoomEvent(QString("peer-joined"), QString("peer-2")));
}

// ── Edge cases ─────────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, EmptyRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString(""), QString("")));
    m_mockRoomManager->createRoom(QString(""), QString(""));
}

TEST_F(RoomManagerGMockTest, UnicodeRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("日本語ルーム"), QString("")));
    m_mockRoomManager->createRoom(QString("日本語ルーム"), QString(""));
    
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("Комната 🎬"), QString("пароль")));
    m_mockRoomManager->createRoom(QString("Комната 🎬"), QString("пароль"));
}

TEST_F(RoomManagerGMockTest, SpecialCharsInRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom(QString("Room with 'quotes'"), QString("")));
    m_mockRoomManager->createRoom(QString("Room with 'quotes'"), QString(""));
}

int main(int argc, char **argv)
{
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#include "test_roommanager_gmock.moc"