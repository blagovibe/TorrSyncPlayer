/**
 * @file roommanager.cpp
 * @brief Реализация менеджера управления комнатами
 */

#include "roommanager.h"
#include "networkmanager.h"

#include <QDebug>

RoomManager::RoomManager(NetworkManager *network, QObject *parent)
    : QObject(parent)
    , m_network(network)
{
    qDebug() << "RoomManager: инициализирован";
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

    if (type == "peer_joined") {
        QString peerId = event["peerId"].toString();
        emit peerJoined(peerId);
    } else if (type == "peer_left") {
        QString peerId = event["peerId"].toString();
        emit peerLeft(peerId);
    }

    emit roomEvent(event);
}

void RoomManager::onSignalReceived(const QJsonObject &signal)
{
    QString action = signal["action"].toString();
    double position = signal["position"].toDouble();
    
    qDebug() << "RoomManager: получен сигнал" << action;
    emit syncAction(action, position);
}
