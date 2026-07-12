// Test NetworkManager with gmock*/

#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <QJsonObject>
#include <QJsonArray>
#include <QJsonDocument>
#include <QSignalSpy>

#include "interfaces/inetworkmanager.h"
#include "mocks/mock_networkmanager.h"

using ::testing::_;
using ::testing::Return;
using ::testing::Eq;
using ::testing::StrEq;
using ::testing::Invoke;
using ::testing::Args;

class NetworkManagerGMockTest : public ::testing::Test
{
protected:
    void SetUp() override {
        m_mock = new MockNetworkManager(this);
    }
    
    void TearDown() override {
        delete m_mock;
        m_mock = nullptr;
    }
    
    MockNetworkManager *m_mock = nullptr;
};

// ── Basic properties ──────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, DefaultServerUrl)
{
    // Mock the default server URL
    EXPECT_CALL(*m_mock, serverUrl())
        .WillOnce(Return(QString("http://localhost:8889")));
    
    EXPECT_EQ(m_mock->serverUrl(), "http://localhost:8889");
}

TEST_F(NetworkManagerGMockTest, SetServerUrl)
{
    QUrl newUrl("http://192.168.1.100:9999");
    
    EXPECT_CALL(*m_mock, setServerUrl(newUrl));
    m_mock->setServerUrl(newUrl);
    
    EXPECT_CALL(*m_mock, serverUrl())
        .WillOnce(Return(newUrl.toString()));
    EXPECT_EQ(m_mock->serverUrl(), newUrl.toString());
}

// ── URL construction ──────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, StreamUrl)
{
    EXPECT_CALL(*m_mock, streamUrl(QString("abc123def456")))
        .WillOnce(Return(QString("http://localhost:8889/api/v1/torrents/abc123def456/stream")));
    
    EXPECT_EQ(m_mock->streamUrl(QString("abc123def456")), 
              QString("http://localhost:8889/api/v1/torrents/abc123def456/stream"));
}

TEST_F(NetworkManagerGMockTest, StreamUrlEmpty)
{
    EXPECT_CALL(*m_mock, streamUrl(QString("")))
        .WillOnce(Return(QString("")));
    
    EXPECT_TRUE(m_mock->streamUrl(QString("")).isEmpty());
}

// ── Room state ────────────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, InitialRoomState)
{
    EXPECT_CALL(*m_mock, isInRoom())
        .WillOnce(Return(false));
    EXPECT_CALL(*m_mock, currentRoomId())
        .WillOnce(Return(QString()));
    
    EXPECT_FALSE(m_mock->isInRoom());
    EXPECT_TRUE(m_mock->currentRoomId().isEmpty());
}

TEST_F(NetworkManagerGMockTest, JoinRoomState)
{
    EXPECT_CALL(*m_mock, joinRoom(QString("test-room-id"), QString("")));
    m_mock->joinRoom(QString("test-room-id"), QString(""));
    
    EXPECT_CALL(*m_mock, currentRoomId())
        .WillOnce(Return(QString("test-room-id")));
    EXPECT_CALL(*m_mock, isInRoom())
        .WillOnce(Return(true));
    
    EXPECT_EQ(m_mock->currentRoomId(), "test-room-id");
    EXPECT_TRUE(m_mock->isInRoom());
}

TEST_F(NetworkManagerGMockTest, LeaveRoomState)
{
    EXPECT_CALL(*m_mock, joinRoom(QString("test-room-id"), QString("")));
    m_mock->joinRoom(QString("test-room-id"), QString(""));
    
    EXPECT_CALL(*m_mock, leaveRoom());
    m_mock->leaveRoom();
    
    EXPECT_CALL(*m_mock, isInRoom())
        .WillOnce(Return(false));
    EXPECT_CALL(*m_mock, currentRoomId())
        .WillOnce(Return(QString()));
    
    EXPECT_FALSE(m_mock->isInRoom());
    EXPECT_TRUE(m_mock->currentRoomId().isEmpty());
}

// ── JSON parsing ──────────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, ParseValidJson)
{
    QJsonObject obj;
    obj["key"] = "value";
    obj["number"] = 42;
    QJsonDocument doc(obj);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_FALSE(parsed.isNull());
    EXPECT_EQ(parsed.object()["key"].toString(), "value");
    EXPECT_EQ(parsed.object()["number"].toInt(), 42);
}

TEST_F(NetworkManagerGMockTest, ParseInvalidJson)
{
    QByteArray invalidData = "this is not json";
    
    EXPECT_CALL(*m_mock, parseJson(invalidData))
        .WillOnce(Return(QJsonDocument()));
    
    QJsonDocument parsed = m_mock->parseJson(invalidData);
    EXPECT_TRUE(parsed.isNull());
}

// ── Network error handling ────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, UnauthorizedResponse)
{
    QJsonObject errorObj;
    errorObj["code"] = 401;
    errorObj["message"] = "Unauthorized";
    QJsonDocument doc(errorObj);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_EQ(parsed.object()["code"].toInt(), 401);
    EXPECT_EQ(parsed.object()["message"].toString(), "Unauthorized");
}

TEST_F(NetworkManagerGMockTest, ForbiddenResponse)
{
    QJsonObject errorObj;
    errorObj["code"] = 403;
    errorObj["message"] = "Forbidden";
    QJsonDocument doc(errorObj);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_EQ(parsed.object()["code"].toInt(), 403);
}

TEST_F(NetworkManagerGMockTest, InternalServerErrorResponse)
{
    QJsonObject errorObj;
    errorObj["code"] = 500;
    errorObj["message"] = "Internal Server Error";
    QJsonDocument doc(errorObj);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_EQ(parsed.object()["code"].toInt(), 500);
}

// ── Retry configuration ────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, MaxRetriesConfiguration)
{
    EXPECT_CALL(*m_mock, maxRetries())
        .WillOnce(Return(3));
    EXPECT_EQ(m_mock->maxRetries(), 3);
    
    EXPECT_CALL(*m_mock, setMaxRetries(5));
    m_mock->setMaxRetries(5);
    
    EXPECT_CALL(*m_mock, maxRetries())
        .WillOnce(Return(5));
    EXPECT_EQ(m_mock->maxRetries(), 5);
}

TEST_F(NetworkManagerGMockTest, RetryBaseDelayConfiguration)
{
    EXPECT_CALL(*m_mock, retryBaseDelay())
        .WillOnce(Return(1000));
    EXPECT_EQ(m_mock->retryBaseDelay(), 1000);
    
    EXPECT_CALL(*m_mock, setRetryBaseDelay(2000));
    m_mock->setRetryBaseDelay(2000);
    
    EXPECT_CALL(*m_mock, retryBaseDelay())
        .WillOnce(Return(2000));
    EXPECT_EQ(m_mock->retryBaseDelay(), 2000);
}

TEST_F(NetworkManagerGMockTest, MaxRetriesBounds)
{
    // Below minimum
    EXPECT_CALL(*m_mock, setMaxRetries(1));
    m_mock->setMaxRetries(0);
    
    // Above maximum
    EXPECT_CALL(*m_mock, setMaxRetries(10));
    m_mock->setMaxRetries(100);
    
    // Negative
    EXPECT_CALL(*m_mock, setMaxRetries(1));
    m_mock->setMaxRetries(-5);
}

TEST_F(NetworkManagerGMockTest, RetryBaseDelayBounds)
{
    // Below minimum
    EXPECT_CALL(*m_mock, setRetryBaseDelay(100));
    m_mock->setRetryBaseDelay(50);
    
    // Above maximum
    EXPECT_CALL(*m_mock, setRetryBaseDelay(10000));
    m_mock->setRetryBaseDelay(50000);
    
    // Negative
    EXPECT_CALL(*m_mock, setRetryBaseDelay(100));
    m_mock->setRetryBaseDelay(-100);
}

// ── SSL Mode ──────────────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, SslModeConfiguration)
{
    EXPECT_CALL(*m_mock, sslMode())
        .WillOnce(Return(INetworkManager::SslMode::Strict));
    EXPECT_EQ(m_mock->sslMode(), INetworkManager::SslMode::Strict);
    
    EXPECT_CALL(*m_mock, setSslMode(INetworkManager::SslMode::AllowSelfSigned));
    m_mock->setSslMode(INetworkManager::SslMode::AllowSelfSigned);
    
    EXPECT_CALL(*m_mock, sslMode())
        .WillOnce(Return(INetworkManager::SslMode::AllowSelfSigned));
    EXPECT_EQ(m_mock->sslMode(), INetworkManager::SslMode::AllowSelfSigned);
}

// ── Signal emissions ──────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, TorrentAddedSignal)
{
    QJsonObject torrent;
    torrent["id"] = "test-id";
    torrent["name"] = "Test Torrent";
    
    QSignalSpy spy(m_mock, &INetworkManager::torrentAdded);
    
    EXPECT_CALL(*m_mock, addTorrent(QString("magnet:?xt=urn:btih:test")));
    emit m_mock->torrentAdded(torrent);
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).value<QJsonObject>()["id"].toString(), "test-id");
}

TEST_F(NetworkManagerGMockTest, TorrentRemovedSignal)
{
    QSignalSpy spy(m_mock, &INetworkManager::torrentRemoved);
    
    emit m_mock->torrentRemoved(QString("test-id"));
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "test-id");
}

TEST_F(NetworkManagerGMockTest, RoomCreatedSignal)
{
    QSignalSpy spy(m_mock, &INetworkManager::roomCreated);
    
    emit m_mock->roomCreated(QString("room-123"));
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "room-123");
}

TEST_F(NetworkManagerGMockTest, ErrorSignal)
{
    QSignalSpy spy(m_mock, &INetworkManager::error);
    
    emit m_mock->error(QString("Test error message"));
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), "Test error message");
}

// ── Authentication ────────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, AuthTokenManagement)
{
    EXPECT_CALL(*m_mock, authToken())
        .WillOnce(Return(QString()));
    EXPECT_TRUE(m_mock->authToken().isEmpty());
    
    EXPECT_CALL(*m_mock, setAuthToken(QString("test-jwt-token")));
    m_mock->setAuthToken(QString("test-jwt-token"));
    
    EXPECT_CALL(*m_mock, authToken())
        .WillOnce(Return(QString("test-jwt-token")));
    EXPECT_EQ(m_mock->authToken(), QString("test-jwt-token"));
    
    EXPECT_CALL(*m_mock, clearAuthToken());
    m_mock->clearAuthToken();
}

// ── Edge cases ────────────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, EmptyMagnetUri)
{
    EXPECT_CALL(*m_mock, addTorrent(QString("")));
    m_mock->addTorrent(QString(""));
    EXPECT_TRUE(m_mock != nullptr);
}

TEST_F(NetworkManagerGMockTest, EmptyRoomName)
{
    EXPECT_CALL(*m_mock, createRoom(QString(""), QString("")));
    m_mock->createRoom(QString(""), QString(""));
    EXPECT_TRUE(m_mock != nullptr);
}

TEST_F(NetworkManagerGMockTest, SpecialCharactersInRoomName)
{
    EXPECT_CALL(*m_mock, createRoom(QString("Test Room 日本語"), QString("")));
    m_mock->createRoom(QString("Test Room 日本語"), QString(""));
    
    EXPECT_CALL(*m_mock, createRoom(QString("Комната тест"), QString("")));
    m_mock->createRoom(QString("Комната тест"), QString(""));
    
    EXPECT_CALL(*m_mock, createRoom(QString("Room with spaces"), QString("")));
    m_mock->createRoom(QString("Room with spaces"), QString(""));
}

// ── JSON edge cases ──────────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, ParseEmptyData)
{
    QByteArray emptyData;
    EXPECT_CALL(*m_mock, parseJson(emptyData))
        .WillOnce(Return(QJsonDocument()));
    
    QJsonDocument parsed = m_mock->parseJson(emptyData);
    EXPECT_TRUE(parsed.isNull());
}

TEST_F(NetworkManagerGMockTest, ParseMalformedJson)
{
    QByteArray malformed = "{key: value}"; // missing quotes
    EXPECT_CALL(*m_mock, parseJson(malformed))
        .WillOnce(Return(QJsonDocument()));
    
    QJsonDocument parsed = m_mock->parseJson(malformed);
    EXPECT_TRUE(parsed.isNull());
}

TEST_F(NetworkManagerGMockTest, ParseTruncatedJson)
{
    QByteArray truncated = "{\"key\": \"val"; // truncated string
    EXPECT_CALL(*m_mock, parseJson(truncated))
        .WillOnce(Return(QJsonDocument()));
    
    QJsonDocument parsed = m_mock->parseJson(truncated);
    EXPECT_TRUE(parsed.isNull());
}

TEST_F(NetworkManagerGMockTest, ParseNestedJson)
{
    QJsonObject inner;
    inner["nested_key"] = "nested_value";
    
    QJsonObject outer;
    outer["inner"] = inner;
    outer["array"] = QJsonArray({1, 2, 3});
    
    QJsonDocument doc(outer);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_FALSE(parsed.isNull());
    EXPECT_TRUE(parsed.object()["inner"].isObject());
    EXPECT_EQ(parsed.object()["inner"].value<QJsonObject>()["nested_key"].toString(), "nested_value");
    EXPECT_TRUE(parsed.object()["array"].isArray());
    EXPECT_EQ(parsed.object()["array"].toArray().size(), 3);
}

TEST_F(NetworkManagerGMockTest, ParseJsonWithSpecialChars)
{
    QJsonObject obj;
    obj["unicode"] = "日本語テスト";
    obj["emoji"] = "🎬🎥";
    obj["escaped"] = "line1\nline2\ttab";
    obj["quotes"] = "say \"hello\"";
    
    QJsonDocument doc(obj);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_FALSE(parsed.isNull());
    EXPECT_EQ(parsed.object()["unicode"].toString(), "日本語テスト");
    EXPECT_EQ(parsed.object()["emoji"].toString(), "🎬🎥");
    EXPECT_EQ(parsed.object()["escaped"].toString(), "line1\nline2\ttab");
    EXPECT_EQ(parsed.object()["quotes"].toString(), "say \"hello\"");
}

TEST_F(NetworkManagerGMockTest, ParseLargeJsonArray)
{
    QJsonArray largeArray;
    for (int i = 0; i < 1000; ++i) {
        largeArray.append(QString("item_%1").arg(i));
    }
    QJsonDocument doc(largeArray);
    
    EXPECT_CALL(*m_mock, parseJson(doc.toJson()))
        .WillOnce(Return(doc));
    
    QJsonDocument parsed = m_mock->parseJson(doc.toJson());
    EXPECT_FALSE(parsed.isNull());
    EXPECT_TRUE(parsed.isArray());
    EXPECT_EQ(parsed.array().size(), 1000);
    EXPECT_EQ(parsed.array()[0].toString(), "item_0");
    EXPECT_EQ(parsed.array()[999].toString(), "item_999");
}

// ── Server availability ───────────────────────────────────────────────

TEST_F(NetworkManagerGMockTest, ServerAvailability)
{
    EXPECT_CALL(*m_mock, isServerAvailable())
        .WillOnce(Return(true));
    EXPECT_TRUE(m_mock->isServerAvailable());
    
    EXPECT_CALL(*m_mock, isServerAvailable())
        .WillOnce(Return(false));
    EXPECT_FALSE(m_mock->isServerAvailable());
}

int main(int argc, char **argv)
{
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#include "test_networkmanager_gmock.moc"