/**
 * @file test_torrentmodel.cpp
 * @brief Unit tests for TorrentModel
 *
 * Tests:
 * - Model creation and destruction
 * - Adding/removing torrents
 * - Updating torrent data
 * - JSON serialization/deserialization
 * - Model roles for QML
 * - Signal emissions via QSignalSpy
 */

#include <QtTest>
#include <QSignalSpy>
#include <QJsonObject>
#include <QJsonArray>
#include "torrentmodel.h"

class TestTorrentModel : public QObject
{
    Q_OBJECT

private slots:
    // ── Initialization ─────────────────────────────────────────────────────
    void initTestCase();
    void cleanupTestCase();
    void init();
    void cleanup();

    // ── Basic operations ─────────────────────────────────────────────────
    void testCreateEmptyModel();
    void testAddTorrent();
    void testAddTorrentDuplicate();
    void testRemoveTorrent();
    void testRemoveNonExistentTorrent();
    void testClearModel();

    // ── Data access ──────────────────────────────────────────────────────
    void testTorrentAt();
    void testTorrentById();
    void testIndexOf();
    void testContains();

    // ── Data update ──────────────────────────────────────────────────────
    void testUpdateTorrent();
    void testUpdateNonExistentTorrent();

    // ── JSON serialization ─────────────────────────────────────────────────
    void testTorrentInfoFromJson();
    void testTorrentInfoToJson();
    void testAddTorrentFromJson();
    void testUpdateTorrentFromJson();
    void testLoadFromJson();

    // ── QAbstractListModel interface ──────────────────────────────────────
    void testRowCount();
    void testData();
    void testRoleNames();
    void testHeaderData();

    // ── Edge cases ───────────────────────────────────────────────────────
    void testEmptyModelAccess();
    void testInvalidIndex();

    // ── Signal emissions ─────────────────────────────────────────────────
    void testTorrentAddedSignal();
    void testTorrentRemovedSignal();
    void testTorrentUpdatedSignal();
    void testTorrentAddedSignalData();
    void testTorrentRemovedSignalData();

private:
    TorrentModel *m_model;

    TorrentInfo createTestTorrent(const QString &id = "test-id",
                                    const QString &name = "Test Torrent",
                                    double progress = 0.5);
    QJsonObject createTestTorrentJson(const QString &id = "test-id",
                                        const QString &name = "Test Torrent",
                                        double progress = 0.5);
};

void TestTorrentModel::initTestCase()
{
}

void TestTorrentModel::cleanupTestCase()
{
}

void TestTorrentModel::init()
{
    m_model = new TorrentModel(this);
}

void TestTorrentModel::cleanup()
{
    delete m_model;
    m_model = nullptr;
}

TorrentInfo TestTorrentModel::createTestTorrent(const QString &id,
                                                  const QString &name,
                                                  double progress)
{
    TorrentInfo info;
    info.id = id;
    info.name = name;
    info.progress = progress;
    info.status = "downloading";
    info.size = 1024 * 1024 * 100; // 100 MB
    info.downloaded = info.size * progress;
    info.uploadSpeed = 1024 * 10; // 10 KB/s
    info.downloadSpeed = 1024 * 100; // 100 KB/s
    return info;
}

QJsonObject TestTorrentModel::createTestTorrentJson(const QString &id,
                                                      const QString &name,
                                                      double progress)
{
    QJsonObject json;
    json["id"] = id;
    json["name"] = name;
    json["progress"] = progress;
    json["status"] = "downloading";
    json["size"] = 1024 * 1024 * 100;
    json["downloaded"] = 1024 * 1024 * 100 * progress;
    json["uploadSpeed"] = 1024 * 10;
    json["downloadSpeed"] = 1024 * 100;
    return json;
}

// ── Basic operations ─────────────────────────────────────────────────────

void TestTorrentModel::testCreateEmptyModel()
{
    QCOMPARE(m_model->rowCount(), 0);
    QVERIFY(m_model->torrents().isEmpty());
}

void TestTorrentModel::testAddTorrent()
{
    TorrentInfo torrent = createTestTorrent("torrent-1", "First Torrent");
    m_model->addTorrent(torrent);

    QCOMPARE(m_model->rowCount(), 1);
    QCOMPARE(m_model->torrentAt(0).id, QString("torrent-1"));
    QCOMPARE(m_model->torrentAt(0).name, QString("First Torrent"));
}

void TestTorrentModel::testAddTorrentDuplicate()
{
    TorrentInfo torrent1 = createTestTorrent("torrent-1", "First Torrent", 0.3);
    TorrentInfo torrent2 = createTestTorrent("torrent-1", "Updated Torrent", 0.7);

    m_model->addTorrent(torrent1);
    m_model->addTorrent(torrent2);

    QCOMPARE(m_model->rowCount(), 1);
    QCOMPARE(m_model->torrentAt(0).progress, 0.7);
}

void TestTorrentModel::testRemoveTorrent()
{
    m_model->addTorrent(createTestTorrent("torrent-1"));
    m_model->addTorrent(createTestTorrent("torrent-2"));

    QCOMPARE(m_model->rowCount(), 2);

    bool removed = m_model->removeTorrent("torrent-1");
    QVERIFY(removed);
    QCOMPARE(m_model->rowCount(), 1);
    QCOMPARE(m_model->torrentAt(0).id, QString("torrent-2"));
}

void TestTorrentModel::testRemoveNonExistentTorrent()
{
    m_model->addTorrent(createTestTorrent("torrent-1"));

    bool removed = m_model->removeTorrent("non-existent");
    QVERIFY(!removed);
    QCOMPARE(m_model->rowCount(), 1);
}

void TestTorrentModel::testClearModel()
{
    m_model->addTorrent(createTestTorrent("torrent-1"));
    m_model->addTorrent(createTestTorrent("torrent-2"));
    m_model->addTorrent(createTestTorrent("torrent-3"));

    QCOMPARE(m_model->rowCount(), 3);

    m_model->clear();
    QCOMPARE(m_model->rowCount(), 0);
}

// ── Data access ──────────────────────────────────────────────────────────

void TestTorrentModel::testTorrentAt()
{
    m_model->addTorrent(createTestTorrent("torrent-1", "First"));
    m_model->addTorrent(createTestTorrent("torrent-2", "Second"));

    QCOMPARE(m_model->torrentAt(0).id, QString("torrent-1"));
    QCOMPARE(m_model->torrentAt(1).id, QString("torrent-2"));
}

void TestTorrentModel::testTorrentById()
{
    m_model->addTorrent(createTestTorrent("torrent-1", "First"));

    TorrentInfo found = m_model->torrentById("torrent-1");
    QCOMPARE(found.id, QString("torrent-1"));
    QCOMPARE(found.name, QString("First"));

    TorrentInfo notFound = m_model->torrentById("non-existent");
    QVERIFY(notFound.id.isEmpty());
}

void TestTorrentModel::testIndexOf()
{
    m_model->addTorrent(createTestTorrent("torrent-1"));
    m_model->addTorrent(createTestTorrent("torrent-2"));
    m_model->addTorrent(createTestTorrent("torrent-3"));

    QCOMPARE(m_model->indexOf("torrent-1"), 0);
    QCOMPARE(m_model->indexOf("torrent-2"), 1);
    QCOMPARE(m_model->indexOf("torrent-3"), 2);
    QCOMPARE(m_model->indexOf("non-existent"), -1);
}

void TestTorrentModel::testContains()
{
    m_model->addTorrent(createTestTorrent("torrent-1"));

    QVERIFY(m_model->contains("torrent-1"));
    QVERIFY(!m_model->contains("non-existent"));
}

// ── Data update ──────────────────────────────────────────────────────────

void TestTorrentModel::testUpdateTorrent()
{
    m_model->addTorrent(createTestTorrent("torrent-1", "Original", 0.3));

    TorrentInfo updated = createTestTorrent("torrent-1", "Updated", 0.8);
    bool result = m_model->updateTorrent("torrent-1", updated);

    QVERIFY(result);
    QCOMPARE(m_model->torrentAt(0).name, QString("Updated"));
    QCOMPARE(m_model->torrentAt(0).progress, 0.8);
}

void TestTorrentModel::testUpdateNonExistentTorrent()
{
    TorrentInfo torrent = createTestTorrent("non-existent");
    bool result = m_model->updateTorrent("non-existent", torrent);

    QVERIFY(!result);
}

// ── JSON serialization ─────────────────────────────────────────────────────

void TestTorrentModel::testTorrentInfoFromJson()
{
    QJsonObject json = createTestTorrentJson("json-torrent", "JSON Torrent", 0.75);

    TorrentInfo info = TorrentInfo::fromJson(json);

    QCOMPARE(info.id, QString("json-torrent"));
    QCOMPARE(info.name, QString("JSON Torrent"));
    QCOMPARE(info.progress, 0.75);
    QCOMPARE(info.status, QString("downloading"));
}

void TestTorrentModel::testTorrentInfoToJson()
{
    TorrentInfo info = createTestTorrent("test-id", "Test", 0.5);

    QJsonObject json = info.toJson();

    QCOMPARE(json["id"].toString(), QString("test-id"));
    QCOMPARE(json["name"].toString(), QString("Test"));
    QCOMPARE(json["progress"].toDouble(), 0.5);
}

void TestTorrentModel::testAddTorrentFromJson()
{
    QJsonObject json = createTestTorrentJson("json-1", "From JSON");
    m_model->addTorrentFromJson(json);

    QCOMPARE(m_model->rowCount(), 1);
    QCOMPARE(m_model->torrentAt(0).id, QString("json-1"));
}

void TestTorrentModel::testUpdateTorrentFromJson()
{
    m_model->addTorrent(createTestTorrent("update-test", "Original"));

    QJsonObject json = createTestTorrentJson("update-test", "Updated", 0.9);
    bool result = m_model->updateTorrentFromJson(json);

    QVERIFY(result);
    QCOMPARE(m_model->torrentAt(0).name, QString("Updated"));
}

void TestTorrentModel::testLoadFromJson()
{
    QJsonArray array;
    array.append(createTestTorrentJson("load-1", "First"));
    array.append(createTestTorrentJson("load-2", "Second"));
    array.append(createTestTorrentJson("load-3", "Third"));

    m_model->loadFromJson(array);

    QCOMPARE(m_model->rowCount(), 3);
    QCOMPARE(m_model->torrentAt(0).id, QString("load-1"));
    QCOMPARE(m_model->torrentAt(2).id, QString("load-3"));
}

// ── QAbstractListModel interface ──────────────────────────────────────────

void TestTorrentModel::testRowCount()
{
    QCOMPARE(m_model->rowCount(), 0);

    m_model->addTorrent(createTestTorrent("t1"));
    QCOMPARE(m_model->rowCount(), 1);

    m_model->addTorrent(createTestTorrent("t2"));
    QCOMPARE(m_model->rowCount(), 2);

    QModelIndex parent;
    QCOMPARE(m_model->rowCount(parent), 2);
}

void TestTorrentModel::testData()
{
    m_model->addTorrent(createTestTorrent("data-test", "Data Test", 0.6));

    QModelIndex index = m_model->index(0);

    QCOMPARE(m_model->data(index, TorrentModel::IdRole).toString(), QString("data-test"));
    QCOMPARE(m_model->data(index, TorrentModel::NameRole).toString(), QString("Data Test"));
    QCOMPARE(m_model->data(index, TorrentModel::ProgressRole).toDouble(), 0.6);
    QCOMPARE(m_model->data(index, TorrentModel::StatusRole).toString(), QString("downloading"));

    QString display = m_model->data(index, Qt::DisplayRole).toString();
    QVERIFY(display.contains("Data Test"));
    QVERIFY(display.contains("60"));
}

void TestTorrentModel::testRoleNames()
{
    QHash<int, QByteArray> roles = m_model->roleNames();

    QVERIFY(roles.contains(TorrentModel::IdRole));
    QVERIFY(roles.contains(TorrentModel::NameRole));
    QVERIFY(roles.contains(TorrentModel::ProgressRole));
    QVERIFY(roles.contains(TorrentModel::StatusRole));
    QVERIFY(roles.contains(TorrentModel::SizeRole));

    QCOMPARE(roles[TorrentModel::IdRole], QByteArray("id"));
    QCOMPARE(roles[TorrentModel::NameRole], QByteArray("name"));
}

void TestTorrentModel::testHeaderData()
{
    QVariant header = m_model->headerData(0, Qt::Horizontal, Qt::DisplayRole);
    QVERIFY(header.isValid());
}

// ── Edge cases ───────────────────────────────────────────────────────────

void TestTorrentModel::testEmptyModelAccess()
{
    TorrentInfo info = m_model->torrentAt(0);
    QVERIFY(info.id.isEmpty());

    info = m_model->torrentById("any");
    QVERIFY(info.id.isEmpty());

    QModelIndex index = m_model->index(0);
    QCOMPARE(m_model->data(index, TorrentModel::IdRole), QVariant());
}

void TestTorrentModel::testInvalidIndex()
{
    m_model->addTorrent(createTestTorrent("valid"));

    TorrentInfo info = m_model->torrentAt(-1);
    QVERIFY(info.id.isEmpty());

    info = m_model->torrentAt(100);
    QVERIFY(info.id.isEmpty());

    QModelIndex invalidIndex = m_model->index(100);
    QCOMPARE(m_model->data(invalidIndex, TorrentModel::IdRole), QVariant());
}

// ── Signal emissions ─────────────────────────────────────────────────────

void TestTorrentModel::testTorrentAddedSignal()
{
    QSignalSpy spy(m_model, &TorrentModel::torrentAdded);

    TorrentInfo torrent = createTestTorrent("signal-test", "Signal Test");
    m_model->addTorrent(torrent);

    QCOMPARE(spy.count(), 1);
}

void TestTorrentModel::testTorrentRemovedSignal()
{
    m_model->addTorrent(createTestTorrent("remove-signal-test"));

    QSignalSpy spy(m_model, &TorrentModel::torrentRemoved);

    m_model->removeTorrent("remove-signal-test");

    QCOMPARE(spy.count(), 1);
}

void TestTorrentModel::testTorrentUpdatedSignal()
{
    m_model->addTorrent(createTestTorrent("update-signal-test", "Original"));

    QSignalSpy spy(m_model, &TorrentModel::torrentUpdated);

    TorrentInfo updated = createTestTorrent("update-signal-test", "Updated", 0.9);
    m_model->updateTorrent("update-signal-test", updated);

    QCOMPARE(spy.count(), 1);
}

void TestTorrentModel::testTorrentAddedSignalData()
{
    QSignalSpy spy(m_model, &TorrentModel::torrentAdded);

    m_model->addTorrent(createTestTorrent("data-signal-test"));

    QCOMPARE(spy.count(), 1);
    QList<QVariant> args = spy.takeFirst();
    QCOMPARE(args.at(0).toInt(), 0); // index should be 0
}

void TestTorrentModel::testTorrentRemovedSignalData()
{
    m_model->addTorrent(createTestTorrent("remove-data-1"));
    m_model->addTorrent(createTestTorrent("remove-data-2"));

    QSignalSpy spy(m_model, &TorrentModel::torrentRemoved);

    m_model->removeTorrent("remove-data-1");

    QCOMPARE(spy.count(), 1);
    QList<QVariant> args = spy.takeFirst();
    QCOMPARE(args.at(0).toInt(), 0); // index should be 0
}

QTEST_MAIN(TestTorrentModel)
#include "test_torrentmodel.moc"
