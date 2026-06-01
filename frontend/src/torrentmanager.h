/**
 * @file torrentmanager.h
 * @brief Менеджер управления торрентами для TorrPlayer
 * 
 * Вынесен из MainWindow для уменьшения размера класса.
 * Управляет добавлением, удалением торрентов и выбором файлов.
 */

#ifndef TORRENTMANAGER_H
#define TORRENTMANAGER_H

#include <QObject>
#include <QJsonObject>
#include <QJsonArray>

// Предварительные объявления
class NetworkManager;
class TorrentModel;

/**
 * @class TorrentManager
 * @brief Менеджер операций с торрентами
 * 
 * Инкапсулирует логику:
 * - Добавление/удаление торрентов
 * - Получение списка файлов
 * - Выбор файла для воспроизведения
 * - Формирование URL потока
 */
class TorrentManager : public QObject
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор менеджера торрентов
     * @param network Менеджер сети для API запросов
     * @param model Модель данных торрентов
     * @param parent Родительский объект
     */
    explicit TorrentManager(NetworkManager *network, TorrentModel *model, QObject *parent = nullptr);
    
    /**
     * @brief Деструктор
     */
    ~TorrentManager();

    /**
     * @brief Получить ID текущего выбранного торрента
     * @return ID торрента
     */
    QString currentTorrentId() const { return m_currentTorrentId; }
    
    /**
     * @brief Установить ID текущего торрента
     * @param id ID торрента
     */
    void setCurrentTorrentId(const QString &id) { m_currentTorrentId = id; }
    
    /**
     * @brief Получить URL потока для торрента
     * @param torrentId ID торрента
     * @return Полный URL для воспроизведения
     */
    QString streamUrl(const QString &torrentId) const;

public slots:
    /**
     * @brief Добавить торрент по magnet-ссылке
     * @param magnetUri Magnet-ссылка
     */
    void addTorrent(const QString &magnetUri);
    
    /**
     * @brief Удалить торрент по ID
     * @param id ID торрента
     */
    void removeTorrent(const QString &id);
    
    /**
     * @brief Запросить список торрентов
     */
    void listTorrents();
    
    /**
     * @brief Запросить список файлов торрента
     * @param torrentId ID торрента
     */
    void getFiles(const QString &torrentId);
    
    /**
     * @brief Выбрать файл для воспроизведения
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     */
    void selectFile(const QString &torrentId, int fileIndex);
    
    /**
     * @brief Обработка получения списка торрентов
     * @param torrents JSON массив торрентов
     */
    void onTorrentListReceived(const QJsonArray &torrents);
    
    /**
     * @brief Обработка получения списка файлов
     * @param torrentId ID торрента
     * @param files JSON массив файлов
     */
    void onFilesReceived(const QString &torrentId, const QJsonArray &files);

signals:
    /**
     * @brief Торрент добавлен
     * @param torrent JSON объект торрента
     */
    void torrentAdded(const QJsonObject &torrent);
    
    /**
     * @brief Торрент удалён
     * @param id ID удалённого торрента
     */
    void torrentRemoved(const QString &id);
    
    /**
     * @brief Получен список файлов
     * @param torrentId ID торрента
     * @param files JSON массив файлов
     */
    void filesReceived(const QString &torrentId, const QJsonArray &files);
    
    /**
     * @brief Файл выбран для воспроизведения
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     * @param streamUrl URL потока
     */
    void fileSelected(const QString &torrentId, int fileIndex, const QString &streamUrl);
    
    /**
     * @brief Ошибка операции с торрентом
     * @param message Описание ошибки
     */
    void error(const QString &message);

private:
    NetworkManager *m_network;      ///< Менеджер сети
    TorrentModel *m_torrentModel;   ///< Модель торрентов
    QString m_currentTorrentId;     ///< ID текущего торрента
};

#endif // TORRENTMANAGER_H
