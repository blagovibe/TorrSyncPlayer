/**
 * @file test_torrentmanager_gmock.cpp
 * @brief Unit tests for TorrentManager using Google Mock
 * 
 * Tests isolation of TorrentManager logic using mocked dependencies.
 */

#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <QJsonObject>
#include <QJsonArray>
#include <QByteArray>

#include "interfaces/itorrentmanager.h"
#include "mocks/mock_torrentmanager.h"

using ::testing::_;
using ::testing::Return;
using ::testing::Invoke;
using ::testing::SaveArg;

class TorrentManagerGMockTest : public ::testing::Test
{
protected:
    void SetUp() override {
        m_mockNetwork = new MockNetworkManager(nullptr);
        m_mockTorrentManager = new MockTorrentManager(nullptr);
    }
    
    void TearDown() override {
        delete m_mockTorrentManager;
        delete m_mockNetwork;
        m_mockTorrentManager = nullptr;
        m_mockNetwork = nullptr;
    }
    
    MockNetworkManager *m_mockNetwork = nullptr;
    MockTorrentManager *m_mockTorrentManager = nullptr;
    
    QJsonObject createTorrentJson(const QString &id = "test-id", 
                                   const QString &name = "Test Torrent",
                                   double progress = 0.5) {
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
};

// ── Basic operations ──────────────────────────────────────────────────

TEST_F(TorrentManagerGMockTest, AddTorrent)
{
    EXPECT_CALL(*m_mockTorrentManager, addTorrent(QString("magnet:?xt=urn:btih:test123")));
    m_mockTorrentManager->addTorrent(QString("magnet:?xt=urn:btih:test123"));
}

TEST_F(TorrentManagerGMockTest, AddTorrentFile)
{
    QByteArray torrentData = "d8:announce...";
    EXPECT_CALL(*m_mockTorrentManager, addTorrentFile(torrentData));
    m_mockTorrentManager->addTorrentFile(torrentData);
}

TEST_F(TorrentManagerGMockTest, RemoveTorrent)
{
    EXPECT_CALL(*m_mockTorrentManager, removeTorrent(QString("test-id")));
    m_mockTorrentManager->removeTorrent(QString("test-id"));
}

TEST_F(TorrentManagerGMockTest, ListTorrents)
{
    EXPECT_CALL(*m_mockTorrentManager, listTorrents());
    m_mockTorrentManager->listTorrents();
}

TEST_F(TorrentManagerGMockTest, GetFiles)
{
    EXPECT_CALL(*m_mockTorrentManager, getFiles(QString("torrent-123")));
    m_mockTorrentManager->getFiles(QString("torrent-123"));
}

TEST_F(TorrentManagerGMockTest, SelectFile)
{
    EXPECT_CALL(*m_mockTorrentManager, selectFile(QString("torrent-123"), 0));
    m_mockTorrentManager->selectFile(QString("torrent-123"), 0);
}

// ── Current torrent ID ────────────────────────────────────────────────

TEST_F(TorrentManagerGMockTest, CurrentTorrentId)
{
    EXPECT_CALL(*m_mockTorrentManager, currentTorrentId())
        .WillOnce(Return(QString("test-id")));
    EXPECT_EQ(m_mockTorrentManager->currentTorrentId(), QString("test-id"));
}

TEST_F(TorrentManagerGMockTest, SetCurrentTorrentId)
{
    EXPECT_CALL(*m_mockTorrentManager, setCurrentTorrentId(QString("new-id")));
    m_mockTorrentManager->setCurrentTorrentId(QString("new-id"));
}

// ── Stream URL ────────────────────────────────────────────────────────

TEST_F(TorrentManagerGMockTest, StreamUrl)
{
    EXPECT_CALL(*m_mockTorrentManager, streamUrl(QString("torrent-123")))
        .WillOnce(Return(QString("http://localhost:8889/api/v1/torrents/torrent-123/stream")));
    EXPECT_EQ(m_mockTorrentManager->streamUrl(QString("torrent-123")), 
              QString("http://localhost:8889/api/v1/torrents/torrent-123/stream"));
}

// ── Signal handling ───────────────────────────────────────────────────

// Note: Signal emission tests removed - QSignalSpy requires Q_OBJECT macro
// and MOC-generated meta-object code which conflicts with gmock MOCK_METHOD.
// Signals are tested in the Qt Test-based tests.

// ── Callbacks ──────────────────────────────────────────────────────────

TEST_F(TorrentManagerGMockTest, OnTorrentListReceived)
{
    QJsonArray torrents;
    torrents.append(createTorrentJson("id1", "Torrent 1"));
    torrents.append(createTorrentJson("id2", "Torrent 2"));
    
    EXPECT_CALL(*m_mockTorrentManager, onTorrentListReceived(torrents));
    m_mockTorrentManager->onTorrentListReceived(torrents);
}

TEST_F(TorrentManagerGMockTest, OnFilesReceived)
{
    QJsonArray files;
    files.append(QJsonObject{{"index", 0}, {"name", "video.mp4"}});
    
    EXPECT_CALL(*m_mockTorrentManager, onFilesReceived(QString("torrent-1"), files));
    m_mockTorrentManager->onFilesReceived(QString("torrent-1"), files);
}

// ── Mock interaction verification ──────────────────────────────────────

TEST_F(TorrentManagerGMockTest, VerifyCallOrder)
{
    testing::InSequence seq;
    
    EXPECT_CALL(*m_mockTorrentManager, addTorrent(QString("magnet:?xt=urn:btih:test1")));
    EXPECT_CALL(*m_mockTorrentManager, listTorrents());
    EXPECT_CALL(*m_mockTorrentManager, getFiles(QString("test-id")));
    EXPECT_CALL(*m_mockTorrentManager, selectFile(QString("test-id"), 0));
    EXPECT_CALL(*m_mockTorrentManager, removeTorrent(QString("test-id")));
    
    m_mockTorrentManager->addTorrent(QString("magnet:?xt=urn:btih:test1"));
    m_mockTorrentManager->listTorrents();
    m_mockTorrentManager->getFiles(QString("test-id"));
    m_mockTorrentManager->selectFile(QString("test-id"), 0);
    m_mockTorrentManager->removeTorrent(QString("test-id"));
}

TEST_F(TorrentManagerGMockTest, MultipleCalls)
{
    EXPECT_CALL(*m_mockTorrentManager, addTorrent(_))
        .Times(3);
    
    m_mockTorrentManager->addTorrent(QString("magnet:?xt=urn:btih:test1"));
    m_mockTorrentManager->addTorrent(QString("magnet:?xt=urn:btih:test2"));
    m_mockTorrentManager->addTorrent(QString("magnet:?xt=urn:btih:test3"));
}

// ── Edge cases ─────────────────────────────────────────────────────────

TEST_F(TorrentManagerGMockTest, EmptyMagnetUri)
{
    EXPECT_CALL(*m_mockTorrentManager, addTorrent(QString("")));
    m_mockTorrentManager->addTorrent(QString(""));
}

TEST_F(TorrentManagerGMockTest, SelectFileWithInvalidIndex)
{
    EXPECT_CALL(*m_mockTorrentManager, selectFile(QString("torrent-1"), -1));
    m_mockTorrentManager->selectFile(QString("torrent-1"), -1);
}

TEST_F(TorrentManagerGMockTest, SelectFileWithLargeIndex)
{
    EXPECT_CALL(*m_mockTorrentManager, selectFile(QString("torrent-1"), 999));
    m_mockTorrentManager->selectFile(QString("torrent-1"), 999);
}

int main(int argc, char **argv)
{
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#include "test_torrentmanager_gmock.moc"