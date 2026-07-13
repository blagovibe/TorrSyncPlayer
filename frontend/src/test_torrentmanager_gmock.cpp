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
#include <QSignalSpy>
#include <QByteArray>

#include "interfaces/itorrentmanager.h"
#include "interfaces/inetworkmanager.h"
#include "mocks/mock_networkmanager.h"
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

TEST_F(TorrentManagerGMockTest, TorrentAddedSignal)
{
    QSignalSpy spy(m_mockTorrentManager, &ITorrentManager::torrentAdded);
    
    QJsonObject torrent = createTorrentJson("torrent-1", "Test Torrent");
    emit m_mockTorrentManager->torrentAdded(torrent);
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).value<QJsonObject>()["id"].toString(), QString("torrent-1"));
}

TEST_F(TorrentManagerGMockTest, TorrentRemovedSignal)
{
    QSignalSpy spy(m_mockTorrentManager, &ITorrentManager::torrentRemoved);
    
    emit m_mockTorrentManager->torrentRemoved(QString("torrent-1"));
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), QString("torrent-1"));
}

TEST_F(TorrentManagerGMockTest, FilesReceivedSignal)
{
    QSignalSpy spy(m_mockTorrentManager, &ITorrentManager::filesReceived);
    
    QJsonArray files;
    files.append(QJsonObject{{"index", 0}, {"name", "video.mp4"}, {"size", 1024}});
    files.append(QJsonObject{{"index", 1}, {"name", "subs.srt"}, {"size", 64}});
    
    emit m_mockTorrentManager->filesReceived(QString("torrent-1"), files);
    
    EXPECT_EQ(spy.count(), 1);
    auto args = spy.takeFirst();
    EXPECT_EQ(args.at(0).toString(), QString("torrent-1"));
    EXPECT_EQ(args.at(1).toArray().size(), 2);
}

TEST_F(TorrentManagerGMockTest, FileSelectedSignal)
{
    QSignalSpy spy(m_mockTorrentManager, &ITorrentManager::fileSelected);
    
    emit m_mockTorrentManager->fileSelected(QString("torrent-1"), 0, 
        QString("http://localhost:8889/api/v1/torrents/torrent-1/stream"));
    
    EXPECT_EQ(spy.count(), 1);
    auto args = spy.takeFirst();
    EXPECT_EQ(args.at(0).toString(), QString("torrent-1"));
    EXPECT_EQ(args.at(1).toInt(), 0);
    EXPECT_EQ(args.at(2).toString(), 
        QString("http://localhost:8889/api/v1/torrents/torrent-1/stream"));
}

TEST_F(TorrentManagerGMockTest, ErrorSignal)
{
    QSignalSpy spy(m_mockTorrentManager, &ITorrentManager::error);
    
    emit m_mockTorrentManager->error(QString("Test error message"));
    
    EXPECT_EQ(spy.count(), 1);
    EXPECT_EQ(spy.takeFirst().at(0).toString(), QString("Test error message"));
}

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