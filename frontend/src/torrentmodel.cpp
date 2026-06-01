/**
 * @file torrentmodel.cpp
 * @brief Реализация модели данных для списка торрентов
 */

#include "torrentmodel.h"

#include <QDebug>
#include <QJsonArray>

// ── Вспомогательные функции ───────────────────────────────────────────

/**
 * @brief Форматирование размера в байтах в читаемый вид
 * @param bytes Размер в байтах
 * @return Строка вида "1.5 GB"
 */
static QString formatBytes(qint64 bytes)
{
    if (bytes < 1024) {
        return QString("%1 B").arg(bytes);
    } else if (bytes < 1024 * 1024) {
        return QString("%1 KB").arg(bytes / 1024.0, 0, 'f', 1);
    } else if (bytes < 1024 * 1024 * 1024) {
        return QString("%1 MB").arg(bytes / (1024.0 * 1024.0), 0, 'f', 1);
    } else {
        return QString("%1 GB").arg(bytes / (1024.0 * 1024.0 * 1024.0), 0, 'f', 2);
    }
}

TorrentModel::TorrentModel(QObject *parent)
    : QAbstractListModel(parent)
{
    qDebug() << "TorrentModel: создана";
}

TorrentModel::~TorrentModel()
{
    qDebug() << "TorrentModel: уничтожена";
}

// ── QAbstractListModel interface ──────────────────────────────────────

int TorrentModel::rowCount(const QModelIndex &parent) const
{
    // Для списковых моделей родитель всегда недействителен
    if (parent.isValid()) {
        return 0;
    }
    return m_torrents.size();
}

QVariant TorrentModel::data(const QModelIndex &index, int role) const
{
    // Проверяем валидность индекса
    if (!index.isValid() || index.row() < 0 || index.row() >= m_torrents.size()) {
        return QVariant();
    }

    const TorrentInfo &torrent = m_torrents.at(index.row());

    switch (role) {
    case IdRole:
        return torrent.id;

    case NameRole:
        return torrent.name;

    case ProgressRole:
        return torrent.progress;

    case StatusRole:
        return torrent.status;

    case SizeRole:
        return static_cast<qulonglong>(torrent.size);

    case DownloadedRole:
        return static_cast<qulonglong>(torrent.downloaded);

    case UploadSpeedRole:
        return static_cast<qulonglong>(torrent.uploadSpeed);

    case DownloadSpeedRole:
        return static_cast<qulonglong>(torrent.downloadSpeed);

    case Qt::DisplayRole:
        // Формируем отображаемый текст
        return QString("%1 (%2%)")
            .arg(torrent.name)
            .arg(torrent.progress * 100, 0, 'f', 1);

    case Qt::ToolTipRole:
        // Подробная информация при наведении
        return QString("ID: %1\nИмя: %2\nПрогресс: %3%\nСтатус: %4\nРазмер: %5")
            .arg(torrent.id)
            .arg(torrent.name)
            .arg(torrent.progress * 100, 0, 'f', 1)
            .arg(torrent.status)
            .arg(formatBytes(torrent.size));

    default:
        return QVariant();
    }
}

QVariant TorrentModel::headerData(int section, Qt::Orientation orientation, int role) const
{
    Q_UNUSED(section)

    if (role != Qt::DisplayRole) {
        return QVariant();
    }

    if (orientation == Qt::Horizontal) {
        return tr("Торрент");
    }

    return QVariant();
}

QHash<int, QByteArray> TorrentModel::roleNames() const
{
    QHash<int, QByteArray> roles;
    roles[IdRole] = "id";
    roles[NameRole] = "name";
    roles[ProgressRole] = "progress";
    roles[StatusRole] = "status";
    roles[SizeRole] = "size";
    roles[DownloadedRole] = "downloaded";
    roles[UploadSpeedRole] = "uploadSpeed";
    roles[DownloadSpeedRole] = "downloadSpeed";
    roles[DisplayRole] = "display";
    return roles;
}

// ── Методы управления моделью ─────────────────────────────────────────

void TorrentModel::addTorrent(const TorrentInfo &torrent)
{
    // Проверяем, нет ли уже торрента с таким ID
    if (contains(torrent.id)) {
        qWarning() << "TorrentModel: торрент с ID" << torrent.id << "уже существует";
        updateTorrent(torrent.id, torrent);
        return;
    }

    int row = m_torrents.size();
    beginInsertRows(QModelIndex(), row, row);
    m_torrents.append(torrent);
    endInsertRows();

    emit torrentAdded(row);
    qDebug() << "TorrentModel: добавлен торрент" << torrent.name;
}

void TorrentModel::addTorrentFromJson(const QJsonObject &json)
{
    addTorrent(TorrentInfo::fromJson(json));
}

bool TorrentModel::removeTorrent(const QString &id)
{
    int index = indexOf(id);
    if (index < 0) {
        qWarning() << "TorrentModel: торрент с ID" << id << "не найден";
        return false;
    }

    beginRemoveRows(QModelIndex(), index, index);
    m_torrents.removeAt(index);
    endRemoveRows();

    emit torrentRemoved(index);
    qDebug() << "TorrentModel: удалён торрент с ID" << id;
    return true;
}

bool TorrentModel::updateTorrent(const QString &id, const TorrentInfo &torrent)
{
    int index = indexOf(id);
    if (index < 0) {
        qWarning() << "TorrentModel: торрент с ID" << id << "не найден для обновления";
        return false;
    }

    m_torrents[index] = torrent;

    // Уведомляем об изменении данных
    QModelIndex modelIndex = createIndex(index, 0);
    emit dataChanged(modelIndex, modelIndex);

    emit torrentUpdated(index);
    qDebug() << "TorrentModel: обновлён торрент" << torrent.name;
    return true;
}

bool TorrentModel::updateTorrentFromJson(const QJsonObject &json)
{
    QString id = json["id"].toString();
    if (id.isEmpty()) {
        qWarning() << "TorrentModel: JSON без ID для обновления";
        return false;
    }

    return updateTorrent(id, TorrentInfo::fromJson(json));
}

TorrentInfo TorrentModel::torrentAt(int index) const
{
    if (index < 0 || index >= m_torrents.size()) {
        return TorrentInfo();
    }
    return m_torrents.at(index);
}

TorrentInfo TorrentModel::torrentById(const QString &id) const
{
    int index = indexOf(id);
    if (index < 0) {
        return TorrentInfo();
    }
    return m_torrents.at(index);
}

int TorrentModel::indexOf(const QString &id) const
{
    for (int i = 0; i < m_torrents.size(); ++i) {
        if (m_torrents.at(i).id == id) {
            return i;
        }
    }
    return -1;
}

bool TorrentModel::contains(const QString &id) const
{
    return indexOf(id) >= 0;
}

void TorrentModel::clear()
{
    beginResetModel();
    m_torrents.clear();
    endResetModel();

    qDebug() << "TorrentModel: очищена";
}

void TorrentModel::loadFromJson(const QJsonArray &torrents)
{
    beginResetModel();
    m_torrents.clear();

    for (const QJsonValue &value : torrents) {
        if (value.isObject()) {
            m_torrents.append(TorrentInfo::fromJson(value.toObject()));
        }
    }

    endResetModel();

    qDebug() << "TorrentModel: загружено" << m_torrents.size() << "торрентов";
}
