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
#include <QLabel>
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

// Предварительные объявления
class MpvWidget;
class NetworkManager;
class TorrentModel;
class TorrentManager;
class RoomManager;
class AuthDialog;

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
     * @brief Обработка входа в приложение
     * Открывает диалог авторизации
     */
    void onLogin();

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
     * @brief Обработка успешной авторизации
     * @param token JWT токен
     */
    void onLoginSuccess(const QString &token);

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
     * @brief Обработка присоединения пира
     * @param peerId ID пира
     */
    void onPeerJoined(const QString &peerId);

    /**
     * @brief Обработка выхода пира
     * @param peerId ID пира
     */
    void onPeerLeft(const QString &peerId);

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

    // Статусная строка
    QLabel *m_statusLabel = nullptr;              ///< Статусное сообщение

    // ── Менеджеры ─────────────────────────────────────────────────────

    NetworkManager *m_network = nullptr;          ///< Менеджер сети
    TorrentModel *m_torrentModel = nullptr;       ///< Модель торрентов
    TorrentManager *m_torrentManager = nullptr;   ///< Менеджер торрентов
    RoomManager *m_roomManager = nullptr;         ///< Менеджер комнат

    // ── Данные ────────────────────────────────────────────────────────

    bool m_isPlaying = false;           ///< Флаг воспроизведения
    bool m_isSeeking = false;           ///< Флаг перемотки (для предотвращения зацикливания)
    double m_duration = 0.0;            ///< Длительность текущего медиа
    qint64 m_currentFileSize = 0;       ///< Размер выбранного файла в байтах (для расчёта позиции буфера)
    QVector<qint64> m_fileSizes;         ///< Размеры файлов текущего торрента (по индексу)
    QTimer *m_bufferPollTimer = nullptr; ///< Таймер опроса состояния буфера во время воспроизведения

    // ── Graceful degradation ───────────────────────────────────────────
    bool m_serverConnected = true;      ///< Флаг подключения к серверу
    QJsonArray m_cachedTorrents;        ///< Кэшированный список торрентов

    // ── Отложенный стрим (ожидание stream-ticket) ────────────────────────
    QString m_pendingStreamTorrentId;   ///< torrentId, ожидающий тикет для play
    QString m_currentTorrentId;          ///< torrentId активно воспроизводимого файла
    int m_pendingStreamFileIndex = -1;  ///< fileIndex, ожидающий тикет для play
};

#endif // MAINWINDOW_H
