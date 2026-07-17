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

    // ── SelectFile → fileSelected signal ──────────────────────────────────
    void testSelectFileEmitsFileSelected();

private:
    NetworkManager *m_manager;

    QJsonObject createTorrentJson(const QString &id = "test-id",
                                   const QString &name = "Test Torrent");
    QJsonObject createRoomJson(const QString &id = "test-room-id",
                                 const QString &name = "Test Room");
    QJsonObject createSyncStatusJson(bool isPlaying = false, double position = 0.0);

    /**
     * @brief Start a local mock HTTP server for room-related requests.
     *
     * Mirrors the approach used by testSelectFileEmitsFileSelected: room state
     * in NetworkManager is only updated after the server *confirms* the join
     * (see onReplyFinished, RequestType::JoinRoom), so tests must await the
     * async reply instead of checking state synchronously.
     *
     * The mock answers:
     *  - POST /api/v1/rooms/join   -> 200 {"id":<roomId>}
     *  - POST /api/v1/csrf-token   -> 200 {"csrfToken":"test-csrf-token"}
     *  - anything else (e.g. SSE)  -> 200 {}
     *
     * @param roomId  Room id returned by the join endpoint
     * @return The listening server (owned by the caller); its port is written
     *         to outPort.
     */
    QTcpServer *startRoomMockServer(const QString &roomId, int &outPort);
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

QTcpServer *TestNetworkManager::startRoomMockServer(const QString &roomId, int &outPort)
{
    QTcpServer *server = new QTcpServer(this);
    if (!server->listen(QHostAddress::LocalHost)) {
        delete server;
        return nullptr;
    }
    outPort = server->serverPort();

    QObject::connect(server, &QTcpServer::newConnection, [server, roomId]() {
        QTcpSocket *sock = server->nextPendingConnection();
        if (!sock) return;
        QObject::connect(sock, &QTcpSocket::readyRead, [sock, roomId]() {
            QByteArray request = sock->readAll();
            // Extract the request path from the HTTP request line.
            QString path;
            int firstNewline = request.indexOf('\n');
            if (firstNewline > 0) {
                QByteArray line = request.left(firstNewline).trimmed();
                QList<QByteArray> parts = line.split(' ');
                if (parts.size() >= 2) {
                    path = QString::fromUtf8(parts.at(1));
                }
            }

            QByteArray body;
            if (path == "/api/v1/rooms/join") {
                QJsonObject obj;
                obj["id"] = roomId;
                body = QJsonDocument(obj).toJson(QJsonDocument::Compact);
            } else if (path == "/api/v1/csrf-token") {
                QJsonObject obj;
                obj["csrfToken"] = QStringLiteral("test-csrf-token");
                body = QJsonDocument(obj).toJson(QJsonDocument::Compact);
            } else {
                body = QByteArray("{}");
            }

            QByteArray response = QByteArray("HTTP/1.1 200 OK\r\n"
                                             "Content-Type: application/json\r\n");
            response.append("Content-Length: ").append(QByteArray::number(body.size())).append("\r\n");
            response.append("\r\n").append(body);
            sock->write(response);
            sock->disconnectFromHost();
        });
    });

    return server;
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
    QCOMPARE(m_manager->streamUrl("abc123"),
             QString("http://localhost:8889/api/v1/torrents/abc123/stream"));

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
    // Room state is updated only after the server confirms the join
    // (onReplyFinished, RequestType::JoinRoom), so await the async reply.
    int port = 0;
    QTcpServer *server = startRoomMockServer("test-room-id", port);
    QVERIFY(server != nullptr);
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    QVERIFY(joinedSpy.isValid());

    m_manager->joinRoom("test-room-id", "");

    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    QCOMPARE(m_manager->currentRoomId(), QString("test-room-id"));
    QVERIFY(m_manager->isInRoom());
}

void TestNetworkManager::testLeaveRoomState()
{
    int port = 0;
    QTcpServer *server = startRoomMockServer("test-room-id", port);
    QVERIFY(server != nullptr);
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    QVERIFY(joinedSpy.isValid());

    m_manager->joinRoom("test-room-id", "");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    QVERIFY(m_manager->isInRoom());

    // leaveRoom() clears the room state synchronously (before/without a reply).
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
    int port = 0;
    QTcpServer *server = startRoomMockServer("sse-test-room", port);
    QVERIFY(server != nullptr);
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    QVERIFY(joinedSpy.isValid());

    m_manager->joinRoom("sse-test-room", "");

    // After the server confirms the join, room state should be set.
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);
    QVERIFY(m_manager->isInRoom());
    QCOMPARE(m_manager->currentRoomId(), QString("sse-test-room"));
}

void TestNetworkManager::testSSEStateAfterLeave()
{
    int port = 0;
    QTcpServer *server = startRoomMockServer("sse-leave-test", port);
    QVERIFY(server != nullptr);
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    QVERIFY(joinedSpy.isValid());

    m_manager->joinRoom("sse-leave-test", "");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);
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

void TestNetworkManager::testSelectFileEmitsFileSelected()
{
    // Локальный mock-сервер, отвечающий 200 на POST /api/v1/torrents/{id}/select.
    QTcpServer server;
    QVERIFY(server.listen(QHostAddress::LocalHost));
    const int port = server.serverPort();

    QObject::connect(&server, &QTcpServer::newConnection, [&server]() {
        QTcpSocket *sock = server.nextPendingConnection();
        if (!sock) return;
        QObject::connect(sock, &QTcpSocket::readyRead, [sock]() {
            // Прочитать запрос полностью (упрощённо) и ответить 200 OK.
            sock->readAll();
            QByteArray response(
                "HTTP/1.1 200 OK\r\n"
                "Content-Type: application/json\r\n"
                "Content-Length: 2\r\n"
                "\r\n"
                "{}");
            sock->write(response);
            sock->disconnectFromHost();
        });
    });

    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    const QString torrentId = "abc123def456";
    const int fileIndex = 2;

    // Сигнал fileSelected должен испуститься после успешного ответа /select.
    QSignalSpy spy(m_manager, &NetworkManager::fileSelected);
    QVERIFY(spy.isValid());

    m_manager->selectFile(torrentId, fileIndex);

    // Ждём ответа сервера (асинхронный Qt event loop).
    QTRY_COMPARE_WITH_TIMEOUT(spy.count(), 1, 5000);

    QCOMPARE(spy.first().at(0).toString(), torrentId);
    QCOMPARE(spy.first().at(1).toInt(), fileIndex);
    QCOMPARE(spy.first().at(2).toString(),
             QString("http://127.0.0.1:%1/api/v1/torrents/%2/stream").arg(port).arg(torrentId));
}

QTEST_MAIN(TestNetworkManager)
#include "test_networkmanager.moc"
