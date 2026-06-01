/**
 * @file test_torrentmodel.cpp
 * @brief Unit-тесты для TorrentModel
 * 
 * Тестирует:
 * - Создание и уничтожение модели
 * - Добавление/удаление торрентов
 * - Обновление данных торрента
 * - Сериализацию/десериализацию JSON
 * - Роли модели для QML
 */

#include <QtTest>
#include <QJsonObject>
#include <QJsonArray>
#include "torrentmodel.h"

class TestTorrentModel : public QObject
{
    Q_OBJECT

private slots:
    // ── Инициализация ─────────────────────────────────────────────────────
    void initTestCase();
    void cleanupTestCase();
    void init();
    void cleanup();

    // ── Базовые операции ─────────────────────────────────────────────────
    void testCreateEmptyModel();
    void testAddTorrent();
    void testAddTorrentDuplicate();
    void testRemoveTorrent();
    void testRemoveNonExistentTorrent();
    void testClearModel();

    // ── Доступ к данным ──────────────────────────────────────────────────
    void testTorrentAt();
    void testTorrentById();
    void testIndexOf();
    void testContains();

    // ── Обновление данных ────────────────────────────────────────────────
    void testUpdateTorrent();
    void testUpdateNonExistentTorrent();

    // ── JSON сериализация ─────────────────────────────────────────────────
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

    // ── Сигналы ───────────────────────────────────────────────────────────
    void testTorrentAddedSignal();
    void testTorrentRemovedSignal();
    void testTorrentUpdatedSignal();

    // ── Граничные случаи ─────────────────────────────────────────────────
    void testEmptyModelAccess();
    void testInvalidIndex();

private:
    TorrentModel *m_model;
    
    // Вспомогательные функции
    TorrentInfo createTestTorrent(const QString &id = "test-id", 
                                    const QString &name = "Test Torrent",
                                    double progress = 0.5);
    QJsonObject createTestTorrentJson(const QString &id = "test-id",
                                        const QString &name = "Test Torrent",
                                        double progress = 0.5);
};

void TestTorrentModel::initTestCase()
{
    // Выполняется один раз перед всеми тестами
}

void TestTorrentModel::cleanupTestCase()
{
    // Выполняется один раз после всех тестов
}

void TestTorrentModel::init()
{
    // Выполняется перед каждым тестом
    m_model = new TorrentModel(this);
}

void TestTorrentModel::cleanup()
{
    // Выполняется после каждого теста
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

// ── Базовые операции ─────────────────────────────────────────────────────

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
    m_model->addTorrent(torrent2); // Должен обновить существующий
    
    QCOMPARE(m_model->rowCount(), 1); // Не добавился дубликат
    QCOMPARE(m_model->torrentAt(0).progress, 0.7); // Данные обновились
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

// ── Доступ к данным ──────────────────────────────────────────────────────

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
    
    // Несуществующий ID
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

// ── Обновление данных ────────────────────────────────────────────────────

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

// ── JSON сериализация ─────────────────────────────────────────────────────

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
    
    // С родителем всегда 0 для списковых моделей
    QModelIndex parent;
    QCOMPARE(m_model->rowCount(parent), 2);
}

void TestTorrentModel::testData()
{
    m_model->addTorrent(createTestTorrent("data-test", "Data Test", 0.6));
    
    QModelIndex index = m_model->index(0);
    
    // Проверяем различные роли
    QCOMPARE(m_model->data(index, TorrentModel::IdRole).toString(), QString("data-test"));
    QCOMPARE(m_model->data(index, TorrentModel::NameRole).toString(), QString("Data Test"));
    QCOMPARE(m_model->data(index, TorrentModel::ProgressRole).toDouble(), 0.6);
    QCOMPARE(m_model->data(index, TorrentModel::StatusRole).toString(), QString("downloading"));
    
    // DisplayRole должен содержать отформатированную строку
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

// ── Сигналы ───────────────────────────────────────────────────────────────

void TestTorrentModel::testTorrentAddedSignal()
{
    QSignalSpy spy(m_model, &TorrentModel::torrentAdded);
    
    m_model->addTorrent(createTestTorrent("signal-test"));
    
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.at(0).at(0).toInt(), 0); // Индекс добавленного элемента
}

void TestTorrentModel::testTorrentRemovedSignal()
{
    m_model->addTorrent(createTestTorrent("to-remove"));
    
    QSignalSpy spy(m_model, &TorrentModel::torrentRemoved);
    
    m_model->removeTorrent("to-remove");
    
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.at(0).at(0).toInt(), 0);
}

void TestTorrentModel::testTorrentUpdatedSignal()
{
    m_model->addTorrent(createTestTorrent("to-update"));
    
    QSignalSpy spy(m_model, &TorrentModel::torrentUpdated);
    
    TorrentInfo updated = createTestTorrent("to-update", "Updated");
    m_model->updateTorrent("to-update", updated);
    
    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.at(0).at(0).toInt(), 0);
}

// ── Граничные случаи ─────────────────────────────────────────────────────

void TestTorrentModel::testEmptyModelAccess()
{
    // Доступ к пустой модели не должен вызывать ошибок
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
    
    // Отрицательный индекс
    TorrentInfo info = m_model->torrentAt(-1);
    QVERIFY(info.id.isEmpty());
    
    // Индекс за пределами
    info = m_model->torrentAt(100);
    QVERIFY(info.id.isEmpty());
    
    // Невалидный индекс для data
    QModelIndex invalidIndex = m_model->index(100);
    QCOMPARE(m_model->data(invalidIndex, TorrentModel::IdRole), QVariant());
}

QTEST_MAIN(TestTorrentModel)
#include "test_torrentmodel.moc"
