/**
 * @file torrentmodel.h
 * @brief Модель данных для списка торрентов
 * 
 * Реализует QAbstractListModel для отображения списка торрентов
 * в QListView с поддержкой ролей для различных свойств торрента.
 */

#ifndef TORRENTMODEL_H
#define TORRENTMODEL_H

#include <QAbstractListModel>
#include <QVector>
#include <QJsonObject>

/**
 * @struct TorrentInfo
 * @brief Структура с информацией о торренте
 * 
 * Содержит все данные о торренте: ID, название, прогресс загрузки,
 * статус, размер и скорости загрузки/отдачи.
 */
struct TorrentInfo {
    QString id;         ///< Уникальный ID торрента (info hash)
    QString name;       ///< Название торрента
    double progress;    ///< Прогресс загрузки (0.0 - 1.0)
    QString status;     ///< Статус: "downloading", "seeding", "paused", "error"
    qint64 size;        ///< Размер в байтах
    qint64 downloaded;  ///< Скачано байт
    qint64 uploadSpeed; ///< Скорость отдачи (байт/с)
    qint64 downloadSpeed; ///< Скорость загрузки (байт/с)
    
    /**
     * @brief Создание из JSON объекта
     * Парсит JSON ответ от API и заполняет структуру
     * @param json JSON от API
     * @return Заполненная структура
     */
    static TorrentInfo fromJson(const QJsonObject &json) {
        TorrentInfo info;
        info.id = json["id"].toString();
        info.name = json["name"].toString();
        info.progress = json["progress"].toDouble(0.0);
        info.status = json["status"].toString("unknown");
        info.size = json["size"].toVariant().toLongLong();
        info.downloaded = json["downloaded"].toVariant().toLongLong();
        info.uploadSpeed = json["uploadSpeed"].toVariant().toLongLong();
        info.downloadSpeed = json["downloadSpeed"].toVariant().toLongLong();
        return info;
    }
    
    /**
     * @brief Конвертация в JSON объект
     * Сериализует структуру в JSON для отправки на сервер
     * @return JSON представление
     */
    QJsonObject toJson() const {
        QJsonObject json;
        json["id"] = id;
        json["name"] = name;
        json["progress"] = progress;
        json["status"] = status;
        json["size"] = static_cast<double>(size);
        json["downloaded"] = static_cast<double>(downloaded);
        json["uploadSpeed"] = static_cast<double>(uploadSpeed);
        json["downloadSpeed"] = static_cast<double>(downloadSpeed);
        return json;
    }
};

/**
 * @class TorrentModel
 * @brief Модель списка торрентов для QML/QWidget
 * 
 * Реализует QAbstractListModel для отображения списка торрентов.
 * Поддерживает динамическое добавление, удаление и обновление элементов.
 * 
 * Поддерживает следующие роли:
 * - IdRole - ID торрента
 * - NameRole - Название
 * - ProgressRole - Прогресс загрузки (0.0-1.0)
 * - StatusRole - Статус
 * - SizeRole - Размер в байтах
 * - DownloadedRole - Скачано байт
 * - DisplayRole - Отображаемое имя
 */
class TorrentModel : public QAbstractListModel
{
    Q_OBJECT

public:
    /**
     * @brief Роли модели для доступа к данным
     * Используются в QML и делегатах для доступа к свойствам торрента
     */
    enum TorrentRoles {
        IdRole = Qt::UserRole + 1,         ///< ID торрента
        NameRole,                           ///< Название
        ProgressRole,                       ///< Прогресс (0.0-1.0)
        StatusRole,                         ///< Статус
        SizeRole,                           ///< Размер
        DownloadedRole,                     ///< Скачано
        UploadSpeedRole,                    ///< Скорость отдачи
        DownloadSpeedRole,                  ///< Скорость загрузки
        DisplayRole = Qt::DisplayRole       ///< Отображаемый текст
    };
    Q_ENUM(TorrentRoles)

    /**
     * @brief Конструктор модели
     * @param parent Родительский объект
     */
    explicit TorrentModel(QObject *parent = nullptr);
    
    /**
     * @brief Деструктор
     */
    ~TorrentModel() override;

    // ── QAbstractListModel interface ──────────────────────────────────
    
    /**
     * @brief Количество строк в модели
     * @param parent Родительский индекс
     * @return Количество торрентов
     */
    int rowCount(const QModelIndex &parent = QModelIndex()) const override;
    
    /**
     * @brief Данные для отображения
     * Возвращает данные в зависимости от запрошенной роли
     * @param index Индекс элемента
     * @param role Роль данных
     * @return Значение для отображения
     */
    QVariant data(const QModelIndex &index, int role = Qt::DisplayRole) const override;
    
    /**
     * @brief Заголовки для представления
     * @param section Номер секции
     * @param orientation Ориентация
     * @param role Роль
     * @return Заголовок
     */
    QVariant headerData(int section, Qt::Orientation orientation, 
                       int role = Qt::DisplayRole) const override;

    /**
     * @brief Роли модели для QML
     * Возвращает хеш ролей для использования в QML
     * @return Хеш ролей
     */
    QHash<int, QByteArray> roleNames() const override;

    // ── Методы управления моделью ─────────────────────────────────────
    
    /**
     * @brief Добавить торрент в модель
     * Вызывает beginInsertRows/endInsertRows для обновления представления
     * @param torrent Информация о торренте
     */
    void addTorrent(const TorrentInfo &torrent);
    
    /**
     * @brief Добавить торрент из JSON
     * Парсит JSON и добавляет торрент в модель
     * @param json JSON объект от API
     */
    void addTorrentFromJson(const QJsonObject &json);
    
    /**
     * @brief Удалить торрент из модели
     * Вызывает beginRemoveRows/endRemoveRows для обновления представления
     * @param id ID торрента
     * @return true если торрент был найден и удалён
     */
    bool removeTorrent(const QString &id);
    
    /**
     * @brief Обновить данные торрента
     * Находит торрент по ID и обновляет его данные
     * @param id ID торрента
     * @param torrent Новые данные
     * @return true если торрент был найден и обновлён
     */
    bool updateTorrent(const QString &id, const TorrentInfo &torrent);
    
    /**
     * @brief Обновить торрент из JSON
     * Парсит JSON и обновляет данные торрента
     * @param json JSON объект от API
     * @return true если торрент был найден и обновлён
     */
    bool updateTorrentFromJson(const QJsonObject &json);
    
    /**
     * @brief Получить торрент по индексу
     * @param index Индекс в модели
     * @return Информация о торренте
     */
    TorrentInfo torrentAt(int index) const;
    
    /**
     * @brief Получить торрент по ID
     * @param id ID торрента
     * @return Информация о торренте (пустую если не найден)
     */
    TorrentInfo torrentById(const QString &id) const;
    
    /**
     * @brief Найти индекс торрента по ID
     * @param id ID торрента
     * @return Индекс в модели (-1 если не найден)
     */
    int indexOf(const QString &id) const;
    
    /**
     * @brief Проверить наличие торрента
     * @param id ID торрента
     * @return true если торрент есть в модели
     */
    bool contains(const QString &id) const;
    
    /**
     * @brief Очистить модель (удалить все торренты)
     * Вызывает beginResetModel/endResetModel
     */
    void clear();
    
    /**
     * @brief Загрузить список торрентов из JSON массива
     * Очищает модель и загружает новые данные
     * @param torrents JSON массив торрентов
     */
    void loadFromJson(const QJsonArray &torrents);

    /**
     * @brief Получить все торренты
     * @return Вектор всех торрентов
     */
    QVector<TorrentInfo> torrents() const { return m_torrents; }

signals:
    /**
     * @brief Сигнал добавления торрента
     * Испускается после успешного добавления
     * @param index Индекс добавленного торрента
     */
    void torrentAdded(int index);
    
    /**
     * @brief Сигнал удаления торрента
     * Испускается после удаления
     * @param index Индекс удалённого торрента
     */
    void torrentRemoved(int index);
    
    /**
     * @brief Сигнал обновления торрента
     * Испускается после обновления данных
     * @param index Индекс обновлённого торрента
     */
    void torrentUpdated(int index);

private:
    QVector<TorrentInfo> m_torrents;  ///< Список торрентов
};

#endif // TORRENTMODEL_H
