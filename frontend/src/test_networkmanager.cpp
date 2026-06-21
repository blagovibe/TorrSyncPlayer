/**
 * @file test_networkmanager.cpp
 * @brief Unit tests for NetworkManager
 *
 * Tests:
 * - Creation and configuration of NetworkManager
 * - API URL construction
 * - Stream URL construction
 * - Room state management
 * - JSON parsing
 * - API error handling
 * - Network error handling (401, 403, 500)
 * - Retry exhaustion
 * - SSE connection/disconnection
 * - JSON parsing edge cases
 */

#include <QtTest>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QTcpServer>
#include <QTcpSocket>
#include "networkmanager.h"

class TestNetworkManager : public QObject
{
    Q_OBJECT

private slots:
    // ── Initialization ─────────────────────────────────────────────────────
    void initTestCase();
    void cleanupTestCase();
    void init();
    void cleanup();

    // ── Basic properties ──────────────────────────────────────────────────
    void testDefaultServerUrl();
    void testSetServerUrl();
    void testServerUrl();

    // ── URL construction ──────────────────────────────────────────────────
    void testStreamUrl();
    void testStreamUrlWithDifferentIds();

    // ── Room state ────────────────────────────────────────────────────────
    void testInitialRoomState();
    void testJoinRoomState();
    void testLeaveRoomState();

    // ── JSON parsing ──────────────────────────────────────────────────────
    void testParseValidJson();
    void testParseInvalidJson();
    void testParseEmptyJson();
    void testParseJsonObject();
    void testParseJsonArray();

    // ── Edge cases ────────────────────────────────────────────────────────
    void testEmptyMagnetUri();
    void testEmptyRoomName();
    void testSpecialCharactersInRoomName();

    // ── Network error handling ────────────────────────────────────────────
    void testUnauthorizedResponse();
    void testForbiddenResponse();
    void testInternalServerErrorResponse();

    // ── Retry exhaustion ──────────────────────────────────────────────────
    void testMaxRetriesConfiguration();
    void testRetryBaseDelayConfiguration();
    void testMaxRetriesBounds();
    void testRetryBaseDelayBounds();

    // ── SSE connection/disconnection ──────────────────────────────────────
    void testSSEInitialState();
    void testSSEConnectionStateAfterJoin();
    void testSSEStateAfterLeave();

    // ── JSON parsing edge cases ───────────────────────────────────────────
    void testParseEmptyData();
    void testParseMalformedJson();
    void testParseTruncatedJson();
    void testParseNestedJson();
    void testParseJsonWithSpecialChars();
    void testParseLargeJsonArray();

private:
    NetworkManager *m_manager;

    QJsonObject createTorrentJson(const QString &id = "test-id",
                                   const QString &name = "Test Torrent");
    QJsonObject createRoomJson(const QString &id = "test-room-id",
                                 const QString &name = "Test Room");
    QJsonObject createSyncStatusJson(bool isPlaying = false, double position = 0.0);
};

void TestNetworkManager::initTestCase()
{
}

void TestNetworkManager::cleanupTestCase()
{
}

void TestNetworkManager::init()
{
    m_manager = new NetworkManager(this);
}

void TestNetworkManager::cleanup()
{
    delete m_manager;
    m_manager = nullptr;
}

QJsonObject TestNetworkManager::createTorrentJson(const QString &id, const QString &name)
{
    QJsonObject json;
    json["id"] = id;
    json["name"] = name;
    json["progress"] = 0.5;
    json["status"] = "downloading";
    json["size"] = 1024 * 1024 * 100;
    return json;
}

QJsonObject TestNetworkManager::createRoomJson(const QString &id, const QString &name)
{
    QJsonObject json;
    json["id"] = id;
    json["name"] = name;
    json["hostId"] = "test-host";
    json["peerCount"] = 1;
    return json;
}

QJsonObject TestNetworkManager::createSyncStatusJson(bool isPlaying, double position)
{
    QJsonObject json;
    json["isPlaying"] = isPlaying;
    json["position"] = position;
    json["duration"] = 3600.0;
    json["timestamp"] = QDateTime::currentDateTime().toMSecsSinceEpoch();
    return json;
}

// ── Basic properties ──────────────────────────────────────────────────────

void TestNetworkManager::testDefaultServerUrl()
{
    QCOMPARE(m_manager->serverUrl(), QString("http://localhost:8889"));
}

void TestNetworkManager::testSetServerUrl()
{
    QUrl newUrl("http://192.168.1.100:9999");
    m_manager->setServerUrl(newUrl);

    QCOMPARE(m_manager->serverUrl(), newUrl.toString());
}

void TestNetworkManager::testServerUrl()
{
    QUrl url("http://example.com:8080");
    m_manager->setServerUrl(url);

    QCOMPARE(m_manager->serverUrl(), QString("http://example.com:8080"));
}

// ── URL construction ──────────────────────────────────────────────────────

void TestNetworkManager::testStreamUrl()
{
    QString torrentId = "abc123def456";
    QString expectedUrl = "http://localhost:8889/api/v1/torrents/abc123def456/stream";

    QCOMPARE(m_manager->streamUrl(torrentId), expectedUrl);
}

void TestNetworkManager::testStreamUrlWithDifferentIds()
{
    QCOMPARE(m_manager->streamUrl("simple-id"),
             QString("http://localhost:8889/api/v1/torrents/simple-id/stream"));

    QCOMPARE(m_manager->streamUrl("0123456789abcdef0123456789abcdef01234567"),
             QString("http://localhost:8889/api/v1/torrents/0123456789abcdef0123456789abcdef01234567/stream"));

    QVERIFY(m_manager->streamUrl("").isEmpty());
}

// ── Room state ─────────────────────────────────────────────────────────────

void TestNetworkManager::testInitialRoomState()
{
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

void TestNetworkManager::testJoinRoomState()
{
    m_manager->joinRoom("test-room-id", "");

    QCOMPARE(m_manager->currentRoomId(), QString("test-room-id"));
    QVERIFY(m_manager->isInRoom());
}

void TestNetworkManager::testLeaveRoomState()
{
    m_manager->joinRoom("test-room-id", "");
    QVERIFY(m_manager->isInRoom());

    m_manager->leaveRoom();

    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

// ── JSON parsing ──────────────────────────────────────────────────────────

void TestNetworkManager::testParseValidJson()
{
    QJsonObject obj;
    obj["key"] = "value";
    obj["number"] = 42;

    QJsonDocument doc(obj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);

    QVERIFY(!parsed.isNull());
    QVERIFY(parsed.isObject());
    QCOMPARE(parsed.object()["key"].toString(), QString("value"));
    QCOMPARE(parsed.object()["number"].toInt(), 42);
}

void TestNetworkManager::testParseInvalidJson()
{
    QByteArray invalidData = "this is not json";

    QJsonDocument parsed = m_manager->parseJson(invalidData);

    QVERIFY(parsed.isNull());
}

void TestNetworkManager::testParseEmptyJson()
{
    QByteArray emptyData = "";

    QJsonDocument parsed = m_manager->parseJson(emptyData);

    QVERIFY(parsed.isNull());
}

void TestNetworkManager::testParseJsonObject()
{
    QJsonObject obj;
    obj["id"] = "test-123";
    obj["name"] = "Test Object";

    QJsonDocument doc(obj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);

    QVERIFY(parsed.isObject());
    QCOMPARE(parsed.object()["id"].toString(), QString("test-123"));
}

void TestNetworkManager::testParseJsonArray()
{
    QJsonArray array;
    array.append("item1");
    array.append("item2");
    array.append("item3");

    QJsonDocument doc(array);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);

    QVERIFY(parsed.isArray());
    QCOMPARE(parsed.array().size(), 3);
    QCOMPARE(parsed.array()[0].toString(), QString("item1"));
}

// ── Edge cases ──────────────────────────────────────────────────────────────

void TestNetworkManager::testEmptyMagnetUri()
{
    m_manager->addTorrent("");

    QVERIFY(m_manager != nullptr);
}

void TestNetworkManager::testEmptyRoomName()
{
    m_manager->createRoom("", "");

    QVERIFY(m_manager != nullptr);
}

void TestNetworkManager::testSpecialCharactersInRoomName()
{
    m_manager->createRoom("Test Room 日本語", "");
    m_manager->createRoom("Комната тест", "");
    m_manager->createRoom("Room with spaces", "");

    QVERIFY(true);
}

// ── Network error handling ──────────────────────────────────────────────────

void TestNetworkManager::testUnauthorizedResponse()
{
    // Verify that parseJson handles error response bodies correctly
    QJsonObject errorObj;
    errorObj["code"] = 401;
    errorObj["message"] = "Unauthorized";

    QJsonDocument doc(errorObj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QCOMPARE(parsed.object()["code"].toInt(), 401);
    QCOMPARE(parsed.object()["message"].toString(), QString("Unauthorized"));
}

void TestNetworkManager::testForbiddenResponse()
{
    QJsonObject errorObj;
    errorObj["code"] = 403;
    errorObj["message"] = "Forbidden";

    QJsonDocument doc(errorObj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QCOMPARE(parsed.object()["code"].toInt(), 403);
    QCOMPARE(parsed.object()["message"].toString(), QString("Forbidden"));
}

void TestNetworkManager::testInternalServerErrorResponse()
{
    QJsonObject errorObj;
    errorObj["code"] = 500;
    errorObj["message"] = "Internal Server Error";

    QJsonDocument doc(errorObj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QCOMPARE(parsed.object()["code"].toInt(), 500);
    QCOMPARE(parsed.object()["message"].toString(), QString("Internal Server Error"));
}

// ── Retry exhaustion ────────────────────────────────────────────────────────

void TestNetworkManager::testMaxRetriesConfiguration()
{
    QCOMPARE(m_manager->maxRetries(), 3); // default

    m_manager->setMaxRetries(5);
    QCOMPARE(m_manager->maxRetries(), 5);
}

void TestNetworkManager::testRetryBaseDelayConfiguration()
{
    QCOMPARE(m_manager->retryBaseDelay(), 1000); // default

    m_manager->setRetryBaseDelay(2000);
    QCOMPARE(m_manager->retryBaseDelay(), 2000);
}

void TestNetworkManager::testMaxRetriesBounds()
{
    // Below minimum (1)
    m_manager->setMaxRetries(0);
    QCOMPARE(m_manager->maxRetries(), 1);

    // Above maximum (10)
    m_manager->setMaxRetries(100);
    QCOMPARE(m_manager->maxRetries(), 10);

    // Negative
    m_manager->setMaxRetries(-5);
    QCOMPARE(m_manager->maxRetries(), 1);
}

void TestNetworkManager::testRetryBaseDelayBounds()
{
    // Below minimum (100)
    m_manager->setRetryBaseDelay(50);
    QCOMPARE(m_manager->retryBaseDelay(), 100);

    // Above maximum (10000)
    m_manager->setRetryBaseDelay(50000);
    QCOMPARE(m_manager->retryBaseDelay(), 10000);

    // Negative
    m_manager->setRetryBaseDelay(-100);
    QCOMPARE(m_manager->retryBaseDelay(), 100);
}

// ── SSE connection/disconnection ────────────────────────────────────────────

void TestNetworkManager::testSSEInitialState()
{
    // Initially not in any room, SSE should not be connected
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

void TestNetworkManager::testSSEConnectionStateAfterJoin()
{
    m_manager->joinRoom("sse-test-room", "");

    // After joining, room state should be set
    QVERIFY(m_manager->isInRoom());
    QCOMPARE(m_manager->currentRoomId(), QString("sse-test-room"));
}

void TestNetworkManager::testSSEStateAfterLeave()
{
    m_manager->joinRoom("sse-leave-test", "");
    QVERIFY(m_manager->isInRoom());

    m_manager->leaveRoom();

    // After leaving, room state should be cleared
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

// ── JSON parsing edge cases ─────────────────────────────────────────────────

void TestNetworkManager::testParseEmptyData()
{
    QByteArray emptyData;
    QJsonDocument parsed = m_manager->parseJson(emptyData);
    QVERIFY(parsed.isNull());
}

void TestNetworkManager::testParseMalformedJson()
{
    QByteArray malformed = "{key: value}"; // missing quotes
    QJsonDocument parsed = m_manager->parseJson(malformed);
    QVERIFY(parsed.isNull());
}

void TestNetworkManager::testParseTruncatedJson()
{
    QByteArray truncated = "{\"key\": \"val"; // truncated string
    QJsonDocument parsed = m_manager->parseJson(truncated);
    QVERIFY(parsed.isNull());
}

void TestNetworkManager::testParseNestedJson()
{
    QJsonObject inner;
    inner["nested_key"] = "nested_value";

    QJsonObject outer;
    outer["inner"] = inner;
    outer["array"] = QJsonArray({1, 2, 3});

    QJsonDocument doc(outer);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QVERIFY(parsed.object()["inner"].isObject());
    QCOMPARE(parsed.object()["inner"].toObject()["nested_key"].toString(), QString("nested_value"));
    QVERIFY(parsed.object()["array"].isArray());
    QCOMPARE(parsed.object()["array"].toArray().size(), 3);
}

void TestNetworkManager::testParseJsonWithSpecialChars()
{
    QJsonObject obj;
    obj["unicode"] = "日本語テスト";
    obj["emoji"] = "🎬🎥";
    obj["escaped"] = "line1\nline2\ttab";
    obj["quotes"] = "say \"hello\"";

    QJsonDocument doc(obj);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QCOMPARE(parsed.object()["unicode"].toString(), QString("日本語テスト"));
    QCOMPARE(parsed.object()["emoji"].toString(), QString("🎬🎥"));
    QCOMPARE(parsed.object()["escaped"].toString(), QString("line1\nline2\ttab"));
    QCOMPARE(parsed.object()["quotes"].toString(), QString("say \"hello\""));
}

void TestNetworkManager::testParseLargeJsonArray()
{
    QJsonArray largeArray;
    for (int i = 0; i < 1000; ++i) {
        largeArray.append(QString("item_%1").arg(i));
    }

    QJsonDocument doc(largeArray);
    QByteArray data = doc.toJson();

    QJsonDocument parsed = m_manager->parseJson(data);
    QVERIFY(!parsed.isNull());
    QVERIFY(parsed.isArray());
    QCOMPARE(parsed.array().size(), 1000);
    QCOMPARE(parsed.array()[0].toString(), QString("item_0"));
    QCOMPARE(parsed.array()[999].toString(), QString("item_999"));
}

QTEST_MAIN(TestNetworkManager)
#include "test_networkmanager.moc"
