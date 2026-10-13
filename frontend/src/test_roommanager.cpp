/**
 * @file test_roommanager.cpp
 * @brief Тесты RoomManager: разбор сигналов синхронизации
 *
 * RoomManager ничего не знает про сеть: он разбирает payload, который
 * прислал backend через SSE, и пере-эмитит его дальше. Разбор — единственное
 * место, где может испортиться синхронизация всего, что видно пользователю,
 * поэтому он и проверяется здесь, без сервера.
 */

#include <QtTest/QtTest>
#include <QJsonObject>
#include <QSignalSpy>

#include "roommanager.h"
#include "networkmanager.h"

/**
 * @brief Тест разбора сигнала синхронизации
 *
 * Покрывает форму payload, которую реально шлёт backend
 * (handlers_sync.go → BroadcastSync): действие в "action", состояние
 * воспроизведения — в "status".
 */
class TestRoomManager : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase();

    void signalPlay_emitsSyncAction();
    void signalPause_emitsSyncAction();
    void signalSeek_emitsPositionFromStatus();
    void signalSeek_fallsBackToTopLevelPosition();
    void signalUnknownAction_isIgnored();

    // ── Буферизация (bug-0011) ──────────────────────────────────────────
    void signalBuffering_true_emitsBufferingChanged();
    void signalBuffering_false_emitsBufferingChanged();
    void signalBuffering_fallsBackToTopLevelFlag();
    void signalBuffering_withoutFlag_isIgnored();
    void signalBuffering_doesNotEmitSyncAction();
    void signalBuffering_leavesPositionUntouched();

    // ── Сверка состояния после реконнекта (bug-0012) ─────────────────────
    void reconcileStatus_emitsStateWithServerValues();
    void reconcileStatus_withNegativePosition_isRejected();
    void reconcileStatus_withNonFinitePosition_isRejected();
    void reconcileRoomInfo_marksSelfAsHostWhenHostUserIdMatches();
    void reconcileRoomInfo_marksSelfAsNonHostWhenHostIsSomeoneElse();
    void reconcileRoomInfo_withoutClientId_keepsPreviousHostFlag();

    // ── Контракт с бэкендом (idea-0013) ───────────────────────────────────
    void parsesBackendPeerJoinedPayload();
    void parsesBackendSeekPayloadWithBothPositions();
    void parsesBackendBufferingFalsePayload();

    // ── Список участников (idea-0017) ─────────────────────────────────────
    void buildsRosterFromRoomInfo();
    void buildsRosterFromPeerListEvent();
    void rosterReplacesWholesaleRatherThanMerging();
    void applyingSameRosterTwiceIsIdempotent();
    void attributesBufferingToTheReportingPeer();
    void lateJoinerSeesExistingPeersAndFindsTheirBuffering();
    void dropsRosterEntriesWithoutPeerId();
    void lagIsSignedAgainstRoomPosition();

    // ── Код приглашения (idea-0016) ───────────────────────────────────────
    void test_makeInviteCodeJoinsRoomIdAndPassword();
    void test_makeInviteCodeWithEmptyPasswordKeepsSeparator();
    void test_parseInviteCodeSplitsOnFirstColon();
    void test_parseInviteCodeKeepsColonsInsidePassword();
    void test_parseInviteCodeWithoutSeparatorMeansNoPassword();
    void test_parseInviteCodeTrailingColonMeansEmptyPassword();
    void test_parseInviteCodeTrimsSurroundingWhitespace();
    void test_parseInviteCodeRejectsMalformedId();
    void test_looksLikeRoomIdMatchesServerRule();
    void test_inviteCodeRoundTrip();
    void test_parseInviteCodeToleratesNullOutputs();

    /// Гонка: /rooms/info отвечает снимком на момент запроса, а peer_list
    /// может прийти раньше этого ответа. Старый ответ не должен стирать пира.
    void staleRoomInfoSnapshotDoesNotEraseNewerPeerList();
    void acceptsRosterSnapshotWithoutVersion();

private:
    NetworkManager *m_network = nullptr;
    RoomManager *m_room = nullptr;

    static QJsonObject makeStatus(double position, bool isPlaying, bool buffering);
    static QJsonObject makeRoomInfo(const QString &hostUserId, int peerCount);
};

void TestRoomManager::initTestCase()
{
    m_network = new NetworkManager(this);
    m_room = new RoomManager(m_network, this);
    // RoomManager отказывается работать вне комнаты, а комната ставится
    // только по ответам сервера. Ставим её напрямую — тест проверяет разбор
    // сигнала, а не путь получения комнаты.
    m_room->onRoomCreated("room-under-test");
}

void TestRoomManager::signalPlay_emitsSyncAction()
{
    QSignalSpy spy(m_room, &RoomManager::syncAction);

    QJsonObject status;
    status["isPlaying"] = true;
    status["position"] = 12.5;

    QJsonObject signal;
    signal["action"] = "play";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toString(), QString("play"));
}

void TestRoomManager::signalPause_emitsSyncAction()
{
    QSignalSpy spy(m_room, &RoomManager::syncAction);

    QJsonObject signal;
    signal["action"] = "pause";

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toString(), QString("pause"));
}

void TestRoomManager::signalSeek_emitsPositionFromStatus()
{
    QSignalSpy spy(m_room, &RoomManager::syncAction);

    // Позиция лежит внутри status — так её шлёт backend. Раньше читался
    // верхний уровень, которого в payload нет, и приходил 0.
    QJsonObject status;
    status["position"] = 42.5;

    QJsonObject signal;
    signal["action"] = "seek";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toString(), QString("seek"));
    QCOMPARE(spy.first().at(1).toDouble(), 42.5);
}

void TestRoomManager::signalSeek_fallsBackToTopLevelPosition()
{
    QSignalSpy spy(m_room, &RoomManager::syncAction);

    QJsonObject signal;
    signal["action"] = "seek";
    signal["position"] = 7.0;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(1).toDouble(), 7.0);
}

void TestRoomManager::signalUnknownAction_isIgnored()
{
    QSignalSpy spy(m_room, &RoomManager::syncAction);

    QJsonObject signal;
    signal["action"] = "drop-database";

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 0);
}

// ── Буферизация (bug-0011) ──────────────────────────────────────────────

void TestRoomManager::signalBuffering_true_emitsBufferingChanged()
{
    QSignalSpy spy(m_room, &RoomManager::bufferingChanged);

    QJsonObject status;
    status["buffering"] = true;
    status["position"] = 100.0;

    QJsonObject signal;
    signal["action"] = "buffering";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toBool(), true);
}

void TestRoomManager::signalBuffering_false_emitsBufferingChanged()
{
    QSignalSpy spy(m_room, &RoomManager::bufferingChanged);

    QJsonObject status;
    status["buffering"] = false;

    QJsonObject signal;
    signal["action"] = "buffering";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toBool(), false);
}

void TestRoomManager::signalBuffering_fallsBackToTopLevelFlag()
{
    QSignalSpy spy(m_room, &RoomManager::bufferingChanged);

    // Backend дублирует флаг и на верхнем уровне payload — разбор обязан
    // пережить любую из двух форм.
    QJsonObject signal;
    signal["action"] = "buffering";
    signal["buffering"] = true;

    m_room->onSignalReceived(signal);

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toBool(), true);
}

void TestRoomManager::signalBuffering_withoutFlag_isIgnored()
{
    QSignalSpy spy(m_room, &RoomManager::bufferingChanged);

    QJsonObject signal;
    signal["action"] = "buffering";

    m_room->onSignalReceived(signal);

    // Без флага нечего применять: молчаливый resume потерял бы момент, когда
    // пир догнал, поэтому событие отбрасывается.
    QCOMPARE(spy.count(), 0);
}

void TestRoomManager::signalBuffering_doesNotEmitSyncAction()
{
    QSignalSpy actionSpy(m_room, &RoomManager::syncAction);
    QSignalSpy bufferingSpy(m_room, &RoomManager::bufferingChanged);

    QJsonObject status;
    status["buffering"] = true;

    QJsonObject signal;
    signal["action"] = "buffering";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    // Буферизация не должна выглядеть как команда play/pause/seek: иначе
    // она записалась бы в ту же ветку и подменяла собой решение хоста.
    QCOMPARE(actionSpy.count(), 0);
    QCOMPARE(bufferingSpy.count(), 1);
}

void TestRoomManager::signalBuffering_leavesPositionUntouched()
{
    QSignalSpy actionSpy(m_room, &RoomManager::syncAction);

    QJsonObject status;
    status["buffering"] = true;
    status["position"] = 500.0;

    QJsonObject signal;
    signal["action"] = "buffering";
    signal["status"] = status;

    m_room->onSignalReceived(signal);

    // Позиция из payload буферизации не должна превращаться в перемотку.
    QCOMPARE(actionSpy.count(), 0);
}

// ── Сверка состояния после реконнекта (bug-0012) ───────────────────────────

QJsonObject TestRoomManager::makeStatus(double position, bool isPlaying, bool buffering)
{
    QJsonObject status;
    status["position"] = position;
    status["isPlaying"] = isPlaying;
    status["buffering"] = buffering;
    status["duration"] = 3600.0;
    return status;
}

QJsonObject TestRoomManager::makeRoomInfo(const QString &hostUserId, int peerCount)
{
    QJsonObject info;
    info["id"] = QStringLiteral("room-under-test");
    info["name"] = QStringLiteral("Test Room");
    info["hostId"] = QStringLiteral("host-1");
    info["hostUserId"] = hostUserId;
    info["peerCount"] = peerCount;
    return info;
}

void TestRoomManager::reconcileStatus_emitsStateWithServerValues()
{
    QSignalSpy spy(m_room, &RoomManager::stateReconciled);

    m_room->onSyncStatusReceived(makeStatus(137.25, true, false));

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toDouble(), 137.25);
    QCOMPARE(spy.first().at(1).toBool(), true);
    QCOMPARE(spy.first().at(2).toBool(), false);
}

void TestRoomManager::reconcileStatus_withNegativePosition_isRejected()
{
    QSignalSpy spy(m_room, &RoomManager::stateReconciled);

    m_room->onSyncStatusReceived(makeStatus(-5.0, true, false));

    // Позиция из ответа — это данные, а не приказ: отрицательное значение
    // означало бы увести плеер в несуществующее место.
    QCOMPARE(spy.count(), 0);
}

void TestRoomManager::reconcileStatus_withNonFinitePosition_isRejected()
{
    QSignalSpy spy(m_room, &RoomManager::stateReconciled);

    QJsonObject status;
    status["isPlaying"] = true;
    status["buffering"] = false;
    // JSON не умеет NaN, поэтому строка вместо числа: toDouble() даёт 0, но
    // isDouble() ложно — такой ответ обязан быть отброшен, а не принят за 0.
    status["position"] = QStringLiteral("not-a-number");

    m_room->onSyncStatusReceived(status);

    QCOMPARE(spy.count(), 0);
}

void TestRoomManager::reconcileRoomInfo_marksSelfAsHostWhenHostUserIdMatches()
{
    m_network->setClientId(QStringLiteral("self-client"));
    QSignalSpy spy(m_room, &RoomManager::rosterChanged);

    m_room->onRoomInfoReceived(makeRoomInfo(QStringLiteral("self-client"), 3));

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(0).toInt(), 3);
    QCOMPARE(spy.first().at(2).toBool(), true);
    QVERIFY(m_room->isHost());
}

void TestRoomManager::reconcileRoomInfo_marksSelfAsNonHostWhenHostIsSomeoneElse()
{
    m_network->setClientId(QStringLiteral("self-client"));
    QSignalSpy spy(m_room, &RoomManager::rosterChanged);

    // Пока мы были отключены, хостом мог стать другой участник.
    m_room->onRoomInfoReceived(makeRoomInfo(QStringLiteral("someone-else"), 5));

    QCOMPARE(spy.count(), 1);
    QCOMPARE(spy.first().at(2).toBool(), false);
    QVERIFY(!m_room->isHost());
}

void TestRoomManager::reconcileRoomInfo_withoutClientId_keepsPreviousHostFlag()
{
    // Роль не выдумывается: без собственного clientId сравнивать hostUserId
    // не с чем, и молча объявлять себя хостом (или не-хостом) означало бы
    // выдумать факт. Снимаем значение ДО вызова и сверяем после.
    m_network->setClientId(QStringLiteral("self-client"));
    m_room->onRoomInfoReceived(makeRoomInfo(QStringLiteral("someone-else"), 4));
    const bool before = m_room->isHost();

    m_network->setClientId(QString());
    m_room->onRoomInfoReceived(makeRoomInfo(QStringLiteral("a-different-host"), 9));

    QCOMPARE(m_room->isHost(), before);
}

// ── Контракт с бэкендом (idea-0013) ────────────────────────────────────────
//
// The payloads below are the exact bytes the Go structs produce, taken from
// backend/internal/models/events_test.go. The rest of this file builds its own
// JSON, which is fine for testing the parser but proves nothing about the
// server actually speaking the same shape. These three close that gap.

void TestRoomManager::parsesBackendPeerJoinedPayload()
{
    // {"type":"peer_joined","data":{...}} — note "peerID", not "peerId".
    QSignalSpy joinedSpy(m_room, &RoomManager::peerJoined);

    const QByteArray frame =
        R"({"type":"peer_joined","data":{"peerID":"peer-abc","userID":"guest-1",)"
        R"("isHost":false,"peerCount":1}})";

    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());

    QCOMPARE(joinedSpy.count(), 1);
    QCOMPARE(joinedSpy.first().at(0).toString(), QStringLiteral("peer-abc"));
}

void TestRoomManager::parsesBackendSeekPayloadWithBothPositions()
{
    // A seek carries the position the host asked for (90) at the top level and
    // where smoothing moved the room (30) inside status. The client reads
    // status.position first — that is the room's real state.
    QSignalSpy actionSpy(m_room, &RoomManager::syncAction);

    const QByteArray frame =
        R"({"action":"seek","position":90,"status":{"isPlaying":true,"position":30,)"
        R"("duration":3600,"timestamp":1700000000000,"buffering":false},"initiator":"host-1"})";

    m_room->onSignalReceived(QJsonDocument::fromJson(frame).object());

    QCOMPARE(actionSpy.count(), 1);
    QCOMPARE(actionSpy.first().at(0).toString(), QStringLiteral("seek"));
    QCOMPARE(actionSpy.first().at(1).toDouble(), 30.0);
}

void TestRoomManager::parsesBackendBufferingFalsePayload()
{
    // buffering:false is the message that lifts a room stalled behind a peer
    // that caught up (idea-0018). With omitempty on a plain bool it would
    // vanish from the payload and this test would fail.
    QSignalSpy bufferingSpy(m_room, &RoomManager::bufferingChanged);

    const QByteArray frame =
        R"({"action":"buffering","buffering":false,"status":{"isPlaying":true,)"
        R"("position":30,"duration":3600,"timestamp":1700000000000,"buffering":false},)"
        R"("initiator":"guest-1"})";

    m_room->onSignalReceived(QJsonDocument::fromJson(frame).object());

    QCOMPARE(bufferingSpy.count(), 1);
    QCOMPARE(bufferingSpy.first().at(0).toBool(), false);
}

// ── Список участников (idea-0017) ───────────────────────────────────────────

// The payloads below are the exact shapes the Go side produces: `peers` in
// /rooms/info and the peer_list event both carry the same PeerInfo entries
// (backend/internal/p2p/roster_test.go).

static QJsonObject peerEntry(const QString &peerId, const QString &userId,
                             bool isHost, bool buffering)
{
    QJsonObject object;
    object["peerID"] = peerId;
    object["userID"] = userId;
    object["isHost"] = isHost;
    object["buffering"] = buffering;
    return object;
}

void TestRoomManager::buildsRosterFromRoomInfo()
{
    QSignalSpy spy(m_room, &RoomManager::rosterUpdated);

    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 2);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-guest"), QStringLiteral("guest-1"), false, false),
    };

    m_room->onRoomInfoReceived(info);

    QCOMPARE(spy.count(), 1);
    const QVector<PeerEntry> roster = m_room->roster();
    QCOMPARE(roster.size(), 2);

    QCOMPARE(roster.at(0).peerId, QStringLiteral("peer-host"));
    QCOMPARE(roster.at(0).userId, QStringLiteral("host-1"));
    QVERIFY(roster.at(0).isHost);
    QVERIFY(!roster.at(0).buffering);

    QCOMPARE(roster.at(1).userId, QStringLiteral("guest-1"));
    QVERIFY(!roster.at(1).isHost);
}

void TestRoomManager::buildsRosterFromPeerListEvent()
{
    QSignalSpy spy(m_room, &RoomManager::rosterUpdated);

    // The wrapping is the same shape as any other room event.
    const QByteArray frame =
        R"({"type":"peer_list","data":{"peers":[)"
        R"({"peerID":"peer-a","userID":"user-a","isHost":true,"buffering":false},)"
        R"({"peerID":"peer-b","userID":"user-b","isHost":false,"buffering":true}]}})";

    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());

    QCOMPARE(spy.count(), 1);
    const QVector<PeerEntry> roster = m_room->roster();
    QCOMPARE(roster.size(), 2);
    QVERIFY(roster.at(1).buffering);
}

void TestRoomManager::rosterReplacesWholesaleRatherThanMerging()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 2);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-guest"), QStringLiteral("guest-1"), false, false),
    };
    m_room->onRoomInfoReceived(info);
    QCOMPARE(m_room->roster().size(), 2);

    // A later snapshot has one peer fewer. If the client merged instead of
    // replacing, the departed guest would linger forever.
    const QByteArray frame =
        R"({"type":"peer_list","data":{"peers":[)"
        R"({"peerID":"peer-host","userID":"host-1","isHost":true,"buffering":false}]}})";
    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());

    const QVector<PeerEntry> roster = m_room->roster();
    QCOMPARE(roster.size(), 1);
    QCOMPARE(roster.at(0).userId, QStringLiteral("host-1"));
}

// peer_list is a full snapshot, so applying it twice must be a no-op. This is
// the property that lets it land on top of the post-reconnect reconciliation
// (bug-0012) without being applied twice.
void TestRoomManager::applyingSameRosterTwiceIsIdempotent()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 2);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-guest"), QStringLiteral("guest-1"), false, true),
    };
    m_room->onRoomInfoReceived(info);

    const QByteArray frame =
        R"({"type":"peer_list","data":{"peers":[)"
        R"({"peerID":"peer-host","userID":"host-1","isHost":true,"buffering":false},)"
        R"({"peerID":"peer-guest","userID":"guest-1","isHost":false,"buffering":true}]}})";

    QSignalSpy spy(m_room, &RoomManager::rosterUpdated);
    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());
    const QVector<PeerEntry> afterFirst = m_room->roster();

    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());
    const QVector<PeerEntry> afterSecond = m_room->roster();

    QCOMPARE(afterSecond.size(), afterFirst.size());
    for (int i = 0; i < afterFirst.size(); ++i) {
        QCOMPARE(afterSecond.at(i).peerId, afterFirst.at(i).peerId);
        QCOMPARE(afterSecond.at(i).buffering, afterFirst.at(i).buffering);
    }
}

// SyncEvent carries Initiator = userID, not peerID, so attribution has to go
// through the roster.
void TestRoomManager::attributesBufferingToTheReportingPeer()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 3);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-a"), QStringLiteral("user-a"), false, false),
        peerEntry(QStringLiteral("peer-b"), QStringLiteral("user-b"), false, false),
    };
    m_room->onRoomInfoReceived(info);

    const QByteArray frame =
        R"({"action":"buffering","buffering":true,"status":{"isPlaying":true,)"
        R"("position":10,"duration":100,"timestamp":1700000000000,"buffering":true},)"
        R"("initiator":"user-b"})";

    m_room->onSignalReceived(QJsonDocument::fromJson(frame).object());

    const QVector<PeerEntry> roster = m_room->roster();
    QCOMPARE(roster.size(), 3);
    QVERIFY(!roster.at(0).buffering);      // the host did not report
    QVERIFY(!roster.at(1).buffering);      // user-a is innocent
    QVERIFY(roster.at(2).buffering);       // user-b reported
}

// The invariant the whole design exists for: a client that joined late never
// saw peer_joined for anyone, yet must still find an existing peer and see it
// buffering.
void TestRoomManager::lateJoinerSeesExistingPeersAndFindsTheirBuffering()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 2);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-guest"), QStringLiteral("early-1"), false, false),
    };
    m_room->onRoomInfoReceived(info);

    QVERIFY(m_room->roster().size() == 2);

    // The peer that was already there reports buffering.
    const QByteArray frame =
        R"({"action":"buffering","buffering":true,"status":{"isPlaying":true,)"
        R"("position":10,"duration":100,"timestamp":1700000000000,"buffering":true},)"
        R"("initiator":"early-1"})";

    m_room->onSignalReceived(QJsonDocument::fromJson(frame).object());

    bool found = false;
    for (const PeerEntry &entry : m_room->roster()) {
        if (entry.userId == QStringLiteral("early-1")) {
            found = true;
            QVERIFY(entry.buffering);
        }
    }
    QVERIFY(found);
}

// An entry with no peerID cannot be addressed and would collide with another
// anonymous entry in the UI, so it is dropped rather than shown.
void TestRoomManager::dropsRosterEntriesWithoutPeerId()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 2);
    QJsonObject anonymous;
    anonymous["userID"] = QStringLiteral("ghost");
    anonymous["isHost"] = false;
    anonymous["buffering"] = false;

    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        anonymous,
    };

    m_room->onRoomInfoReceived(info);

    QCOMPARE(m_room->roster().size(), 1);
    QCOMPARE(m_room->roster().at(0).userId, QStringLiteral("host-1"));
}

void TestRoomManager::lagIsSignedAgainstRoomPosition()
{
    m_room->onSyncStatusReceived(makeStatus(100.0, true, false));

    // Behind the room reads positive: we are the one lagging.
    QCOMPARE(m_room->lagSeconds(130.0), 30.0);
    // Ahead reads negative, so the UI can say "опережаем" rather than "lead".
    QCOMPARE(m_room->lagSeconds(70.0), -30.0);
    QCOMPARE(m_room->lagSeconds(100.0), 0.0);
}

// /rooms/info is an asynchronous GET, so its answer is a snapshot from the
// moment the request was served. A peer that joins after that moment can reach
// the client as peer_list FIRST — and the older HTTP response would then erase
// it from the roster. Comparing versions is what prevents that.
void TestRoomManager::staleRoomInfoSnapshotDoesNotEraseNewerPeerList()
{
    // The reconnect fetches /rooms/info; the server snapshots the roster now,
    // while only the host is present.
    QJsonObject staleInfo = makeRoomInfo(QStringLiteral("host-1"), 1);
    staleInfo["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
    };
    staleInfo["rosterSeq"] = 4;

    // Before its response arrives, a guest joins and the event stream says so.
    const QByteArray freshFrame =
        R"({"type":"peer_list","rosterSeq":7,"data":{"rosterSeq":7,"peers":[)"
        R"({"peerID":"peer-host","userID":"host-1","isHost":true,"buffering":false},)"
        R"({"peerID":"peer-guest","userID":"guest-1","isHost":false,"buffering":false}]}})";

    m_room->onRoomEvent(QJsonDocument::fromJson(freshFrame).object());
    QCOMPARE(m_room->roster().size(), 2);

    // Now the older response finally lands. It must not win.
    m_room->onRoomInfoReceived(staleInfo);

    QCOMPARE(m_room->roster().size(), 2);
    QCOMPARE(m_room->roster().at(1).userId, QStringLiteral("guest-1"));

    // And the reverse order works too: the newer snapshot still applies.
    QJsonObject freshInfo = makeRoomInfo(QStringLiteral("host-1"), 3);
    freshInfo["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
        peerEntry(QStringLiteral("peer-guest"), QStringLiteral("guest-1"), false, false),
        peerEntry(QStringLiteral("peer-late"), QStringLiteral("late-1"), false, false),
    };
    freshInfo["rosterSeq"] = 9;
    m_room->onRoomInfoReceived(freshInfo);
    QCOMPARE(m_room->roster().size(), 3);
}

// A server that does not send a version must keep working: the snapshot is
// taken as-is rather than discarded for lacking a field it never promised.
void TestRoomManager::acceptsRosterSnapshotWithoutVersion()
{
    QJsonObject info = makeRoomInfo(QStringLiteral("host-1"), 1);
    info["peers"] = QJsonArray{
        peerEntry(QStringLiteral("peer-host"), QStringLiteral("host-1"), true, false),
    };
    // deliberately no rosterSeq
    m_room->onRoomInfoReceived(info);

    QCOMPARE(m_room->roster().size(), 1);

    const QByteArray frame =
        R"({"type":"peer_list","data":{"peers":[)"
        R"({"peerID":"peer-host","userID":"host-1","isHost":true,"buffering":false}]}})";
    m_room->onRoomEvent(QJsonDocument::fromJson(frame).object());
    QCOMPARE(m_room->roster().size(), 1);
}

// ── Код приглашения ─────────────────────────────────────────────────────
//
// Код — это «ID:пароль», то есть ровно те данные, которые и так уходят в
// запрос входа. Тесты проверяют именно раскладку по полям и отказ на мусоре;
// ничего секретного в коде нет, и проверять тут нечего.

void TestRoomManager::test_makeInviteCodeJoinsRoomIdAndPassword()
{
    const QString id = QStringLiteral("0123456789abcdef0123456789abcdef");
    QCOMPARE(RoomManager::makeInviteCode(id, QStringLiteral("hunter2")),
             QStringLiteral("0123456789abcdef0123456789abcdef:hunter2"));
}

void TestRoomManager::test_makeInviteCodeWithEmptyPasswordKeepsSeparator()
{
    // Двоеточие остаётся, чтобы разбор не спутал комнату без пароля с
    // вводом одного ID, и хост видел, что пароль не забыт, а пуст.
    const QString id = QStringLiteral("0123456789abcdef0123456789abcdef");
    QCOMPARE(RoomManager::makeInviteCode(id, QString()),
             QStringLiteral("0123456789abcdef0123456789abcdef:"));
}

void TestRoomManager::test_parseInviteCodeSplitsOnFirstColon()
{
    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdef:hunter2"),
        &roomId, &password));
    QCOMPARE(roomId, QStringLiteral("0123456789abcdef0123456789abcdef"));
    QCOMPARE(password, QStringLiteral("hunter2"));
}

void TestRoomManager::test_parseInviteCodeKeepsColonsInsidePassword()
{
    // Пароль произвольный: двоеточие в нём — часть пароля, а не разделитель.
    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdef:a:b:c"),
        &roomId, &password));
    QCOMPARE(roomId, QStringLiteral("0123456789abcdef0123456789abcdef"));
    QCOMPARE(password, QStringLiteral("a:b:c"));
}

void TestRoomManager::test_parseInviteCodeWithoutSeparatorMeansNoPassword()
{
    // Вставка одного ID должна входить в комнату без пароля, а не ругаться.
    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdef"),
        &roomId, &password));
    QCOMPARE(roomId, QStringLiteral("0123456789abcdef0123456789abcdef"));
    QVERIFY(password.isEmpty());
}

void TestRoomManager::test_parseInviteCodeTrailingColonMeansEmptyPassword()
{
    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdef:"),
        &roomId, &password));
    QVERIFY(password.isEmpty());
}

void TestRoomManager::test_parseInviteCodeTrimsSurroundingWhitespace()
{
    // Код копируют из мессенджера, там почти всегда есть пробелы и перевод строки.
    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("  0123456789abcdef0123456789abcdef:pw\n"),
        &roomId, &password));
    QCOMPARE(roomId, QStringLiteral("0123456789abcdef0123456789abcdef"));
    QCOMPARE(password, QStringLiteral("pw"));
}

void TestRoomManager::test_parseInviteCodeRejectsMalformedId()
{
    // Проверяем ровно то, что проверяет сервер: 32 hex-символа. Мусор должен
    // отсекаться на клиенте, а not ехать на сервер.
    QString roomId = QStringLiteral("sentinel");
    QString password = QStringLiteral("sentinel");

    QVERIFY(!RoomManager::parseInviteCode(QStringLiteral("short:pw"), &roomId, &password));
    QVERIFY(!RoomManager::parseInviteCode(QString(), &roomId, &password));
    QVERIFY(!RoomManager::parseInviteCode(QStringLiteral("   "), &roomId, &password));
    QVERIFY(!RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdeg:pw"), &roomId, &password));
    QVERIFY(!RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcde:pw"), &roomId, &password));

    // На отказе выходные параметры не трогаем — иначе вызывающий не отличит
    // «разобрали» от «затерли».
    QCOMPARE(roomId, QStringLiteral("sentinel"));
    QCOMPARE(password, QStringLiteral("sentinel"));
}

void TestRoomManager::test_looksLikeRoomIdMatchesServerRule()
{
    QVERIFY(RoomManager::looksLikeRoomId(QStringLiteral("0123456789abcdef0123456789abcdef")));
    QVERIFY(RoomManager::looksLikeRoomId(QStringLiteral("ABCDEF0123456789abcdef0123456789")));
    QVERIFY(!RoomManager::looksLikeRoomId(QStringLiteral("0123456789abcdef0123456789abcde")));
    QVERIFY(!RoomManager::looksLikeRoomId(QStringLiteral("0123456789abcdef0123456789abcdef0")));
    QVERIFY(!RoomManager::looksLikeRoomId(QStringLiteral("g123456789abcdef0123456789abcdef")));
    QVERIFY(!RoomManager::looksLikeRoomId(QString()));
}

void TestRoomManager::test_inviteCodeRoundTrip()
{
    // Собрали код как хост — разобрали как гость: значения должны совпасть.
    const QString id = QStringLiteral("0123456789abcdef0123456789abcdef");
    const QString pw = QStringLiteral("секрет:с двоеточием");

    QString roomId;
    QString password;
    QVERIFY(RoomManager::parseInviteCode(RoomManager::makeInviteCode(id, pw),
                                         &roomId, &password));
    QCOMPARE(roomId, id);
    QCOMPARE(password, pw);
}

void TestRoomManager::test_parseInviteCodeToleratesNullOutputs()
{
    // Вызывающий может интересоваться только ID; указатели должны быть необязательными.
    QVERIFY(RoomManager::parseInviteCode(
        QStringLiteral("0123456789abcdef0123456789abcdef:pw"), nullptr, nullptr));
}

QTEST_MAIN(TestRoomManager)
#include "test_roommanager.moc"
