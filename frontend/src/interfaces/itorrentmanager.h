/**
 * @file itorrentmanager.h
 * @brief Абстрактный интерфейс для TorrentManager
 * 
 * Позволяет подменять реализацию для тестирования через gmock.
 */

#ifndef ITORRENTMANAGER_H
#define ITORRENTMANAGER_H

#include <QObject>
#include <QJsonObject>
#include <QJsonArray>
#include <QByteArray>

/**
 * @class ITorrentManager
 * @brief Абстрактный интерфейс для управления торрентами
 * 
 * Наследуется от QObject для поддержки сигналов/слотов.
 * Реализации должны обеспечивать все методы для работы с торрентами.
 */
class ITorrentManager : public QObject
{
    Q_OBJECT

public:
    explicit ITorrentManager(QObject *parent = nullptr) : QObject(parent) {}
    virtual ~ITorrentManager() = default;

    /**
     * @brief Получить ID текущего выбранного торрента
     * @return ID торрента
     */
    virtual QString currentTorrentId() const = 0;
    
    /**
     * @brief Установить ID текущего торрента
     * @param id ID торрента
     */
    virtual void setCurrentTorrentId(const QString &id) = 0;
    
    /**
     * @brief Получить URL потока для торрента
     * @param torrentId ID торрента
     * @return Полный URL для воспроизведения
     */
    virtual QString streamUrl(const QString &torrentId) const = 0;

public slots:
    /**
     * @brief Добавить торрент по magnet-ссылке
     * @param magnetUri Magnet-ссылка
     */
    virtual void addTorrent(const QString &magnetUri) = 0;

    /**
     * @brief Добавить торрент из файла .torrent
     * @param torrentData Содержимое файла .torrent (bencoded)
     */
    virtual void addTorrentFile(const QByteArray &torrentData) = 0;
    
    /**
     * @brief Удалить торрент по ID
     * @param id ID торрента
     */
    virtual void removeTorrent(const QString &id) = 0;
    
    /**
     * @brief Запросить список торрентов
     */
    virtual void listTorrents() = 0;
    
    /**
     * @brief Запросить список файлов торрента
     * @param torrentId ID торрента
     */
    virtual void getFiles(const QString &torrentId) = 0;
    
    /**
     * @brief Выбрать файл для воспроизведения
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     */
    virtual void selectFile(const QString &torrentId, int fileIndex) = 0;
    
    /**
     * @brief Обработка получения списка торрентов
     * @param torrents JSON массив торрентов
     */
    virtual void onTorrentListReceived(const QJsonArray &torrents) = 0;
    
    /**
     * @brief Обработка получения списка файлов
     * @param torrentId ID торрента
     * @param files JSON массив файлов
     */
    virtual void onFilesReceived(const QString &torrentId, const QJsonArray &files) = 0;

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
};

#endif // ITORRENTMANAGER_H