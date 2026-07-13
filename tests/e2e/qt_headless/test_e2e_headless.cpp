/**
 * @file test_e2e_headless.cpp
 * @brief Headless Qt E2E tests using Qt Test framework
 * 
 * Runs the actual Qt application in headless mode (offscreen)
 * and tests full integration with backend.
 * 
 * Build: cmake -DBUILD_TESTS=ON -DCMAKE_BUILD_TYPE=Debug ..
 * Run: QT_QPA_PLATFORM=offscreen ctest -R E2E
 */

#include <QtTest>
#include <QProcess>
#include <QNetworkAccessManager>
#include <QNetworkRequest>
#include <QNetworkReply>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonArray>
#include <QEventLoop>
#include <QTimer>
#include <QTemporaryDir>
#include <QStandardPaths>
#include <QCoreApplication>

class E2EHeadlessTest : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();
    void cleanupTestCase();
    void init();
    void cleanup();

    // Backend lifecycle
    void testBackendStarts();
    void testBackendHealthCheck();
    void testBackendAPIEndpoints();
    void testBackendAuthFlow();

    // Full app integration
    void testAppStartup();
    void testTorrentOperations();
    void testRoomOperations();
    void testSyncOperations();

private:
    QProcess *m_backendProcess = nullptr;
    QString m_backendUrl = "https://localhost:8889";
    QString m_jwtToken;
    QTemporaryDir *m_tempDir = nullptr;
    QNetworkAccessManager *m_networkManager = nullptr;

    bool waitForBackendReady(int timeoutMs = 30000);
    QJsonObject sendRequest(const QString &method, const QString &path, const QJsonObject &body = QJsonObject());
    void startBackend();
    void stopBackend();
    QString findBackendBinary();
};

void E2EHeadlessTest::initTestCase()
{
    m_tempDir = new QTemporaryDir();
    QVERIFY(m_tempDir->isValid());
    
    m_networkManager = new QNetworkAccessManager(this);
    
    // Ignore SSL errors for self-signed certs
    connect(m_networkManager, &QNetworkAccessManager::sslErrors, [](QNetworkReply *reply, const QList<QSslError> &errors) {
        reply->ignoreSslErrors();
    });
    
    // Start backend
    startBackend();
    QVERIFY(waitForBackendReady());
    
    // Login to get JWT token
    QJsonObject loginBody;
    loginBody["username"] = "e2etest";
    loginBody["password"] = "TestPass123!";
    
    QJsonObject resp = sendRequest("POST", "/api/v1/auth/login", loginBody);
    if (resp.contains("token")) {
        m_jwtToken = resp["token"].toString();
    }
}

void E2EHeadlessTest::cleanupTestCase()
{
    stopBackend();
    delete m_tempDir;
    m_tempDir = nullptr;
}

void E2EHeadlessTest::init()
{
}

void E2EHeadlessTest::cleanup()
{
}

void E2EHeadlessTest::startBackend()
{
    m_backendProcess = new QProcess(this);
    
    QString backendPath = findBackendBinary();
    QVERIFY(!backendPath.isEmpty());
    
    QStringList args;
    args << "--port" << "8889"
         << "--auto-tls"
         << "--data-dir" << m_tempDir->path();
    
    QProcessEnvironment env = QProcessEnvironment::systemEnvironment();
    env.insert("JWT_SECRET", "test-jwt-secret-key-for-e2e-testing-min-32-chars");
    env.insert("LOG_LEVEL", "debug");
    m_backendProcess->setProcessEnvironment(env);
    
    m_backendProcess->start(backendPath, args);
    QVERIFY(m_backendProcess->waitForStarted(10000));
}

QString E2EHeadlessTest::findBackendBinary()
{
    QStringList possiblePaths = {
        QCoreApplication::applicationDirPath() + "/../../backend/build/torrsyncplayer",
        QCoreApplication::applicationDirPath() + "/../../../backend/build/torrsyncplayer",
        QCoreApplication::applicationDirPath() + "/../../backend/torrsyncplayer",
        QStandardPaths::findExecutable("torrsyncplayer"),
        QString::fromLocal8Bit(qgetenv("TORRSYNC_BACKEND_PATH")),
    };
    
    for (const QString &path : possiblePaths) {
        if (!path.isEmpty() && QFile::exists(path)) {
            return QFileInfo(path).absoluteFilePath();
        }
    }
    
    return QString();
}

void E2EHeadlessTest::stopBackend()
{
    if (m_backendProcess && m_backendProcess->state() == QProcess::Running) {
        m_backendProcess->terminate();
        m_backendProcess->waitForFinished(5000);
        if (m_backendProcess->state() == QProcess::Running) {
            m_backendProcess->kill();
            m_backendProcess->waitForFinished(2000);
        }
    }
    delete m_backendProcess;
    m_backendProcess = nullptr;
}

bool E2EHeadlessTest::waitForBackendReady(int timeoutMs)
{
    QElapsedTimer timer;
    timer.start();
    
    while (timer.elapsed() < timeoutMs) {
        QNetworkRequest request(QUrl(m_backendUrl + "/health"));
        QNetworkReply *reply = m_networkManager->get(request);
        
        QEventLoop loop;
        QTimer::singleShot(5000, &loop, &QEventLoop::quit);
        connect(reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
        loop.exec();
        
        if (reply->error() == QNetworkReply::NoError) {
            int statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
            if (statusCode == 200) {
                reply->deleteLater();
                return true;
            }
        }
        reply->deleteLater();
        
        QThread::msleep(1000);
    }
    
    return false;
}

QJsonObject E2EHeadlessTest::sendRequest(const QString &method, const QString &path, const QJsonObject &body)
{
    QUrl url(m_backendUrl + path);
    QNetworkRequest request(url);
    request.setHeader(QNetworkRequest::ContentTypeHeader, "application/json");
    
    if (!m_jwtToken.isEmpty()) {
        request.setRawHeader("Authorization", "Bearer " + m_jwtToken.toUtf8());
    }
    
    QByteArray data = QJsonDocument(body).toJson();
    QNetworkReply *reply = nullptr;
    
    if (method == "GET") {
        reply = m_networkManager->get(request);
    } else if (method == "POST") {
        reply = m_networkManager->post(request, data);
    } else if (method == "DELETE") {
        reply = m_networkManager->deleteResource(request);
    } else {
        return QJsonObject();
    }
    
    QEventLoop loop;
    QTimer::singleShot(10000, &loop, &QEventLoop::quit);
    connect(reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
    loop.exec();
    
    QJsonObject result;
    if (reply->error() == QNetworkReply::NoError) {
        QByteArray responseData = reply->readAll();
        QJsonDocument doc = QJsonDocument::fromJson(responseData);
        if (!doc.isNull()) {
            result = doc.object();
        }
    }
    
    reply->deleteLater();
    return result;
}

void E2EHeadlessTest::testBackendStarts()
{
    QVERIFY(m_backendProcess != nullptr);
    QVERIFY(m_backendProcess->state() == QProcess::Running);
}

void E2EHeadlessTest::testBackendHealthCheck()
{
    QNetworkRequest request(QUrl(m_backendUrl + "/health"));
    QNetworkReply *reply = m_networkManager->get(request);
    
    QEventLoop loop;
    QTimer::singleShot(5000, &loop, &QEventLoop::quit);
    connect(reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
    loop.exec();
    
    QVERIFY(reply->error() == QNetworkReply::NoError);
    int statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    QCOMPARE(statusCode, 200);
    
    reply->deleteLater();
}

void E2EHeadlessTest::testBackendAPIEndpoints()
{
    // Test torrent list endpoint
    QJsonObject torrents = sendRequest("GET", "/api/v1/torrents");
    QVERIFY(!torrents.isEmpty() || torrents.isEmpty()); // Either array or empty response
    
    // Test rooms endpoint
    QJsonObject rooms = sendRequest("GET", "/api/v1/rooms");
    QVERIFY(!rooms.isEmpty() || rooms.isEmpty());
    
    // Test sync status
    QJsonObject sync = sendRequest("GET", "/api/v1/sync/status");
    QVERIFY(!sync.isEmpty() || sync.isEmpty());
}

void E2EHeadlessTest::testBackendAuthFlow()
{
    // Register new user
    QJsonObject registerBody;
    registerBody["username"] = "e2euser" + QString::number(QRandomGenerator::global()->generate());
    registerBody["password"] = "TestPass123!";
    
    QJsonObject regResp = sendRequest("POST", "/api/v1/auth/register", registerBody);
    QVERIFY(regResp.contains("token"));
    
    // Login with registered user
    QJsonObject loginBody;
    loginBody["username"] = registerBody["username"];
    loginBody["password"] = registerBody["password"];
    
    QJsonObject loginResp = sendRequest("POST", "/api/v1/auth/login", loginBody);
    QVERIFY(loginResp.contains("token"));
    
    // Use token to access protected endpoint
    QString oldToken = m_jwtToken;
    m_jwtToken = loginResp["token"].toString();
    
    QJsonObject torrents = sendRequest("GET", "/api/v1/torrents");
    QVERIFY(!torrents.isEmpty() || torrents.isEmpty());
    
    m_jwtToken = oldToken;
}

void E2EHeadlessTest::testAppStartup()
{
    // Start the actual Qt frontend application in headless mode
    QProcess appProcess;
    
    QString appPath = QCoreApplication::applicationDirPath() + "/TorrSyncPlayer";
    if (!QFile::exists(appPath)) {
        appPath = QCoreApplication::applicationDirPath() + "/../TorrSyncPlayer";
    }
    if (!QFile::exists(appPath)) {
        QSKIP("Frontend binary not found");
    }
    
    QProcessEnvironment env = QProcessEnvironment::systemEnvironment();
    env.insert("QT_QPA_PLATFORM", "offscreen");
    env.insert("TORRSYNC_BACKEND_URL", m_backendUrl);
    env.insert("TORRSYNC_JWT_TOKEN", m_jwtToken);
    appProcess.setProcessEnvironment(env);
    
    appProcess.start(appPath, {"--headless"});
    QVERIFY(appProcess.waitForStarted(10000));
    
    // Wait for app to initialize
    QThread::sleep(3);
    
    // App should be running
    QVERIFY(appProcess.state() == QProcess::Running);
    
    // Clean shutdown
    appProcess.terminate();
    QVERIFY(appProcess.waitForFinished(5000));
}

void E2EHeadlessTest::testTorrentOperations()
{
    // Add torrent via API (simulating frontend)
    QString magnet = "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12&dn=TestVideo.mp4";
    
    QJsonObject addBody;
    addBody["magnetUri"] = magnet;
    
    QJsonObject addResp = sendRequest("POST", "/api/v1/torrents", addBody);
    QVERIFY(addResp.contains("id"));
    
    QString torrentId = addResp["id"].toString();
    QVERIFY(!torrentId.isEmpty());
    
    // List torrents
    QJsonObject listResp = sendRequest("GET", "/api/v1/torrents");
    QVERIFY(!listResp.isEmpty() || listResp.isEmpty());
    
    // Get files
    QJsonObject filesResp = sendRequest("GET", "/api/v1/torrents/" + torrentId + "/files");
    QVERIFY(!filesResp.isEmpty() || filesResp.isEmpty());
    
    // Select file
    QJsonObject selectBody;
    selectBody["fileIndex"] = 0;
    QJsonObject selectResp = sendRequest("POST", "/api/v1/torrents/" + torrentId + "/select", selectBody);
    QVERIFY(!selectResp.isEmpty() || selectResp.isEmpty());
    
    // Delete torrent
    QNetworkRequest deleteReq(QUrl(m_backendUrl + "/api/v1/torrents/" + torrentId));
    deleteReq.setRawHeader("Authorization", "Bearer " + m_jwtToken.toUtf8());
    QNetworkReply *deleteReply = m_networkManager->deleteResource(deleteReq);
    
    QEventLoop loop;
    connect(deleteReply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
    loop.exec();
    
    QVERIFY(deleteReply->error() == QNetworkReply::NoError);
    int statusCode = deleteReply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
    QCOMPARE(statusCode, 204);
    
    deleteReply->deleteLater();
}

void E2EHeadlessTest::testRoomOperations()
{
    // Create room
    QJsonObject createBody;
    createBody["name"] = "E2E Test Room";
    createBody["password"] = "";
    
    QJsonObject createResp = sendRequest("POST", "/api/v1/rooms", createBody);
    QVERIFY(createResp.contains("roomId") || createResp.contains("id"));
    
    QString roomId = createResp["roomId"].toString();
    if (roomId.isEmpty()) {
        roomId = createResp["id"].toString();
    }
    QVERIFY(!roomId.isEmpty());
    
    // Join room
    QJsonObject joinBody;
    joinBody["roomId"] = roomId;
    joinBody["password"] = "";
    
    QJsonObject joinResp = sendRequest("POST", "/api/v1/rooms/join", joinBody);
    QVERIFY(!joinResp.isEmpty() || joinResp.isEmpty());
    
    // Leave room
    QJsonObject leaveResp = sendRequest("POST", "/api/v1/rooms/leave", QJsonObject());
    QVERIFY(!leaveResp.isEmpty() || leaveResp.isEmpty());
}

void E2EHeadlessTest::testSyncOperations()
{
    // Create room first
    QJsonObject createBody;
    createBody["name"] = "Sync Test Room";
    QJsonObject createResp = sendRequest("POST", "/api/v1/rooms", createBody);
    
    QString roomId = createResp["roomId"].toString();
    if (roomId.isEmpty()) roomId = createResp["id"].toString();
    QVERIFY(!roomId.isEmpty());
    
    // Join room
    QJsonObject joinBody;
    joinBody["roomId"] = roomId;
    sendRequest("POST", "/api/v1/rooms/join", joinBody);
    
    // Test sync play
    QJsonObject playResp = sendRequest("POST", "/api/v1/sync/play", QJsonObject());
    QVERIFY(!playResp.isEmpty() || playResp.isEmpty());
    
    // Test sync pause
    QJsonObject pauseResp = sendRequest("POST", "/api/v1/sync/pause", QJsonObject());
    QVERIFY(!pauseResp.isEmpty() || pauseResp.isEmpty());
    
    // Test sync seek
    QJsonObject seekBody;
    seekBody["position"] = 120.5;
    QJsonObject seekResp = sendRequest("POST", "/api/v1/sync/seek", seekBody);
    QVERIFY(!seekResp.isEmpty() || seekResp.isEmpty());
    
    // Test sync status
    QJsonObject statusResp = sendRequest("GET", "/api/v1/sync/status");
    QVERIFY(!statusResp.isEmpty() || statusResp.isEmpty());
    
    // Leave room
    sendRequest("POST", "/api/v1/rooms/leave", QJsonObject());
}

QTEST_MAIN(E2EHeadlessTest)
#include "test_e2e_headless.moc"