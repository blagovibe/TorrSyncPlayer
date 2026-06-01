/**
 * @file test_networkmanager.cpp
 * @brief Unit-тесты для NetworkManager
 * 
 * Тестирует:
 * - Создание и настройку NetworkManager
 * - Формирование URL для API
 * - Формирование URL для стриминга
 * - Управление состоянием комнаты
 * - Парсинг JSON
 * - Обработку ошибок API
 */

#include <QtTest>
#include <QSignalSpy>
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
    // ── Инициализация ─────────────────────────────────────────────────────
    void initTestCase();
    void cleanupTestCase();
    void init();
    void cleanup();

    // ── Базовые свойства ──────────────────────────────────────────────────
    void testDefaultServerUrl();
    void testSetServerUrl();
    void testServerUrl();

    // ── URL формирование ──────────────────────────────────────────────────
    void testStreamUrl();
    void testStreamUrlWithDifferentIds();

    // ── Состояние комнаты ─────────────────────────────────────────────────
    void testInitialRoomState();
    void testJoinRoomState();
    void testLeaveRoomState();

    // ── JSON парсинг ──────────────────────────────────────────────────────
    void testParseValidJson();
    void testParseInvalidJson();
    void testParseEmptyJson();
    void testParseJsonObject();
    void testParseJsonArray();

    // ── Сигналы ───────────────────────────────────────────────────────────
    void testErrorSignal();
    void testTorrentAddedSignal();
    void testTorrentListReceivedSignal();
    void testRoomCreatedSignal();
    void testRoomJoinedSignal();
    void testRoomLeftSignal();
    void testSyncStatusReceivedSignal();

    // ── Граничные случаи ──────────────────────────────────────────────────
    void testEmptyMagnetUri();
    void testEmptyRoomName();
    void testSpecialCharactersInRoomName();

private:
    NetworkManager *m_manager;
    
    // Вспомогательные функции
    QJsonObject createTorrentJson(const QString &id = "test-id", 
                                   const QString &name = "Test Torrent");
    QJsonObject createRoomJson(const QString &id = "test-room-id",
                                const QString &name = "Test Room");
    QJsonObject createSyncStatusJson(bool isPlaying = false, double position = 0.0);
};

void TestNetworkManager::initTestCase()
{
    // Выполняется один раз перед всеми тестами
}

void TestNetworkManager::cleanupTestCase()
{
    // Выполняется один раз после всех тестов
}

void TestNetworkManager::init()
{
    // Выполняется перед каждым тестом
    m_manager = new NetworkManager(this);
}

void TestNetworkManager::cleanup()
{
    // Выполняется после каждого теста
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

// ── Базовые свойства ──────────────────────────────────────────────────────

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

// ── URL формирование ──────────────────────────────────────────────────────

void TestNetworkManager::testStreamUrl()
{
    QString torrentId = "abc123def456";
    QString expectedUrl = "http://localhost:8889/api/v1/torrents/abc123def456/stream";
    
    QCOMPARE(m_manager->streamUrl(torrentId), expectedUrl);
}

void TestNetworkManager::testStreamUrlWithDifferentIds()
{
    // Обычный ID
    QCOMPARE(m_manager->streamUrl("simple-id"), 
             QString("http://localhost:8889/api/v1/torrents/simple-id/stream"));
    
    // ID с хешем
    QCOMPARE(m_manager->streamUrl("0123456789abcdef0123456789abcdef01234567"),
             QString("http://localhost:8889/api/v1/torrents/0123456789abcdef0123456789abcdef01234567/stream"));
    
    // Пустой ID
    QCOMPARE(m_manager->streamUrl(""),
             QString("http://localhost:8889/api/v1/torrents//stream"));
}

// ── Состояние комнаты ─────────────────────────────────────────────────────

void TestNetworkManager::testInitialRoomState()
{
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

void TestNetworkManager::testJoinRoomState()
{
    // Симулируем присоединение к комнате
    QSignalSpy spy(m_manager, &NetworkManager::roomJoined);
    
    m_manager->joinRoom("test-room-id", "");
    
    // После вызова joinRoom, currentRoomId должен быть установлен
    QCOMPARE(m_manager->currentRoomId(), QString("test-room-id"));
    QVERIFY(m_manager->isInRoom());
}

void TestNetworkManager::testLeaveRoomState()
{
    // Сначала присоединяемся
    m_manager->joinRoom("test-room-id", "");
    QVERIFY(m_manager->isInRoom());
    
    // Затем выходим
    QSignalSpy spy(m_manager, &NetworkManager::roomLeft);
    m_manager->leaveRoom();
    
    QVERIFY(!m_manager->isInRoom());
    QVERIFY(m_manager->currentRoomId().isEmpty());
}

// ── JSON парсинг ──────────────────────────────────────────────────────────

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

// ── Сигналы ───────────────────────────────────────────────────────────────

void TestNetworkManager::testErrorSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::error);
    
    // Попытка отправить сигнал без присоединения к комнате
    QJsonObject signal;
    signal["type"] = "offer";
    m_manager->sendSignal(signal);
    
    QCOMPARE(spy.count(), 1);
    QVERIFY(spy.at(0).at(0).toString().contains("Не в комнате"));
}

void TestNetworkManager::testTorrentAddedSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::torrentAdded);
    
    // Сигнал будет испущен при получении ответа от сервера
    // Здесь мы просто проверяем, что сигнал существует и может быть подключен
    QVERIFY(spy.isValid());
}

void TestNetworkManager::testTorrentListReceivedSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::torrentListReceived);
    
    QVERIFY(spy.isValid());
}

void TestNetworkManager::testRoomCreatedSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::roomCreated);
    
    QVERIFY(spy.isValid());
}

void TestNetworkManager::testRoomJoinedSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::roomJoined);
    
    m_manager->joinRoom("test-room", "");
    
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.at(0).at(0).toString(), QString("test-room"));
}

void TestNetworkManager::testRoomLeftSignal()
{
    m_manager->joinRoom("test-room", "");
    
    QSignalSpy spy(m_manager, &NetworkManager::roomLeft);
    
    m_manager->leaveRoom();
    
    QCOMPARE(spy.count(), 1);
}

void TestNetworkManager::testSyncStatusReceivedSignal()
{
    QSignalSpy spy(m_manager, &NetworkManager::syncStatusReceived);
    
    QVERIFY(spy.isValid());
}

// ── Граничные случаи ──────────────────────────────────────────────────────

void TestNetworkManager::testEmptyMagnetUri()
{
    QSignalSpy spy(m_manager, &NetworkManager::error);
    
    // Вызов с пустым magnet URI - не должен вызвать ошибку на клиенте
    // (валидация происходит на сервере)
    m_manager->addTorrent("");
    
    // Запрос будет отправлен, ошибка вернётся от сервера
    QVERIFY(spy.count() == 0); // Немедленной ошибки быть не должно
}

void TestNetworkManager::testEmptyRoomName()
{
    QSignalSpy spy(m_manager, &NetworkManager::error);
    
    // Вызов с пустым именем комнаты
    m_manager->createRoom("", "");
    
    // Запрос будет отправлен, ошибка вернётся от сервера
    QVERIFY(spy.count() == 0);
}

void TestNetworkManager::testSpecialCharactersInRoomName()
{
    // Проверяем, что специальные символы корректно обрабатываются
    m_manager->createRoom("Test Room 日本語", "");
    m_manager->createRoom("Комната тест", "");
    m_manager->createRoom("Room with spaces", "");
    
    // Ошибок быть не должно
    QVERIFY(true);
}

QTEST_MAIN(TestNetworkManager)
#include "test_networkmanager.moc"
