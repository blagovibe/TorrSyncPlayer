/**
 * @file mock_networkmanager.h
 * @brief Google Mock реализация INetworkManager
 * 
 * Используется для изолированного тестирования компонентов,
 * зависящих от NetworkManager, без реальных сетевых запросов.
 */

#ifndef MOCK_NETWORKMANAGER_H
#define MOCK_NETWORKMANAGER_H

#include "gmock/gmock.h"
#include "../interfaces/inetworkmanager.h"

/**
 * @class MockNetworkManager
 * @brief Mock реализация INetworkManager для gmock
 * 
 * Все методы объявлены как MOCK_METHOD для возможности
 * установки ожиданий (EXPECT_CALL) и возврата значений (ON_CALL).
 */
class MockNetworkManager : public INetworkManager
{
public:
    MockNetworkManager(QObject *parent = nullptr) : INetworkManager(parent) {}
    ~MockNetworkManager() override = default;

    // ── Torrent API ───────────────────────────────────────────────────────
    
    MOCK_METHOD(void, addTorrent, (const QString &magnetUri), (override));
    MOCK_METHOD(void, addTorrentFile, (const QByteArray &torrentData), (override));
    MOCK_METHOD(void, removeTorrent, (const QString &id), (override));
    MOCK_METHOD(void, listTorrents, (), (override));
    MOCK_METHOD(void, getFiles, (const QString &torrentId), (override));
    MOCK_METHOD(void, selectFile, (const QString &torrentId, int fileIndex), (override));
    MOCK_METHOD(void, setBufferPosition, (const QString &torrentId, qint64 positionBytes), (override));
    MOCK_METHOD(void, getBufferInfo, (const QString &torrentId), (override));

    // ── Room API ──────────────────────────────────────────────────────────
    
    MOCK_METHOD(void, createRoom, (const QString &name, const QString &password), (override));
    MOCK_METHOD(void, joinRoom, (const QString &roomId, const QString &password), (override));
    MOCK_METHOD(void, leaveRoom, (), (override));
    MOCK_METHOD(void, sendSignal, (const QJsonObject &signal), (override));

    // ── Sync API ──────────────────────────────────────────────────────────
    
    MOCK_METHOD(void, syncPlay, (), (override));
    MOCK_METHOD(void, syncPause, (), (override));
    MOCK_METHOD(void, syncSeek, (double position), (override));

    // ── Утилиты ─────────────────────────────────────────────────────────
    
    MOCK_METHOD(QString, streamUrl, (const QString &torrentId), (const, override));
    MOCK_METHOD(QString, serverUrl, (), (const, override));
    MOCK_METHOD(void, setServerUrl, (const QUrl &url), (override));
    MOCK_METHOD(bool, isInRoom, (), (const, override));
    MOCK_METHOD(QString, currentRoomId, (), (const, override));
    MOCK_METHOD(bool, isServerAvailable, (), (const, override));

    // Additional NetworkManager specific methods (for extended testing)
    MOCK_METHOD(void, setAuthToken, (const QString &token), (override));
    MOCK_METHOD(void, clearAuthToken, (), (override));
    MOCK_METHOD(QString, authToken, (), (const, override));
    MOCK_METHOD(void, setMaxRetries, (int retries), (override));
    MOCK_METHOD(int, maxRetries, (), (const, override));
    MOCK_METHOD(void, setRetryBaseDelay, (int delayMs), (override));
    MOCK_METHOD(int, retryBaseDelay, (), (const, override));
    MOCK_METHOD(void, setSslMode, (SslMode mode), (override));
    MOCK_METHOD(SslMode, sslMode, (), (const, override));
    MOCK_METHOD(QJsonDocument, parseJson, (const QByteArray &data), (override));
};

#endif // MOCK_NETWORKMANAGER_H