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

void RoomManager::createRoom(const QString &name, const QString &password)
{
    if (name.isEmpty()) {
        emit error(tr("Имя комнаты не может быть пустым"));
        return;
    }
    
    m_network->createRoom(name, password);
    qDebug() << "RoomManager: запрошено создание комнаты" << name;
}

void RoomManager::joinRoom(const QString &roomId, const QString &password)
{
    if (roomId.isEmpty()) {
        emit error(tr("ID комнаты не может быть пустым"));
        return;
    }
    
    m_network->joinRoom(roomId, password);
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

void RoomManager::onRoomEvent(const QJsonObject &event)
{
    QString type = event["type"].toString();
    if (type.isEmpty()) return;
    qDebug() << "RoomManager: событие комнаты" << type;

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
