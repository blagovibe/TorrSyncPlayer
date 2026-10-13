/**
 * @file roommanager.h
 * @brief Менеджер управления комнатами для TorrPlayer
 * 
 * Вынесен из MainWindow для уменьшения размера класса.
 * Управляет созданием, присоединением и выходом из комнат.
 */

#ifndef ROOMMANAGER_H
#define ROOMMANAGER_H

#include <QObject>
#include <QJsonArray>
#include <QJsonObject>
#include <QVector>

// Предварительные объявления
class NetworkManager;

/**
 * @struct PeerEntry
 * @brief Один участник комнаты в том виде, в каком его сообщает сервер
 *
 * Ровно те поля, которые сервер действительно знает. Поля «синхронизирован с
 * комнатой» здесь нет и не должно быть: позиция в комнате одна общая, поэтому
 * сервер не может знать, насколько отстал конкретный пир. Своё отставание
 * клиент считает сам, сравнивая свою позицию с позицией комнаты.
 */
struct PeerEntry
{
    QString peerId;     ///< ID сессии пира; меняется при переподключении
    QString userId;     ///< ID клиента; стабилен, по нему идёт сверка
    QString username;   ///< Отображаемое имя, может быть пустым
    bool isHost = false;///< Хост комнаты, он управляет play/pause/seek
    bool buffering = false; ///< Пир сообщил, что у него кончились данные
};

/**
 * @class RoomManager
 * @brief Менеджер операций с комнатами синхронизации
 * 
 * Инкапсулирует логику:
 * - Создание комнаты
 * - Присоединение к комнате
 * - Выход из комнаты
 * - Синхронизация воспроизведения
 * - Обработка событий комнаты
 */
class RoomManager : public QObject
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор менеджера комнат
     * @param network Менеджер сети для API запросов
     * @param parent Родительский объект
     */
    explicit RoomManager(NetworkManager *network, QObject *parent = nullptr);
    
    /**
     * @brief Деструктор
     */
    ~RoomManager();

    /**
     * @brief Проверить, находится ли пользователь в комнате
     * @return true если в комнате
     */
    bool isInRoom() const;
    
    /**
     * @brief Получить ID текущей комнаты
     * @return ID комнаты
     */
    QString currentRoomId() const { return m_currentRoomId; }
    
    /**
     * @brief Проверить, является ли пользователь хостом
     * @return true если хост
     */
    bool isHost() const { return m_isHost; }
    
    /**
     * @brief Установить флаг хоста
     * @param host true если хост
     */
    void setHost(bool host) { m_isHost = host; }

    /// @brief Сколько сейчас участников в комнате, включая хоста
    int peerCount() const { return m_peerCount; }

    /// @brief Полный список участников, как сообщил сервер
    QVector<PeerEntry> roster() const { return m_roster; }

    /// @brief Позиция комнаты по последней сверке — с ней сравнивают свою
    double serverPosition() const { return m_serverPosition; }

    /// @brief Идёт ли воспроизведение в комнате по последней сверке
    bool serverIsPlaying() const { return m_serverIsPlaying; }

    /// @brief Наш собственный userID — чтобы отметить себя в списке
    QString selfUserId() const { return m_selfUserId; }

    /**
     * @brief Насколько мы отстали от комнаты, секунды
     * @param localPosition Наша текущая позиция в плеере
     *
     * Знак важен: отрицательное значение означает, что мы впереди комнаты, а не
     * отстали. Ноль — мы точно в комнате.
     */
    double lagSeconds(double localPosition) const;

    /**
     * @brief Собрать код приглашения из ID и пароля комнаты
     *
     * Формат — `roomId:password`. Это ровно те два поля, которые запрос входа
     * и так требует, собранные в одну строку, чтобы можно было скопировать и
     * отправить одним сообщением. Ничего нового код не добавляет: он не
     * шифрует, не подписывает и не защищает — в нём ровно те же данные, что
     * приглашённому и так придётся передать, чтобы войти.
     *
     * Токен доступа сюда намеренно не входит. У отправителя он уже есть, но
     * это учётка на весь процесс, а не секрет комнаты: положив её в
     * приглашение, мы подняли бы привилегии гостя с «войти в эту комнату» до
     * «управлять всем сервером». Смысла в ней нет, а вреда — наоборот.
     */
    static QString makeInviteCode(const QString &roomId, const QString &password);

    /**
     * @brief Разобрать код приглашения на ID и пароль
     *
     * Разделитель — первое двоеточие: ID комнаты состоит из шестнадцатеричных
     * символов и двоеточия не содержит, а пароль может. Код без двоеточия
     * считается ID комнаты с пустым паролем, иначе вставленная строка молча
     * ломала бы вход в комнату без пароля.
     *
     * @param code Строка, полученная от хоста
     * @param roomId Разбираемый ID комнаты
     * @param password Разбираемый пароль
     * @return true, если строка разобрана и ID выглядит как ID комнаты
     */
    static bool parseInviteCode(const QString &code,
                                QString *roomId,
                                QString *password);

    /**
     * @brief Проверить, что строка похожа на ID комнаты
     *
     * Повторяет серверное правило (32 шестнадцатеричных символа). Нужно, чтобы
     * разбор отвергал мусор на клиенте, а не отправлял его на сервер.
     */
    static bool looksLikeRoomId(const QString &roomId);

public slots:
    /**
     * @brief Создать новую комнату
     * @param name Имя комнаты
     * @param password Пароль (опционально)
     * @param torrentId infohash торрента, к которому привязывается комната
     *        (idea-0046). Обязателен: сервер отвергает комнату без него.
     */
    void createRoom(const QString &name, const QString &password, const QString &torrentId);

    /**
     * @brief Присоединиться к комнате
     * @param roomId ID комнаты
     * @param password Пароль (опционально)
     * @param torrentId infohash торрента комнаты; сервер сверяет его и отказывает
     *        при несовпадении.
     */
    void joinRoom(const QString &roomId, const QString &password, const QString &torrentId);
    
    /**
     * @brief Покинуть текущую комнату
     */
    void leaveRoom();
    
    /**
     * @brief Синхронизировать воспроизведение (play)
     */
    void syncPlay();
    
    /**
     * @brief Синхронизировать паузу
     */
    void syncPause();
    
    /**
     * @brief Синхронизировать перемотку
     * @param position Позиция в секундах
     */
    void syncSeek(double position);

    /**
     * @brief Сообщить комнате о начале/окончании буферизации
     * В отличие от play/pause/seek, вызывается любым участником: буферизация
     * — локальное состояние пира, поэтому о нём сообщает тот, у кого не
     * хватает данных, а остальные подстраиваются.
     * @param buffering true — началась буферизация, false — закончилась
     */
    void syncBuffering(bool buffering);
    
    /**
     * @brief Обработка события комнаты
     * @param event JSON объект события
     */
    void onRoomEvent(const QJsonObject &event);
    
    /**
     * @brief Обработка сигнала от пира
     * @param signal JSON объект сигнала
     */
    void onSignalReceived(const QJsonObject &signal);

    /**
     * @brief Применение актуального статуса синхронизации
     * Приходит после (пере)подключения к комнате. Значения проверяются на
     * корректность до того, как попасть дальше: ответ сервера — это данные,
     * а не приказ (bug-0012).
     * @param status JSON объект SyncStatus
     */
    void onSyncStatusReceived(const QJsonObject &status);

    /**
     * @brief Применение информации о комнате
     * Обновляет роль хоста (сравнением hostUserId со своим X-Client-ID) и
     * число участников (bug-0012).
     * @param info JSON объект RoomInfo
     */
    void onRoomInfoReceived(const QJsonObject &info);

    /**
     * @brief Реакция на подтверждение создания комнаты
     * @param roomId ID комнаты
     *
     * Фиксирует состояние и пере-эмитит сигнал дальше. Без этих слотов
     * m_currentRoomId никогда не заполнялся, isInRoom() всегда возвращала
     * false, а сигналы roomCreated/roomJoined до MainWindow не доходили.
     */
    void onRoomCreated(const QString &roomId);

    /**
     * @brief Реакция на подтверждение присоединения к комнате
     * @param roomId ID комнаты
     */
    void onRoomJoined(const QString &roomId);

    /**
     * @brief Реакция на выход из комнаты
     */
    void onRoomLeft();

signals:
    /**
     * @brief Комната создана
     * @param roomId ID созданной комнаты
     */
    void roomCreated(const QString &roomId);
    
    /**
     * @brief Присоединились к комнате
     * @param roomId ID комнаты
     */
    void roomJoined(const QString &roomId);
    
    /**
     * @brief Покинули комнату
     */
    void roomLeft();
    
    /**
     * @brief Получено событие комнаты
     * @param event JSON объект события
     */
    void roomEvent(const QJsonObject &event);
    
    /**
     * @brief Получен сигнал синхронизации
     * @param action Действие (play/pause/seek)
     * @param position Позиция (для seek)
     */
    void syncAction(const QString &action, double position = 0.0);

    /**
     * @brief Пир сообщил о буферизации
     * @param buffering true — пир буферизует и ждёт данных, false — догнал
     */
    void bufferingChanged(bool buffering);

    /**
     * @brief Состояние комнаты сверено с сервером
     * Испускается после (пере)подключения к комнате, когда пришёл актуальный
     * /sync/status. Плеер должен догнать эти значения: пока соединение было
     * рвано, он показывал устаревшую позицию и мог играть при паузе в комнате.
     * @param position Позиция комнаты, секунды
     * @param isPlaying Идёт ли воспроизведение в комнате
     * @param buffering Буферизует ли кто-то в комнате
     */
    void stateReconciled(double position, bool isPlaying, bool buffering);

    /**
     * @brief Состав комнаты обновлён
     * @param peerCount Сколько сейчас участников
     * @param hostUserId Идентификатор клиента хоста
     * @param isHost Являемся ли мы хостом
     */
    void rosterChanged(int peerCount, const QString &hostUserId, bool isHost);

    /**
     * @brief Список участников обновлён
     *
     * Приходит из двух независимых источников, оба — полные снимки:
     * GET /rooms/info (клиент только подключился и не видел ни одного
     * peer_joined) и событие peer_list (кто-то пришёл или ушёл). Так как оба
     * источника заменяют список целиком, применить их дважды безопасно —
     * именно поэтому peer_list может прийти поверх сверки после реконнекта
     * (bug-0012) и не будет применён дважды.
     */
    void rosterUpdated(const QVector<PeerEntry> &peers);

    /**
     * @brief ID торрента комнаты определён
     * Выдаётся при получении /rooms/info после присоединения к комнате
     * (idea-0046: комната привязана к торренту). Позволяет UI автоматически
     * добавить торрент, если его ещё нет локально.
     * @param torrentId infohash торрента, к которому привязана комната
     */
    void roomTorrentIdentified(const QString &torrentId);

    /**
     * @brief Пир присоединился к комнате
     * @param peerId ID пира
     */
    void peerJoined(const QString &peerId);
    
    /**
     * @brief Пир покинул комнату
     * @param peerId ID пира
     */
    void peerLeft(const QString &peerId);
    
    /**
     * @brief Ошибка операции с комнатой
     * @param message Описание ошибки
     */
    void error(const QString &message);

private:
    /**
     * @brief Разбирает JSON-массив roster в PeerEntry
     *
     * Записи без peerID пропускаются: без него пира нельзя адресовать, и он
     * слился бы с соседом в интерфейсе.
     */
    static QVector<PeerEntry> parseRoster(const QJsonArray &array);

    /**
     * @brief Заменяет список участников целиком и уведомляет подписчиков
     *
     * Порядок и содержимое приходят с сервера как есть: сервер сортирует список
     * по peerID, иначе порядок зависел бы от обхода map и каждое событие
     * выглядело бы изменившимся без реальных изменений.
     */
    void applyRoster(const QVector<PeerEntry> &peers, int seq);

    /**
     * @brief Помечает пира буферизирующим по его userID
     * @param userID Кто сообщил о буферизации
     * @param buffering Буферизирует ли он
     *
     * SyncEvent несёт Initiator = userID, а не peerID, поэтому сопоставление
     * идёт через roster. Это нужно для мгновенной реакции: событие peer_list с
     * тем же фактом придёт следующим, но список не должен мигать до него.
     */
    void markBufferingByUserID(const QString &userID, bool buffering);

    NetworkManager *m_network;      ///< Менеджер сети
    QString m_currentRoomId;        ///< ID текущей комнаты
    bool m_isHost = false;          ///< Флаг хоста комнаты
    /// Участников в комнате по последней сверке. Считает ВСЕХ, включая хоста:
    /// хост — полноценный участник (bug-0021), и это же число приходит в
    /// peers.size(), поэтому расхождение между ними — всегда дефект.
    int m_peerCount = 0;
    QVector<PeerEntry> m_roster;    ///< Полный список участников, как сообщил сервер
    QString m_selfUserId;            ///< Наш userID: единственный способ отличить себя
    /// Версия принятого снимка roster. 0 — сервер версию не присылает.
    int m_rosterSeq = 0;
    double m_serverPosition = 0.0;  ///< Позиция комнаты по последней сверке, секунды
    bool m_serverIsPlaying = false; ///< Идёт ли воспроизведение в комнате
    bool m_serverBuffering = false; ///< Буферизует ли кто-то в комнате
};

#endif // ROOMMANAGER_H
