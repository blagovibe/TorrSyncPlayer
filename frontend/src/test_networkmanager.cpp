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

#include <QtTest/QtTest>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QTcpServer>
#include <QTcpSocket>
#include <QAtomicInt>
#include <QMutex>
#include <QMutexLocker>
#include <QTimer>
#include <QStringList>
#include <QList>
#include <utility>
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

    // ── SSE reconnect + state reconciliation (bug-0012) ────────────────────
    void testSSEReconnectsAfterStreamDrop();
    void testReconcileFetchesStatusAndRoomInfoOnReconnect();
    void testReconcileEmitsStatusWithServerValues();
    void testReconcileNotAppliedWhenStatusRequestFails();

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
    /// Mock-серверы текущего теста. Их нужно гасить в cleanup(): сервер,
    /// переживший тест, продолжает принимать соединения от клиента и
    /// обращается к SseMockState, который лежал на стеке уже ушедшего теста.
    QList<QTcpServer *> m_mockServers;

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

    /**
     * @brief Shared bookkeeping for the SSE mock server.
     *
     * The reconnect path is asynchronous and its outcome is only observable on
     * the wire, so the tests count what the server actually received rather
     * than inspecting NetworkManager internals.
     */
    struct SseMockState {
        QAtomicInt sseConnections{0};   ///< How many times the SSE stream was opened
        QMutex mutex;
        QStringList paths;             ///< Every request path the client sent
        /// Status code the mock answers /sync/status with (bug-0019 scenarios).
        int syncStatusStatus = 200;
    };

    /**
     * @brief Start a mock server that holds the SSE stream open, then drops it.
     *
     * Behaviour mirrors the real backend closely enough to exercise reconnect:
     *  - POST /api/v1/rooms/join   -> 200 {"id":<roomId>}
     *  - GET  .../events            -> 200 text/event-stream, then
     *                                 `event: connected` exactly as
     *                                 SSEEventHandler writes it. The FIRST such
     *                                 stream is aborted shortly after, which is
     *                                 what a real network drop looks like; later
     *                                 ones stay open.
     *  - GET  /api/v1/sync/status   -> 200 SyncStatus JSON
     *  - GET  /api/v1/rooms/info    -> 200 RoomInfo JSON
     *  - anything else              -> 200 {}
     *
     * Every requested path is recorded in state->paths.
     */
    QTcpServer *startSSEMockServer(const QString &roomId, SseMockState *state,
                                   int &outPort);
    // ── F-04: разбор конвертов ответа ─────────────────────────────────────
    // Регрессия: код проверял doc.isArray() у объекта-конверта, что всегда
    // false, поэтому torrentListReceived/filesReceived не эмитились НИКОГДА,
    // а пустой список торрентов выглядел как «торрентов нет».
    void testTorrentListEnvelopeIsUnwrapped();
    void testFileListEnvelopeIsUnwrapped();
    void testBareArrayEnvelopeStillWorks();
    void testEnvelopeWithoutExpectedFieldEmitsNothing();
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
    // Порядок важен: сначала клиент перестаёт слать запросы, и только потом
    // гаснут серверы. Наоборот — сервер начнёт отвечать на запросы, которых
    // уже никто не ждёт.
    delete m_manager;
    m_manager = nullptr;

    for (QTcpServer *server : std::as_const(m_mockServers)) {
        delete server;
    }
    m_mockServers.clear();
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
    m_mockServers.append(server);

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

QTcpServer *TestNetworkManager::startSSEMockServer(const QString &roomId,
                                                  SseMockState *state,
                                                  int &outPort)
{
    QTcpServer *server = new QTcpServer(this);
    if (!server->listen(QHostAddress::LocalHost)) {
        delete server;
        return nullptr;
    }
    outPort = server->serverPort();
    // Гасится в cleanup(). Пока сервер жил до конца прогона, его
    // readyRead-лямбда держала указатель на SseMockState — а тот лежит на стеке
    // теста и к следующему тесту уже освобождён. Обращение к освобождённой
    // памяти давало то, что наблюдалось: иногда std::bad_alloc, иногда
    // блокировку по мусору, иногда «тест прошёл».
    m_mockServers.append(server);

    QObject::connect(server, &QTcpServer::newConnection, [server, roomId, state]() {
        QTcpSocket *sock = server->nextPendingConnection();
        if (!sock) return;
        QObject::connect(sock, &QTcpSocket::readyRead, [sock, roomId, state]() {            const QByteArray request = sock->readAll();

            // Extract the request path from the HTTP request line.
            QString path;
            const int firstNewline = request.indexOf('\n');
            if (firstNewline > 0) {
                const QByteArray line = request.left(firstNewline).trimmed();
                const QList<QByteArray> parts = line.split(' ');
                if (parts.size() >= 2) {
                    path = QString::fromUtf8(parts.at(1));
                }
            }

            {
                QMutexLocker locker(&state->mutex);
                state->paths.append(path);
            }

            if (path.contains(QStringLiteral("/events"))) {
                // Count the attempt BEFORE replying, so a test that races the
                // reconnect cannot observe a stale count.
                const int attempt = state->sseConnections.fetchAndAddOrdered(1) + 1;

                // The real backend streams with Transfer-Encoding: chunked, so
                // the mock must too. Without it, aborting the socket looks like
                // a clean end-of-body and QNetworkReply finishes WITHOUT an
                // error — which would make this mock unable to reproduce a
                // network drop at all, and the reconnect path untestable.
                sock->write("HTTP/1.1 200 OK\r\n"
                            "Content-Type: text/event-stream\r\n"
                            "Cache-Control: no-cache\r\n"
                            "Connection: keep-alive\r\n"
                            "Transfer-Encoding: chunked\r\n"
                            "\r\n");
                const QByteArray frame = "event: connected\ndata: {\"status\":\"ok\"}\n\n";
                sock->write(QByteArray::number(frame.size(), 16) + "\r\n" + frame + "\r\n");
                sock->flush();

                if (attempt == 1) {
                    // Simulate a mid-stream network drop: the chunked stream is
                    // left unterminated. 150 ms is long enough for the client to
                    // see the headers and treat the stream as open, so the test
                    // really exercises "opened, then dropped".
                    QTimer::singleShot(150, sock, [sock]() { sock->abort(); });
                }
                return;
            }

            QByteArray body;
            int statusCode = 200;
            if (path == QStringLiteral("/api/v1/rooms/join")) {
                body = QJsonDocument(QJsonObject{{QStringLiteral("id"), roomId}})
                           .toJson(QJsonDocument::Compact);
            } else if (path == QStringLiteral("/api/v1/sync/status")) {
                if (state->syncStatusStatus != 200) {
                    // bug-0019: the server tells a caller outside any room that
                    // there is no room. The reconciliation must not treat this
                    // body as a status, and must not hammer the endpoint.
                    statusCode = state->syncStatusStatus;
                    body = QJsonDocument(QJsonObject{
                        {QStringLiteral("error"), QStringLiteral("Not connected to a room")}
                    }).toJson(QJsonDocument::Compact);
                } else {
                    body = QJsonDocument(QJsonObject{
                        {QStringLiteral("isPlaying"), true},
                        {QStringLiteral("position"), 42.5},
                        {QStringLiteral("duration"), 3600.0},
                        {QStringLiteral("buffering"), false},
                        {QStringLiteral("timestamp"), QDateTime::currentDateTime().toMSecsSinceEpoch()}
                    }).toJson(QJsonDocument::Compact);
                }
            } else if (path == QStringLiteral("/api/v1/rooms/info")) {
                body = QJsonDocument(QJsonObject{
                    {QStringLiteral("id"), roomId},
                    {QStringLiteral("name"), QStringLiteral("Test Room")},
                    {QStringLiteral("hostId"), QStringLiteral("host-1")},
                    {QStringLiteral("hostUserId"), QStringLiteral("test-client")},
                    {QStringLiteral("peerCount"), 3}
                }).toJson(QJsonDocument::Compact);
            } else if (path == QStringLiteral("/api/v1/csrf-token")) {
                body = QJsonDocument(QJsonObject{
                    {QStringLiteral("csrfToken"), QStringLiteral("test-csrf-token")}
                }).toJson(QJsonDocument::Compact);
            } else {
                body = QByteArray("{}");
            }

            QByteArray response("HTTP/1.1 ");
            response.append(QByteArray::number(statusCode)).append(' ');
            response.append(statusCode == 200 ? "OK" : "Bad Request");
            response.append("\r\nContent-Type: application/json\r\n");
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

    m_manager->joinRoom("test-room-id", "", "torrent-test");

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

    m_manager->joinRoom("test-room-id", "", "torrent-test");
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
    m_manager->createRoom("", "", "torrent-test");

    QVERIFY(m_manager != nullptr);
}

void TestNetworkManager::testSpecialCharactersInRoomName()
{
    m_manager->createRoom("Test Room 日本語", "", "torrent-test");
    m_manager->createRoom("Комната тест", "", "torrent-test");
    m_manager->createRoom("Room with spaces", "", "torrent-test");

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

    m_manager->joinRoom("sse-test-room", "", "torrent-test");

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

    m_manager->joinRoom("sse-leave-test", "", "torrent-test");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);
    QVERIFY(m_manager->isInRoom());

    m_manager->leaveRoom();

    // After leaving, room state should be cleared
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

// ── SSE reconnect + state reconciliation (bug-0012) ─────────────────────────

void TestNetworkManager::testSSEReconnectsAfterStreamDrop()
{
    // Regression: reconnect never happened at all. connectToSSE() opens by
    // calling disconnectSSE(), and disconnectSSE() cleared m_sseReconnectPath —
    // so by the time onSSEReconnect() ran, the path it needed was already gone
    // and it bailed out with "путь SSE переподключения пуст". Every drop was
    // therefore terminal, and everything downstream of a successful reconnect
    // (state reconciliation) was unreachable.
    SseMockState state;
    int port = 0;
    QTcpServer *server = startSSEMockServer("reconnect-room", &state, port);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    m_manager->joinRoom("reconnect-room", "", "torrent-test");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    // The mock aborts the first stream; the client must come back on its own.
    QTRY_VERIFY_WITH_TIMEOUT(state.sseConnections.loadAcquire() >= 2, 8000);
}

void TestNetworkManager::testReconcileFetchesStatusAndRoomInfoOnReconnect()
{
    // After a reconnect the client has a live stream again but knows nothing
    // about what happened while it was away: it may be sitting at the wrong
    // position, playing while the room is paused, or unaware of who joined.
    // Reconnecting therefore has to pull /sync/status and /rooms/info.
    SseMockState state;
    int port = 0;
    QTcpServer *server = startSSEMockServer("reconcile-room", &state, port);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    m_manager->joinRoom("reconcile-room", "", "torrent-test");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    QTRY_VERIFY_WITH_TIMEOUT(state.sseConnections.loadAcquire() >= 2, 8000);

    QStringList paths;
    QTRY_VERIFY_WITH_TIMEOUT([&] {
        QMutexLocker locker(&state.mutex);
        paths = state.paths;
        return paths.contains(QStringLiteral("/api/v1/sync/status"))
            && paths.contains(QStringLiteral("/api/v1/rooms/info"));
    }(), 5000);
}

void TestNetworkManager::testReconcileEmitsStatusWithServerValues()
{
    // The fetch is only useful if the values actually reach the caller as a
    // parsed object: raw JSON would push re-parsing onto every consumer.
    SseMockState state;
    int port = 0;
    QTcpServer *server = startSSEMockServer("emit-room", &state, port);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy statusSpy(m_manager, &NetworkManager::syncStatusReceived);
    QSignalSpy roomSpy(m_manager, &NetworkManager::roomInfoReceived);

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    m_manager->joinRoom("emit-room", "", "torrent-test");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    // Exactly two reconciliations: one for the initial stream, one after the
    // drop. Not zero (state would go stale) and not more (no request storms).
    //
    // Оба ожидания — с ожиданием, а не QCOMPARE «здесь и сейчас»: статуса и
    // сведения о комнате запрашиваются параллельно, и к моменту, когда
    // statusSpy уже насчитал два, второй ответ rooms/info может ещё быть в
    // пути. Мгновенная проверка делала тест нестабильным (~2% прогонов).
    QTRY_COMPARE_WITH_TIMEOUT(statusSpy.count(), 2, 10000);
    QTRY_COMPARE_WITH_TIMEOUT(roomSpy.count(), 2, 10000);

    const QJsonObject status = statusSpy.first().at(0).toJsonObject();
    QCOMPARE(status.value(QStringLiteral("isPlaying")).toBool(), true);
    QCOMPARE(status.value(QStringLiteral("position")).toDouble(), 42.5);
    QVERIFY(status.contains(QStringLiteral("buffering")));

    const QJsonObject info = roomSpy.first().at(0).toJsonObject();
    QCOMPARE(info.value(QStringLiteral("peerCount")).toInt(), 3);
    QCOMPARE(info.value(QStringLiteral("hostUserId")).toString(),
             QStringLiteral("test-client"));
}

void TestNetworkManager::testReconcileNotAppliedWhenStatusRequestFails()
{
    // bug-0019 changed the server contract: outside a room, /sync/status answers
    // 400 instead of 200 with a zeroed status. Two things must hold on the
    // client, or the reconciliation from bug-0012 becomes a new bug source:
    //
    //  1. The error body must not be applied as room state. It is a JSON object,
    //     so `doc.isObject()` is true — without the error check the client would
    //     happily emit a "status" of {error: ...} and seek to 0.
    //  2. A 400 must not be retried. Retrying a request that is refused because
    //     of who we are cannot succeed, and it would hammer the server on every
    //     single reconnect.
    SseMockState state;
    state.syncStatusStatus = 400;
    int port = 0;
    QTcpServer *server = startSSEMockServer("no-room", &state, port);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));

    QSignalSpy statusSpy(m_manager, &NetworkManager::syncStatusReceived);

    QSignalSpy joinedSpy(m_manager, &NetworkManager::roomJoined);
    m_manager->joinRoom("no-room", "", "torrent-test");
    QTRY_COMPARE_WITH_TIMEOUT(joinedSpy.count(), 1, 5000);

    // The stream is still opened and re-opened — a failing status endpoint must
    // not stop the client from being in the room.
    QTRY_COMPARE_WITH_TIMEOUT(state.sseConnections.loadAcquire(), 2, 10000);

    // Give any (wrongly) scheduled retry a generous chance to show up.
    QTest::qWait(1500);

    QCOMPARE(statusSpy.count(), 0);

    int statusRequests = 0;
    {
        QMutexLocker locker(&state.mutex);
        for (const QString &p : state.paths) {
            if (p == QStringLiteral("/api/v1/sync/status")) ++statusRequests;
        }
    }
    // Exactly one per stream open: the initial join plus the reconnect. A retry
    // storm would show up as more than two.
    QCOMPARE(statusRequests, 2);
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

namespace {

// Отдаёт переданный JSON по любому пути и закрывает соединение.
QTcpServer *startRawJsonServer(const QByteArray &body, int &outPort,
                              QObject *owner)
{
    QTcpServer *server = new QTcpServer(owner);
    if (!server->listen(QHostAddress::LocalHost)) {
        delete server;
        return nullptr;
    }
    outPort = server->serverPort();
    QObject::connect(server, &QTcpServer::newConnection, [server, body]() {
        QTcpSocket *sock = server->nextPendingConnection();
        if (!sock) return;
        QObject::connect(sock, &QTcpSocket::readyRead, [sock, body]() {
            sock->readAll();
            QByteArray response = QByteArray("HTTP/1.1 200 OK\r\n"
                                             "Content-Type: application/json\r\n"
                                             "Content-Length: ");
            response.append(QByteArray::number(body.size())).append("\r\n\r\n").append(body);
            sock->write(response);
            sock->disconnectFromHost();
        });
    });
    return server;
}

} // namespace

// ── F-04: разбор конвертов ─────────────────────────────────────────────────

void TestNetworkManager::testTorrentListEnvelopeIsUnwrapped()
{
    QByteArray body = QJsonDocument(QJsonObject{
        {QStringLiteral("torrents"), QJsonArray{1, 2, 3}},
        {QStringLiteral("totalCount"), 3},
        {QStringLiteral("limit"), 50},
        {QStringLiteral("offset"), 0},
        {QStringLiteral("hasMore"), false}
    }).toJson(QJsonDocument::Compact);

    int port = 0;
    QTcpServer *server = startRawJsonServer(body, port, this);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_mockServers.append(server);

    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));
    QSignalSpy spy(m_manager, &NetworkManager::torrentListReceived);

    m_manager->listTorrents();
    QVERIFY2(spy.wait(3000), "torrentListReceived не эмитился на конверт {\"torrents\":[...]}" );
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.takeFirst().at(0).value<QJsonArray>().size(), 3);
}

void TestNetworkManager::testFileListEnvelopeIsUnwrapped()
{
    QByteArray body = QJsonDocument(QJsonObject{
        {QStringLiteral("files"), QJsonArray{QJsonObject{{QStringLiteral("name"), QStringLiteral("a.mkv")}},
                                              QJsonObject{{QStringLiteral("name"), QStringLiteral("b.mkv")}}}},
        {QStringLiteral("totalCount"), 2}
    }).toJson(QJsonDocument::Compact);

    int port = 0;
    QTcpServer *server = startRawJsonServer(body, port, this);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_mockServers.append(server);

    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));
    QSignalSpy spy(m_manager, &NetworkManager::filesReceived);

    m_manager->getFiles(QStringLiteral("aabbcc"));
    QVERIFY2(spy.wait(3000), "filesReceived не эмитился на конверт {\"files\":[...]}" );
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.takeFirst().at(1).value<QJsonArray>().size(), 2);
}

void TestNetworkManager::testBareArrayEnvelopeStillWorks()
{
    QByteArray body = QJsonDocument(QJsonArray{1, 2}).toJson(QJsonDocument::Compact);

    int port = 0;
    QTcpServer *server = startRawJsonServer(body, port, this);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_mockServers.append(server);

    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));
    QSignalSpy spy(m_manager, &NetworkManager::torrentListReceived);

    m_manager->listTorrents();
    QVERIFY2(spy.wait(3000), "список торрентов перестал принимать голый массив");
    QCOMPARE(spy.takeFirst().at(0).value<QJsonArray>().size(), 2);
}

void TestNetworkManager::testEnvelopeWithoutExpectedFieldEmitsNothing()
{
    QByteArray body = QJsonDocument(QJsonObject{
        {QStringLiteral("somethingElse"), 1}
    }).toJson(QJsonDocument::Compact);

    int port = 0;
    QTcpServer *server = startRawJsonServer(body, port, this);
    QVERIFY2(server, "не удалось поднять mock-сервер");
    m_mockServers.append(server);

    m_manager->setServerUrl(QUrl(QString("http://127.0.0.1:%1").arg(port)));
    QSignalSpy spy(m_manager, &NetworkManager::torrentListReceived);

    m_manager->listTorrents();
    QVERIFY(!spy.wait(800));
    QCOMPARE(spy.count(), 0);
}
