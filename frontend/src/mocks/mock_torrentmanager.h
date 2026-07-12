/**
 * @file mock_torrentmanager.h
 * @brief Mock реализация ITorrentManager для gmock тестов
 */

#ifndef MOCK_TORRENTMANAGER_H
#define MOCK_TORRENTMANAGER_H

#include "../interfaces/itorrentmanager.h"
#include <gmock/gmock.h>

/**
 * @class MockTorrentManager
 * @brief Mock для ITorrentManager использующий gmock
 */
class MockTorrentManager : public ITorrentManager
{
    Q_OBJECT

public:
    MockTorrentManager(QObject *parent = nullptr) : ITorrentManager(parent) {}
    ~MockTorrentManager() override = default;

    MOCK_CONST_METHOD(QString, currentTorrentId, (), (override));
    MOCK_METHOD(void, setCurrentTorrentId, (const QString &id), (override));
    MOCK_CONST_METHOD(QString, streamUrl, (const QString &torrentId), (override));

    MOCK_METHOD(void, addTorrent, (const QString &magnetUri), (override));
    MOCK_METHOD(void, addTorrentFile, (const QByteArray &torrentData), (override));
    MOCK_METHOD(void, removeTorrent, (const QString &id), (override));
    MOCK_METHOD(void, listTorrents, (), (override));
    MOCK_METHOD(void, getFiles, (const QString &torrentId), (override));
    MOCK_METHOD(void, selectFile, (const QString &torrentId, int fileIndex), (override));
    MOCK_METHOD(void, onTorrentListReceived, (const QJsonArray &torrents), (override));
    MOCK_METHOD(void, onFilesReceived, (const QString &torrentId, const QJsonArray &files), (override));
};

#endif // MOCK_TORRENTMANAGER_H