/**
 * @file roommanager.cpp
 * @brief Реализация менеджера управления комнатами
 */

#include "roommanager.h"
#include "networkmanager.h"

#include <QDebug>
#include <QtGlobal>
#include <cmath>

RoomManager::RoomManager(NetworkManager *network, QObject *parent)
    : QObject(parent)
    , m_network(network)
{
    // Связи с NetworkManager. Без них RoomManager был мёртвым: он получал
    // события комнаты, но никогда не узнавал, в какой комнате находится, и
    // никогда не сообщал MainWindow о создании/входе/выходе.
    if (m_network) {
        connect(m_network, &NetworkManager::roomCreated,
                this, &RoomManager::onRoomCreated);
        connect(m_network, &NetworkManager::roomJoined,
                this, &RoomManager::onRoomJoined);
        connect(m_network, &NetworkManager::roomLeft,
                this, &RoomManager::onRoomLeft);
        // Сверка после (пере)подключения к комнате. Пока SSE-поток был рван,
        // RoomManager не знал, что комната продолжала жить: кто-то перемотал,
        // хост поставил паузу, кто-то вошёл. Эти ответы — единственный
        // источник правды после разрыва (bug-0012).
        connect(m_network, &NetworkManager::syncStatusReceived,
                this, &RoomManager::onSyncStatusReceived);
        connect(m_network, &NetworkManager::roomInfoReceived,
                this, &RoomManager::onRoomInfoReceived);
    }
    qDebug() << "RoomManager: инициализирован";
}

void RoomManager::onRoomCreated(const QString &roomId)
{
    if (roomId.isEmpty()) {
        qWarning() << "RoomManager: создана комната без id";
        return;
    }
    m_currentRoomId = roomId;
    m_isHost = true;
    qDebug() << "RoomManager: создана комната" << roomId << "(хост)";
    emit roomCreated(roomId);
}

void RoomManager::onRoomJoined(const QString &roomId)
{
    if (roomId.isEmpty()) {
        qWarning() << "RoomManager: вход в комнату без id";
        return;
    }
    m_currentRoomId = roomId;
    m_isHost = false;
    qDebug() << "RoomManager: вошли в комнату" << roomId;
    emit roomJoined(roomId);
}

void RoomManager::onRoomLeft()
{
    m_currentRoomId.clear();
    m_isHost = false;
    qDebug() << "RoomManager: вышли из комнаты";
    emit roomLeft();
}

RoomManager::~RoomManager()
{
    qDebug() << "RoomManager: уничтожен";
}

bool RoomManager::isInRoom() const
{
    return !m_currentRoomId.isEmpty();
}

void RoomManager::createRoom(const QString &name, const QString &password, const QString &torrentId)
{
    if (name.isEmpty()) {
        emit error(tr("Имя комнаты не может быть пустым"));
        return;
    }

    // Комната привязана к торренту (idea-0046): без infohash участникам нечего
    // качать, а сервер вернёт 400.
    if (torrentId.isEmpty()) {
        emit error(tr("Сначала добавьте торрент и выберите файл"));
        return;
    }

    m_network->createRoom(name, password, torrentId);
    qDebug() << "RoomManager: запрошено создание комнаты" << name << "для торрента" << torrentId;
}

void RoomManager::joinRoom(const QString &roomId, const QString &password, const QString &torrentId)
{
    if (roomId.isEmpty()) {
        emit error(tr("ID комнаты не может быть пустым"));
        return;
    }

    if (torrentId.isEmpty()) {
        emit error(tr("Сначала добавьте тот же торрент, который смотрит комната"));
        return;
    }

    m_network->joinRoom(roomId, password, torrentId);
    qDebug() << "RoomManager: запрошено присоединение к комнате" << roomId;
}

void RoomManager::leaveRoom()
{
    if (!isInRoom()) {
        emit error(tr("Не в комнате"));
        return;
    }
    
    m_network->leaveRoom();
    qDebug() << "RoomManager: запрошен выход из комнаты";
}

void RoomManager::syncPlay()
{
    if (!isInRoom()) {
        qWarning() << "RoomManager: попытка syncPlay вне комнаты";
        return;
    }
    
    m_network->syncPlay();
    qDebug() << "RoomManager: синхронизация play";
}

void RoomManager::syncPause()
{
    if (!isInRoom()) {
        qWarning() << "RoomManager: попытка syncPause вне комнаты";
        return;
    }
    
    m_network->syncPause();
    qDebug() << "RoomManager: синхронизация pause";
}

void RoomManager::syncSeek(double position)
{
    if (!isInRoom()) {
        qWarning() << "RoomManager: попытка syncSeek вне комнаты";
        return;
    }
    
    if (position < 0) {
        qWarning() << "RoomManager: некорректная позиция для syncSeek";
        return;
    }
    
    m_network->syncSeek(position);
    qDebug() << "RoomManager: синхронизация seek на позицию" << position;
}

void RoomManager::syncBuffering(bool buffering)
{
    if (!isInRoom()) {
        qWarning() << "RoomManager: попытка syncBuffering вне комнаты";
        return;
    }

    m_network->syncBuffering(buffering);
    qDebug() << "RoomManager: синхронизация buffering" << buffering;
}

void RoomManager::onRoomEvent(const QJsonObject &event)
{
    QString type = event["type"].toString();
    if (type.isEmpty()) return;
    qDebug() << "RoomManager: событие комнаты" << type;

    if (type == "peer_list") {
        // Полный снимок состава комнаты (idea-0017). Именно он закрывает дыру,
        // из-за которой нужен и /rooms/info: клиент, вошедший позже, не видел
        // peer_joined для тех, кто был до него.
        const QJsonObject data = event["data"].toObject();
        applyRoster(parseRoster(data.value(QStringLiteral("peers")).toArray()),
                    data.value(QStringLiteral("rosterSeq")).toInt());
        return;
    }

    if (type == "peer_joined" || type == "peer_left") {
        // Полезная нагрузка лежит вложенно в "data", а идентификатор пира
        // называется "peerID". Раньше читался event["peerId"] — путь и
        // написание ключа не совпадали с сервером, поэтому сигналы уходили
        // с пустым идентификатором.
        const QJsonObject data = event["data"].toObject();
        const QString peerId = data["peerID"].toString();
        if (peerId.isEmpty()) {
            qWarning() << "RoomManager: событие" << type << "без peerID, data:" << data;
        }
        if (type == "peer_joined") {
            emit peerJoined(peerId);
        } else {
            emit peerLeft(peerId);
        }
    }

    emit roomEvent(event);
}

void RoomManager::onSignalReceived(const QJsonObject &signal)
{
    // Бэкенд шлёт синхронизацию так (handlers_sync.go → BroadcastSync):
    //   {"action":"seek", "status":{"isPlaying":…,"position":…,"duration":…,
    //    "timestamp":…}, "initiator":"…"}
    // Позиция лежит В status.position. Раньше читался signal["position"],
    // которого в payload нет, поэтому всегда приходил 0.
    QString action = signal["action"].toString();

    double position = 0.0;
    const QJsonObject status = signal["status"].toObject();
    if (status.contains(QStringLiteral("position"))) {
        position = status["position"].toDouble(0.0);
    } else if (signal.contains(QStringLiteral("position"))) {
        // Запасной путь для более простых форм payload.
        position = signal["position"].toDouble(0.0);
    }

    // Буферизация — отдельный случай: это не команда «играй/стой», а
    // сообщение о состоянии пира. Флаг лежит в status.buffering, но backend
    // дублирует его и на верхнем уровне payload — берём любой из двух путей.
    if (action == "buffering") {
        bool buffering = false;
        if (status.contains(QStringLiteral("buffering"))) {
            buffering = status["buffering"].toBool();
        } else if (signal.contains(QStringLiteral("buffering"))) {
            buffering = signal["buffering"].toBool();
        } else {
            qWarning() << "RoomManager: buffering без флага в payload:" << signal;
            return;
        }
        qDebug() << "RoomManager: получен сигнал buffering" << buffering;

        // Кто именно буферизирует. SyncEvent несёт Initiator = userID, а не
        // peerID, поэтому сопоставляем через roster. Событие peer_list с тем же
        // фактом придёт следом, но список не должен мигать до него — а у того,
        // кто сообщил, статус иначе остался бы устаревшим.
        markBufferingByUserID(signal.value(QStringLiteral("initiator")).toString(), buffering);

        emit bufferingChanged(buffering);
        return;
    }

    // Валидируем action — только play/pause/seek
    static const QStringList validActions = {"play", "pause", "seek"};
    if (!validActions.contains(action)) {
        qWarning() << "RoomManager: невалидный action от пира:" << action;
        return;
    }

    // Валидируем position — неотрицательное конечное число
    if (action == "seek") {
        if (!std::isfinite(position) || position < 0.0) {
            qWarning() << "RoomManager: невалидная позиция от пира:" << position;
            return;
        }
    }

    qDebug() << "RoomManager: получен сигнал" << action << "позиция" << position;
    emit syncAction(action, position);
}

void RoomManager::onSyncStatusReceived(const QJsonObject &status)
{
    // Ответ /sync/status. Значения валидируются ДО применения: ответ с
    // сервера — это данные, а не приказ, и подставить в плеер мусорный
    // position значит увести его в несуществующее место.
    const QJsonValue posValue = status.value(QStringLiteral("position"));
    const double position = posValue.toDouble(0.0);
    if (!posValue.isDouble() || !std::isfinite(position) || position < 0.0) {
        qWarning() << "RoomManager: /sync/status вернул некорректную позицию:" << posValue;
        return;
    }

    const bool isPlaying = status.value(QStringLiteral("isPlaying")).toBool();
    const bool buffering = status.value(QStringLiteral("buffering")).toBool();

    m_serverPosition = position;
    m_serverIsPlaying = isPlaying;
    m_serverBuffering = buffering;

    qDebug() << "RoomManager: сверка состояния — позиция" << position
             << "играет" << isPlaying << "буферизация" << buffering;
    emit stateReconciled(position, isPlaying, buffering);
}

void RoomManager::onRoomInfoReceived(const QJsonObject &info)
{
    const QString hostUserId = info.value(QStringLiteral("hostUserId")).toString();
    const int peerCount = info.value(QStringLiteral("peerCount")).toInt();

    // Хост определяется сравнением с нашим собственным X-Client-ID: ответ
    // приходит с идентификатором ХОСТА, а не с флагом «вы хост». Сравнение
    // с собственным id — единственный способ узнать это надёжно.
    const QString selfId = m_network ? m_network->clientId() : QString();
    m_selfUserId = selfId;
    if (hostUserId.isEmpty() || selfId.isEmpty()) {
        qWarning() << "RoomManager: /rooms/info без hostUserId или без своего clientId"
                   << "— роль хоста не определена, оставляю как была";
    } else {
        m_isHost = (hostUserId == selfId);
    }

    m_peerCount = peerCount;
    qDebug() << "RoomManager: комната" << info.value(QStringLiteral("name")).toString()
             << "участников" << peerCount << "хост" << (m_isHost ? "мы" : "не мы");
    emit rosterChanged(peerCount, hostUserId, m_isHost);

    // ID торрента комнаты (idea-0046). Комната привязана к торренту при создании,
    // и сервер возвращает его в /rooms/info. Если мы не хост и у нас ещё нет
    // этого торрента — UI может автоматически предложить его добавить.
    const QString torrentId = info.value(QStringLiteral("torrentId")).toString();
    if (!torrentId.isEmpty()) {
        emit roomTorrentIdentified(torrentId);
    }

    // Список приходит вместе с ответом о комнате — это первичная заливка для
    // клиента, который только подключился и не видел ни одного peer_joined.
    // Старые серверы поле не отдают: остаёмся с тем, что есть.
    const QJsonArray peers = info.value(QStringLiteral("peers")).toArray();
    if (!peers.isEmpty()) {
        applyRoster(parseRoster(peers), info.value(QStringLiteral("rosterSeq")).toInt());
    }
}

QVector<PeerEntry> RoomManager::parseRoster(const QJsonArray &array)
{
    QVector<PeerEntry> roster;
    roster.reserve(array.size());
    for (const QJsonValue &value : array) {
        const QJsonObject object = value.toObject();
        PeerEntry entry;
        entry.peerId = object.value(QStringLiteral("peerID")).toString();
        entry.userId = object.value(QStringLiteral("userID")).toString();
        entry.username = object.value(QStringLiteral("username")).toString();
        entry.isHost = object.value(QStringLiteral("isHost")).toBool();
        entry.buffering = object.value(QStringLiteral("buffering")).toBool();

        if (entry.peerId.isEmpty()) {
            // Без peerID пира нельзя адресовать и нельзя отличить от соседа.
            qWarning() << "RoomManager: запись roster без peerID, пропускаю:" << object;
            continue;
        }
        roster.append(entry);
    }
    return roster;
}

void RoomManager::applyRoster(const QVector<PeerEntry> &peers, int seq)
{
    // Отбрасываем устаревший снимок. Два источника roster соревнуются по
    // построению: /rooms/info — асинхронный GET, то есть снимок на момент
    // обслуживания запроса, а событие peer_list может прийти раньше этого
    // ответа. Без сравнения версий старый ответ стирал бы пира, только что
    // появившегося в списке (idea-0017).
    //
    // seq == 0 — сервер версию не прислал (старый сервер). Тогда сравнивать
    // нечем, и снимок принимается как есть: это лучше, чем отбросить всё.
    if (seq > 0 && seq < m_rosterSeq) {
        qWarning() << "RoomManager: снимок roster устарел (seq" << seq
                   << "<" << m_rosterSeq << "), игнорирую";
        return;
    }
    if (seq > 0) {
        m_rosterSeq = seq;
    }

    // Снимок полный, поэтому заменяем целиком, а не правим по частям. Так
    // повторное применение того же списка ничего не меняет — это и позволяет
    // peer_list приходить поверх сверки после реконнекта (bug-0012) без
    // двойного применения.
    m_roster = peers;

    // Счётчик и список обязаны сходиться: оба приходят с сервера, оба про
    // один и тот же состав. Расхождение — это дефект на одной из сторон
    // (bug-0021 показал, как это выглядит снаружи), а не повод подгонять одно
    // под другое. Поэтому при расхождении верим списку и сообщаем.
    const int fromRoster = m_roster.size();
    if (m_peerCount != 0 && m_peerCount != fromRoster) {
        qWarning() << "RoomManager: peerCount" << m_peerCount
                   << "не совпадает с числом записей roster" << fromRoster
                   << "— верю списку";
        m_peerCount = fromRoster;
    }

    emit rosterUpdated(m_roster);
}

void RoomManager::markBufferingByUserID(const QString &userID, bool buffering)
{
    if (userID.isEmpty()) {
        return;
    }
    bool changed = false;
    for (PeerEntry &entry : m_roster) {
        if (entry.userId == userID) {
            changed = (entry.buffering != buffering);
            entry.buffering = buffering;
            break;
        }
    }
    if (!changed) {
        // Либо список пуст (ещё не пришёл), либо состояние уже такое — обновлять
        // UI незачем.
        return;
    }
    emit rosterUpdated(m_roster);
}

double RoomManager::lagSeconds(double localPosition) const
{
    return localPosition - m_serverPosition;
}

// ── Код приглашения ─────────────────────────────────────────────────────

QString RoomManager::makeInviteCode(const QString &roomId, const QString &password)
{
    return roomId.trimmed() + ':' + password;
}

bool RoomManager::looksLikeRoomId(const QString &roomId)
{
    // То же правило, что на сервере (validation.ValidateRoomID): ровно 32
    // шестнадцатеричных символа. Дублируем, чтобы мусор отсекался здесь.
    if (roomId.length() != 32) {
        return false;
    }
    for (const QChar c : roomId) {
        const bool hex = (c >= '0' && c <= '9')
                      || (c >= 'a' && c <= 'f')
                      || (c >= 'A' && c <= 'F');
        if (!hex) {
            return false;
        }
    }
    return true;
}

bool RoomManager::parseInviteCode(const QString &code,
                                  QString *roomId,
                                  QString *password)
{
    const QString trimmed = code.trimmed();
    if (trimmed.isEmpty()) {
        return false;
    }

    // Первое двоеточие — граница. Всё после него пароль целиком, включая
    // двоеточия внутри: пароль произвольный, ID комнаты — 32 hex-символа.
    const int sep = trimmed.indexOf(':');
    const QString id  = (sep < 0) ? trimmed : trimmed.left(sep);
    const QString pass = (sep < 0) ? QString() : trimmed.mid(sep + 1);

    if (!looksLikeRoomId(id)) {
        return false;
    }

    if (roomId) {
        *roomId = id;
    }
    if (password) {
        *password = pass;
    }
    return true;
}
