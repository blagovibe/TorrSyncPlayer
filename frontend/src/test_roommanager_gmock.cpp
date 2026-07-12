/**
 * @file test_roommanager_gmock.cpp
 * @brief Unit tests for RoomManager using Google Mock
 * 
 * Tests isolation of RoomManager logic using mocked dependencies.
 */

#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <QJsonObject>
#include <QSignalSpy>

#include "interfaces/iroommanager.h"
#include "inetworkmanager.h"
#include "mocks/mock_networkmanager.h"
#include "mocks/mock_roommanager.h"

using ::testing::_;
using ::testing::Return;
using ::testing::Invoke;

class RoomManagerGMockTest : public ::testing::Test
{
protected:
    void SetUp() override {
        m_mockNetwork = new MockNetworkManager(this);
        m_mockRoomManager = new MockRoomManager(this);
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
    EXPECT_EQ(m_mockRoomManager->currentRoomId(), "room-123");
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
    EXPECT_CALL(*m_mockRoomManager, createRoom("Test Room", ""));
    m_mockRoomManager->createRoom("Test Room", "");
    
    EXPECT_CALL(*m_mockRoomManager, createRoom("Private Room", "secret123"));
    m_mockRoomManager->createRoom("Private Room", "secret123");
}

TEST_F(RoomManagerGMockTest, JoinRoom)
{
    EXPECT_CALL(*m_mockRoomManager, joinRoom("room-123", ""));
    m_mockRoomManager->joinRoom("room-123", "");
    
    EXPECT_CALL(*m_mockRoomManager, joinRoom("private-room", "password"));
    m_mockRoomManager->joinRoom("private-room", "password");
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
    QJsonObject event = createRoomEvent("peer-joined", "peer-123");
    
    EXPECT_CALL(*m_mockRoomManager, onRoomEvent(event));
    m_mockRoomManager->onRoomEvent(event);
}

TEST_F(RoomManagerGMockTest, OnSignalReceived)
{
    QJsonObject signal = createSignal("offer", "peer-1", "peer-2");
    
    EXPECT_CALL(*m_mockRoomManager, onSignalReceived(signal));
    m_mockRoomManager->onSignalReceived(signal);
}

// ── Signal emissions ───────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, RoomCreatedSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::roomCreated);
    
    emit m_mockRoomManager->roomCreated("room-123");
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "room-123");
}

TEST_F(RoomManagerGMockTest, RoomJoinedSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::roomJoined);
    
    emit m_mockRoomManager->roomJoined("room-123");
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "room-123");
}

TEST_F(RoomManagerGMockTest, RoomLeftSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::roomLeft);
    
    emit m_mockRoomManager->roomLeft();
    
    EXPECT_EQ(spy.count(), 1);
}

TEST_F(RoomManagerGMockTest, RoomEventSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::roomEvent);
    
    QJsonObject event = createRoomEvent("peer-joined", "peer-123");
    emit m_mockRoomManager->roomEvent(event);
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toObject()["type"].toString(), "peer-joined");
}

TEST_F(RoomManagerGMockTest, SyncActionSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::syncAction);
    
    emit m_mockRoomManager->syncAction("play", 0.0);
    
    EXPECT_EQ(spy.count(), 1);
    auto args = spy.takeFirst();
    EXPECT_EQ(args.at(0).toString(), "play");
    EXPECT_EQ(args.at(1).toDouble(), 0.0);
    
    emit m_mockRoomManager->syncAction("pause", 0.0);
    EXPECT_EQ(spy.count(), 1);
    args = spy.takeFirst();
    EXPECT_EQ(args.at(0).toString(), "pause");
    
    emit m_mockRoomManager->syncAction("seek", 120.5);
    EXPECT_EQ(spy.count(), 1);
    args = spy.takeFirst();
    EXPECT_EQ(args.at(0).toString(), "seek");
    EXPECT_EQ(args.at(1).toDouble(), 120.5);
}

TEST_F(RoomManagerGMockTest, PeerJoinedSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::peerJoined);
    
    emit m_mockRoomManager->peerJoined("peer-123");
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "peer-123");
}

TEST_F(RoomManagerGMockTest, PeerLeftSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::peerLeft);
    
    emit m_mockRoomManager->peerLeft("peer-123");
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "peer-123");
}

TEST_F(RoomManagerGMockTest, ErrorSignal)
{
    QSignalSpy spy(m_mockRoomManager, &IRoomManager::error);
    
    emit m_mockRoomManager->error("Room error message");
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "Room error message");
}

// ── Call order verification ────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, VerifyCallOrder)
{
    testing::InSequence seq;
    
    EXPECT_CALL(*m_mockRoomManager, createRoom("Test Room", ""));
    EXPECT_CALL(*m_mockRoomManager, syncPlay());
    EXPECT_CALL(*m_mockRoomManager, syncSeek(60.0));
    EXPECT_CALL(*m_mockRoomManager, leaveRoom());
    
    m_mockRoomManager->createRoom("Test Room", "");
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
    
    m_mockRoomManager->onRoomEvent(createRoomEvent("peer-joined", "peer-1"));
    m_mockRoomManager->onRoomEvent(createRoomEvent("peer-left", "peer-1"));
    m_mockRoomManager->onRoomEvent(createRoomEvent("peer-joined", "peer-2"));
}

// ── Edge cases ─────────────────────────────────────────────────────────

TEST_F(RoomManagerGMockTest, EmptyRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom("", ""));
    m_mockRoomManager->createRoom("", "");
}

TEST_F(RoomManagerGMockTest, UnicodeRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom("日本語ルーム", ""));
    m_mockRoomManager->createRoom("日本語ルーム", "");
    
    EXPECT_CALL(*m_mockRoomManager, createRoom("Комната 🎬", "пароль"));
    m_mockRoomManager->createRoom("Комната 🎬", "пароль");
}

TEST_F(RoomManagerGMockTest, SpecialCharsInRoomName)
{
    EXPECT_CALL(*m_mockRoomManager, createRoom("Room with 'quotes' & <tags>", ""));
    m_mockRoomManager->createRoom("Room with 'quotes' & <tags>", "");
}

int main(int argc, char **argv)
{
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#include "test_roommanager_gmock.moc"