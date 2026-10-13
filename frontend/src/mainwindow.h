/*
 * SPDX-License-Identifier: MIT
 * Copyright (c) 2025-2026 TorrSyncPlayer contributors
 * See LICENSE file for full license text
 */

/**
 * @file mainwindow.h
 * @brief Главное окно приложения TorrPlayer
 *
 * Содержит:
 * - Левую панель: список торрентов, список файлов, поле ввода magnet-ссылки
 * - Правую панель: MPV виджет, контролы воспроизведения, кнопки комнат
 * - Статусную строку
 *
 * Логика управления торрентами вынесена в TorrentManager.
 * Логика управления комнатами вынесена в RoomManager.
 * Поддерживает graceful degradation при недоступности сервера.
 */

#ifndef MAINWINDOW_H
#define MAINWINDOW_H

#include <QMainWindow>
#include <QLineEdit>
#include <QPushButton>
#include <QListView>
#include <QSlider>
#include "roommanager.h"
#include <QLabel>
#include <QAction>
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QGroupBox>
#include <QProgressBar>
#include <QSplitter>
#include <QVector>
#include <QStatusBar>
#include <QJsonArray>
#include <QCloseEvent>
#include <QUrl>
#include <QTimer>
#include <QUuid>

// Предварительные объявления
class MpvWidget;
class NetworkManager;
class TorrentModel;
class TorrentManager;
class RoomManager;


/**
 * @class MainWindow
 * @brief Главное окно видеоплеера TorrPlayer
 *
 * Обеспечивает интерфейс для:
 * - Управления торрентами (добавление, удаление, выбор файлов)
 * - Воспроизведения видео через libmpv
 * - Создания и присоединения к комнатам синхронизации
 * - Синхронизации воспроизведения между пирами
 * - Graceful degradation при недоступности сервера
 */
class MainWindow : public QMainWindow
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор главного окна
     * @param parent Родительский виджет
     */
    explicit MainWindow(QWidget *parent = nullptr);

    /**
     * @brief Деструктор
     */
    ~MainWindow();

    /**
     * @brief Инициализация UI компонентов
     * Создаёт и размещает все виджеты главного окна
     */
    void setupUI();

    /**
     * @brief Подключение сигналов и слотов
     * Соединяет сигналы виджетов со слотами обработки
     */
    void setupConnections();

    /**
     * @brief Установить URL сервера для NetworkManager
     * @param url URL сервера
     */
    void setServerUrl(const QUrl &url);
    void setControlServerUrl(const QUrl &url);
    void setLocalBackendUrl(const QUrl &url);

    /**
     * @brief Установить токен доступа для API
     * Токен выдаётся сервером при запуске и передаётся как аргумент командной строки
     * @param token Access token для заголовка X-Access-Token
     */
    void setAccessToken(const QString &token);
    void setControlAccessToken(const QString &token);
    void setLocalAccessToken(const QString &token);

    /**
     * @brief Установить идентификатор клиента для P2P комнат
     * Используется заголовок X-Client-ID для идентификации участника в комнате
     * @param clientId Уникальный идентификатор этого экземпляра плеера
     */
    void setClientId(const QString &clientId);

    /**
     * @brief Инициализировать загрузку торрентов после установки URL сервера
     * Вызывает запрос списка торрентов к серверу
     */
    void initialize();

private slots:
    // ── Слоты управления торрентами ────────────────────────────────────

    /**
     * @brief Обработка нажатия кнопки "Добавить торрент"
     * Отправляет magnet-ссылку на сервер
     */
    void onAddTorrent();



    /**
     * @brief Обработка выбора торрент-файла через диалог
     * Читает файл и отправляет на сервер
     */
    void onAddTorrentFile();

    /**
     * @brief Обработка выбора торрента в списке
     * @param index Индекс выбранного торрента
     */
    void onTorrentSelected(const QModelIndex &index);

    /**
     * @brief Обработка выбора файла в списке
     * @param index Индекс выбранного файла
     */
    void onFileSelected(const QModelIndex &index);

    /**
     * @brief Обработка нажатия кнопки "Удалить торрент"
     */
    void onRemoveTorrent();

    // ── Слоты управления комнатами ────────────────────────────────────

    /**
     * @brief Обработка нажатия кнопки "Создать комнату"
     * Открывает диалог создания комнаты
     */
    void onCreateRoom();

    /**
     * @brief Обработка нажатия кнопки "Присоединиться"
     * Открывает диалог присоединения к комнате
     */
    void onJoinRoom();

/**
     * @brief Обработка выхода из комнаты
     * Отправляет запрос на выход из комнаты
     */
    void onLeaveRoom();

    /**
     * @brief Скопировать код приглашения текущей комнаты в буфер обмена
     *
     * Код — это «ID:пароль», то есть ровно то, что и так нужно передать
     * приглашённому. Он не шифрует и не защищает: пароль остаётся паролем.
     * Токен доступа в код не входит намеренно — это учётка на весь процесс,
     * и в приглашении ей не место.
     */
    void onCopyInviteCode();

    // ── Слоты управления воспроизведением ─────────────────────────────

    /**
     * @brief Обработка нажатия кнопки Play/Pause
     * Переключает состояние воспроизведения
     */
    void onPlayPause();

    /**
     * @brief Обработка перемотки слайдером
     * @param value Новое значение слайдера
     */
    void onSeek(int value);

    /**
     * @brief Обработка нажатия слайдера перемотки
     */
    void onSeekSliderPressed();

    /**
     * @brief Обработка отпускания слайдера перемотки
     */
    void onSeekSliderReleased();

    /**
     * @brief Обработка изменения позиции воспроизведения
     * @param position Новая позиция в секундах
     */
    void onPositionChanged(double position);

    /**
     * @brief Обработка изменения длительности
     * @param duration Новая длительность в секундах
     */
    void onDurationChanged(double duration);

    /**
     * @brief Получена информация о буферизации
     * Обновляет прогресс-бар буфера реальным процентом из backend.
     * @param info JSON объект BufferInfo
     */
    void onBufferInfoReceived(const QJsonObject &info);

    /**
     * @brief Обработка завершения воспроизведения
     * Вызывается когда видео доходит до конца
     */
    void onPlaybackFinished();

    /**
     * @brief Обработка ошибки воспроизведения
     * @param message Описание ошибки
     */
    void onPlaybackError(const QString &message);

    // ── Слоты ошибок ──────────────────────────────────────────────────

    /**
     * @brief Обработка ошибки сети
     * @param message Описание ошибки
     */
    void onNetworkError(const QString &message);

    /**
     * @brief Обработка получения списка файлов торрента
     * @param torrentId ID торрента
     * @param files JSON массив файлов
     */
    void onFilesReceived(const QString &torrentId, const QJsonArray &files);

    // ── Слоты graceful degradation ─────────────────────────────────────

    /**
     * @brief Обработка недоступности сервера
     * Показывает уведомление и переключает в offline режим
     */
    void onServerUnavailable();

    /**
     * @brief Обработка восстановления связи с сервером
     * Обновляет данные и переключает в online режим
     */
    void onServerAvailable();

    // ── Слоты TorrentManager ──────────────────────────────────────────

    /**
     * @brief Обработка выбора файла для воспроизведения
     * @param torrentId ID торрента
     * @param fileIndex Индекс файла
     * @param url URL потока
     */
    void onFileSelectedByManager(const QString &torrentId, int fileIndex, const QString &url);

    /**
     * @brief Кэширование списка торрентов для graceful degradation
     * @param torrents Список торрентов (QJsonArray)
     */
    void onTorrentListReceived(const QJsonArray &torrents);

    /**
     * @brief Получен stream-ticket — запустить воспроизведение с тикетом
     * @param torrentId ID торрента
     * @param ticket Подписанный тикет для /stream
     */
    void onStreamTicketReceived(const QString &torrentId, const QString &ticket);

    // ── Слоты RoomManager ─────────────────────────────────────────────

    /**
     * @brief Обработка создания комнаты
     * @param roomId ID созданной комнаты
     */
    void onRoomCreated(const QString &roomId);

    /**
     * @brief Обработка присоединения к комнате
     * @param roomId ID комнаты
     */
    void onRoomJoined(const QString &roomId);

    /**
     * @brief Обработка выхода из комнаты
     */
    void onRoomLeft();

    /**
     * @brief Обработка синхронизирующего действия
     * @param action Действие (play/pause/seek)
     * @param position Позиция в секундах
     */
    void onSyncAction(const QString &action, double position);

    /**
     * @brief Наш mpv начал или закончил буферизацию
     * Сообщает комнате, чтобы остальные участники держали паузу, пока
     * данных не хватает (bug-0011).
     * @param buffering true — идёт буферизация
     */
    void onLocalBufferingState(bool buffering);

    /**
     * @brief Другой участник начал или закончил буферизацию
     * Ставит/снимает паузу локально. Команда в комнату при этом не
     * отправляется: буферизация — не команда хоста.
     * @param buffering true — пир буферизует
     */
    void onPeerBufferingState(bool buffering);

    /**
     * @brief Обработка присоединения пира
     * @param peerId ID пира
     */
    void onPeerJoined(const QString &peerId);

    /**
     * @brief Обработка выхода пира
     * @param peerId ID пира
     */
    void onPeerLeft(const QString &peerId);

    /**
     * @brief Применение состояния комнаты после реконнекта
     * Возвращает локальный плеер к тому, в чём комната жила, пока соединение
     * было рвано: play/pause, позиция, буферизация (bug-0012).
     * @param position Позиция комнаты, секунды
     * @param isPlaying Идёт ли воспроизведение в комнате
     * @param buffering Буферизует ли кто-то в комнате
     */
    void onStateReconciled(double position, bool isPlaying, bool buffering);

    /**
     * @brief Обновление состава комнаты после реконнекта
     * Пока соединение было рвано, кто-то мог войти или выйти, а хостом мог
     * стать другой участник (bug-0012).
     * @param peerCount Сколько сейчас участников
     * @param hostUserId Идентификатор клиента хоста
     * @param isHost Являемся ли мы хостом
     */
    void onRosterChanged(int peerCount, const QString &hostUserId, bool isHost);

    /**
     * @brief Список участников обновлён
     * @param peers Состав комнаты, как сообщил сервер
     */
    void onRosterUpdated(const QVector<PeerEntry> &peers);

    /**
     * @brief ID торрента комнаты определён
     * Вызывается при получении /rooms/info. Если у нас нет этого торрента —
     * добавляем его и выбираем файл (idea-0046: per-peer selectedFiles, bug-0048).
     * @param torrentId infohash торрента, к которому привязана комната
     */
    void onRoomTorrentIdentified(const QString &torrentId);

    /**
     * @brief Пересобрать строку состава комнаты
     *
     * Показывает только то, что сервер действительно знает: кто в комнате, кто
     * хост, кто буферизирует. Отдельного «синхронизирован с комнатой» нет и
     * быть не может — сервер не знает позицию отдельного пира. Своё отставание
     * показывает отдельный индикатор.
     */
    void updateRosterLabel();

    /**
     * @brief Пересчитать индикатор отставания от комнаты
     *
     * Показывает, насколько наша позиция расходится с позицией комнаты. Ниже
     * порога ничего не показывается: небольшой разброс — это нормальное
     * дрожание, и мигающая надпись об отставании в полсекунды только мешает.
     */
    void updateLagIndicator();

protected:
    /**
     * @brief Обработка события закрытия окна
     * Выполняет graceful shutdown: выход из комнаты, остановка плеера
     * @param event Событие закрытия
     */
    void closeEvent(QCloseEvent *event) override;

private:
    /**
     * @brief Создание левой панели
     * Содержит список торрентов, файлов и поле ввода magnet-ссылки
     * @return Виджет левой панели
     */
    QWidget* createLeftPanel();

    /**
     * @brief Создание правой панели
     * Содержит видеоплеер и контролы воспроизведения
     * @return Виджет правой панели
     */
    QWidget* createRightPanel();

    /**
     * @brief Создание панели управления воспроизведением
     * Содержит кнопки play/pause, слайдер перемотки, метку времени
     * @return Виджет панели управления
     */
    QWidget* createControlsPanel();

    /**
     * @brief Создание панели комнат
     * Содержит кнопки создания/присоединения/выхода из комнаты
     * @return Виджет панели комнат
     */
    QWidget* createRoomPanel();

    /**
     * @brief Обновление статусной строки
     * @param message Сообщение статуса
     */
    void updateStatus(const QString &message);

    /**
     * @brief Обновление метки времени
     * @param position Текущая позиция в секундах
     * @param duration Длительность в секундах
     */
    void updateTimeLabel(double position, double duration);

    /**
     * @brief Форматирование времени
     * @param seconds Время в секундах
     * @return Строка вида "01:23:45"
     */
    QString formatTime(double seconds) const;

    /**
     * @brief Обновление UI при входе в комнату
     * @param roomId ID комнаты
     * @param isHost true если пользователь хост
     */
    void updateRoomUI(const QString &roomId, bool isHost);

    /**
     * @brief Обновление UI при выходе из комнаты
     */
    void clearRoomUI();

    // ── Компоненты UI ─────────────────────────────────────────────────

    // Левая панель
    QLineEdit *m_magnetInput = nullptr;           ///< Поле ввода magnet-ссылки
    QPushButton *m_addButton = nullptr;           ///< Кнопка добавления торрента (magnet)
    QPushButton *m_addFileButton = nullptr;       ///< Кнопка добавления торрента (файл)
    QListView *m_torrentList = nullptr;           ///< Список торрентов
    QListView *m_fileList = nullptr;              ///< Список файлов торрента
    QPushButton *m_removeTorrentButton = nullptr; ///< Кнопка удаления торрента

    // Правая панель
    MpvWidget *m_mpvWidget = nullptr;             ///< Виджет видеоплеера
    QPushButton *m_playPauseButton = nullptr;     ///< Кнопка Play/Pause
    QSlider *m_seekSlider = nullptr;              ///< Слайдер перемотки
    QLabel *m_timeLabel = nullptr;                ///< Метка времени
    QProgressBar *m_bufferProgress = nullptr;     ///< Прогресс буферизации

    // Панель комнат
    QPushButton *m_createRoomButton = nullptr;    ///< Кнопка создания комнаты
    QPushButton *m_joinRoomButton = nullptr;      ///< Кнопка присоединения
    QPushButton *m_leaveRoomButton = nullptr;     ///< Кнопка выхода из комнаты
    QLabel *m_roomStatusLabel = nullptr;          ///< Статус комнаты
    QAction *m_copyInviteAction = nullptr;      ///< «Скопировать код приглашения»

    // Пароль, заданный при создании комнаты. Нужен, чтобы после получения
    // серверного ID собрать код приглашения. Пустой — комната без пароля.
    QString m_createdRoomPassword;

    // Статусная строка
    QLabel *m_statusLabel = nullptr;              ///< Статусное сообщение
    QLabel *m_rosterLabel = nullptr;             ///< Строка состава комнаты
    QLabel *m_lagLabel = nullptr;                ///< Индикатор отставания от комнаты

    // ── Менеджеры ─────────────────────────────────────────────────────

    NetworkManager *m_network = nullptr;          ///< Менеджер сети
    TorrentModel *m_torrentModel = nullptr;       ///< Модель торрентов
    TorrentManager *m_torrentManager = nullptr;   ///< Менеджер торрентов
    RoomManager *m_roomManager = nullptr;         ///< Менеджер комнат

    // ── Данные ────────────────────────────────────────────────────────

    bool m_isPlaying = false;           ///< Флаг воспроизведения
    bool m_isSeeking = false;           ///< Флаг перемотки (для предотвращения зацикливания)
    double m_duration = 0.0;            ///< Длительность текущего медиа
    double m_currentPosition = 0.0;     ///< Текущая позиция воспроизведения (для возобновления после смены тикета)
    qint64 m_currentFileSize = 0;       ///< Размер выбранного файла в байтах (для расчёта позиции буфера)
    QVector<qint64> m_fileSizes;         ///< Размеры файлов текущего торрента (по индексу)
    QTimer *m_bufferPollTimer = nullptr; ///< Таймер опроса состояния буфера во время воспроизведения
    QTimer *m_bufferPositionTimer = nullptr; ///< Таймер отправки позиции воспроизведения на бэкенд

    // ── Обновление stream-тикета ────────────────────────────────────────
    // Тикет живёт 5 минут (backend constants.StreamTicketTTL) и раньше
    // запрашивался ровно один раз, поэтому любой фильм длиннее 5 минут
    // молча умирал без возможности продолжить. Обновляем тикет заранее и
    // один раз повторяем запрос при ошибке воспроизведения.
    QTimer *m_streamTicketTimer = nullptr; ///< Таймер протухания тикета
    bool m_streamTicketRefresh = false;    ///< true — идёт обновление тикета для уже идущего видео
    bool m_streamTicketRetryUsed = false;  ///< Повтор при ошибке уже использован

    // ── Graceful degradation ───────────────────────────────────────────
    bool m_serverConnected = true;      ///< Флаг подключения к серверу
    QJsonArray m_cachedTorrents;        ///< Кэшированный список торрентов

    // ── Отложенный стрим (ожидание stream-ticket) ────────────────────────
    QString m_pendingStreamTorrentId;   ///< torrentId, ожидающий тикет для play
    QString m_currentTorrentId;          ///< torrentId активно воспроизводимого файла
    int m_pendingStreamFileIndex = -1;  ///< fileIndex, ожидающий тикет для play

    // ── Авто-добавление торрента комнаты (bug-0048) ────────────────────
    QString m_pendingRoomTorrentId;      ///< torrentId комнаты, который нужно добавить/выбрать
};

#endif // MAINWINDOW_H
