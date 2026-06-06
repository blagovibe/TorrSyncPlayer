/**
 * @file torrentmanager.cpp
 * @brief Реализация менеджера управления торрентами
 */

#include "torrentmanager.h"
#include "networkmanager.h"
#include "torrentmodel.h"

#include <QDebug>

TorrentManager::TorrentManager(NetworkManager *network, TorrentModel *model, QObject *parent)
    : QObject(parent)
    , m_network(network)
    , m_torrentModel(model)
{
    qDebug() << "TorrentManager: инициализирован";
}

TorrentManager::~TorrentManager()
{
    qDebug() << "TorrentManager: уничтожен";
}

QString TorrentManager::streamUrl(const QString &torrentId) const
{
    return m_network->streamUrl(torrentId);
}

void TorrentManager::addTorrent(const QString &magnetUri)
{
    // Простая валидация magnet-ссылки
    if (magnetUri.isEmpty()) {
        emit error(tr("Magnet-ссылка не может быть пустой"));
        return;
    }
    
    // Проверка максимальной длины magnet URI (8192 символа)
    const int maxMagnetUriLength = 8192;
    if (magnetUri.length() > maxMagnetUriLength) {
        emit error(tr("Magnet-ссылка слишком длинная (максимум %1 символов)").arg(maxMagnetUriLength));
        return;
    }
    
    if (!magnetUri.startsWith("magnet:?")) {
        emit error(tr("Некорректная magnet-ссылка. Должна начинаться с 'magnet:?'"));
        return;
    }
    
    m_network->addTorrent(magnetUri);
    qDebug() << "TorrentManager: запрошено добавление торрента";
}

void TorrentManager::removeTorrent(const QString &id)
{
    if (id.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }
    
    m_network->removeTorrent(id);
    
    // Очищаем текущий торрент если он был удалён
    if (m_currentTorrentId == id) {
        m_currentTorrentId.clear();
    }
    
    qDebug() << "TorrentManager: запрошено удаление торрента" << id;
}

void TorrentManager::listTorrents()
{
    m_network->listTorrents();
    qDebug() << "TorrentManager: запрошен список торрентов";
}

void TorrentManager::getFiles(const QString &torrentId)
{
    if (torrentId.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }
    
    m_currentTorrentId = torrentId;
    m_network->getFiles(torrentId);
    qDebug() << "TorrentManager: запрошен список файлов для" << torrentId;
}

void TorrentManager::selectFile(const QString &torrentId, int fileIndex)
{
    if (torrentId.isEmpty()) {
        emit error(tr("ID торрента не может быть пустым"));
        return;
    }
    
    if (fileIndex < 0) {
        emit error(tr("Некорректный индекс файла"));
        return;
    }
    
    m_network->selectFile(torrentId, fileIndex);

    qDebug() << "TorrentManager: выбран файл" << fileIndex << "для" << torrentId;
}

void TorrentManager::onTorrentListReceived(const QJsonArray &torrents)
{
    if (m_torrentModel) {
        m_torrentModel->loadFromJson(torrents);
    }
    qDebug() << "TorrentManager: получено торрентов:" << torrents.size();
}

void TorrentManager::onFilesReceived(const QString &torrentId, const QJsonArray &files)
{
    Q_UNUSED(torrentId)
    emit filesReceived(torrentId, files);
    qDebug() << "TorrentManager: получено файлов:" << files.size();
}
