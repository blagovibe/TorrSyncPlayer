/**
 * @file mainwindow.cpp
 * @brief Реализация главного окна приложения TorrPlayer
 *
 * Рефакторинг: логика управления торрентами вынесена в TorrentManager,
 * логика управления комнатами вынесена в RoomManager.
 * Длинные лямбды вынесены в отдельные слоты.
 */

#include "mainwindow.h"
#include "mpvwidget.h"
#include "networkmanager.h"
#include "torrentmodel.h"
#include "torrentmanager.h"
#include "roomdialog.h"
#include "roommanager.h"
#include "authdialog.h"
#include "utils.h"

#include <QDebug>
#include <QMessageBox>
#include <QInputDialog>
#include <QJsonArray>
#include <QJsonObject>
#include <QStandardItemModel>
#include <QHeaderView>
#include <QToolBar>
#include <QMenuBar>
#include <QMenu>
#include <QAction>
#include <QCloseEvent>
#include <QApplication>
#include <QThread>
#include <QTimer>
#include <QFileDialog>
#include <QFile>

MainWindow::MainWindow(QWidget *parent)
    : QMainWindow(parent)
    , m_network(new NetworkManager(this))
    , m_torrentModel(new TorrentModel(this))
    , m_torrentManager(new TorrentManager(m_network, m_torrentModel, this))
    , m_roomManager(new RoomManager(m_network, this))
{
    // Устанавливаем заголовок и размер окна
    setWindowTitle(tr("TorrPlayer - Видеоплеер с торрентами"));
    resize(1200, 700);

    // Инициализация UI
    setupUI();
    setupConnections();

    updateStatus(tr("Готово к работе"));
    qDebug() << "MainWindow: инициализировано";
}

MainWindow::~MainWindow()
{
    qDebug() << "MainWindow: уничтожено";
}

void MainWindow::setServerUrl(const QUrl &url)
{
    if (m_network) {
        if (!url.isValid() || url.isEmpty()) {
            qWarning() << "MainWindow: невалидный URL сервера:" << url.toString();
            return;
        }
        m_network->setServerUrl(url);
        qDebug() << "MainWindow: установлен URL сервера:" << url.toString();
    }
}

void MainWindow::initialize()
{
    // Загружаем список торрентов после установки URL сервера
    m_torrentManager->listTorrents();
    updateStatus(tr("Готово к работе"));
    qDebug() << "MainWindow: инициализация завершена, загрузка торрентов...";
}

// ── Инициализация UI ────────────────────────────────────────────────────

void MainWindow::setupUI()
{
    // Центральный виджет
    QWidget *centralWidget = new QWidget(this);
    QHBoxLayout *mainLayout = new QHBoxLayout(centralWidget);
    mainLayout->setContentsMargins(5, 5, 5, 5);
    mainLayout->setSpacing(5);

    // Сплиттер для разделения панелей
    QSplitter *splitter = new QSplitter(Qt::Horizontal, this);
    splitter->addWidget(createLeftPanel());
    splitter->addWidget(createRightPanel());
    splitter->setStretchFactor(0, 1);  // Левая панель
    splitter->setStretchFactor(1, 3);  // Правая панель

    mainLayout->addWidget(splitter);
    setCentralWidget(centralWidget);

    // Статусная строка
    m_statusLabel = new QLabel(tr("Готово"), this);
    statusBar()->addPermanentWidget(m_statusLabel, 1);

    // Меню
    QMenu *fileMenu = menuBar()->addMenu(tr("&Файл"));
    QAction *addTorrentAction = fileMenu->addAction(tr("Добавить торрент..."));
    addTorrentAction->setShortcut(QKeySequence::New);
    connect(addTorrentAction, &QAction::triggered, this, &MainWindow::onAddTorrent);
    fileMenu->addSeparator();
    QAction *loginAction = fileMenu->addAction(tr("Войти"));
    connect(loginAction, &QAction::triggered, this, &MainWindow::onLogin);
    fileMenu->addSeparator();
    QAction *quitAction = fileMenu->addAction(tr("Выход"));
    quitAction->setShortcut(QKeySequence::Quit);
    connect(quitAction, &QAction::triggered, this, &QWidget::close);

    QMenu *roomMenu = menuBar()->addMenu(tr("&Комната"));
    QAction *createRoomAction = roomMenu->addAction(tr("Создать комнату..."));
    connect(createRoomAction, &QAction::triggered, this, &MainWindow::onCreateRoom);
    QAction *joinRoomAction = roomMenu->addAction(tr("Присоединиться..."));
    connect(joinRoomAction, &QAction::triggered, this, &MainWindow::onJoinRoom);
    QAction *leaveRoomAction = roomMenu->addAction(tr("Покинуть комнату"));
    connect(leaveRoomAction, &QAction::triggered, this, &MainWindow::onLeaveRoom);
}

QWidget* MainWindow::createLeftPanel()
{
    QGroupBox *groupBox = new QGroupBox(tr("Торренты"), this);
    QVBoxLayout *layout = new QVBoxLayout(groupBox);

    // Поле ввода magnet-ссылки
    QHBoxLayout *inputLayout = new QHBoxLayout();
    m_magnetInput = new QLineEdit(this);
    m_magnetInput->setPlaceholderText(tr("Вставьте magnet-ссылку..."));
    m_magnetInput->setClearButtonEnabled(true);
    inputLayout->addWidget(m_magnetInput);

    m_addButton = new QPushButton(QIcon(":/icons/add.png"), tr("Добавить"), this);
    m_addButton->setToolTip(tr("Добавить торрент"));
    m_addButton->setFixedWidth(40);
    inputLayout->addWidget(m_addButton);

    m_addFileButton = new QPushButton(QIcon(":/icons/folder.png"), QString(), this);
    m_addFileButton->setToolTip(tr("Добавить .torrent файл"));
    m_addFileButton->setFixedWidth(40);
    inputLayout->addWidget(m_addFileButton);
    layout->addLayout(inputLayout);

    // Список торрентов
    m_torrentList = new QListView(this);
    m_torrentList->setModel(m_torrentModel);
    m_torrentList->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_torrentList->setSelectionMode(QAbstractItemView::SingleSelection);
    m_torrentList->setAlternatingRowColors(true);
    layout->addWidget(m_torrentList);

    // Кнопка удаления
    m_removeTorrentButton = new QPushButton(QIcon(":/icons/remove.png"), tr("Удалить торрент"), this);
    m_removeTorrentButton->setEnabled(false);
    layout->addWidget(m_removeTorrentButton);

    // Список файлов
    QLabel *filesLabel = new QLabel(tr("Файлы торрента:"), this);
    layout->addWidget(filesLabel);

    m_fileList = new QListView(this);
    m_fileList->setEditTriggers(QAbstractItemView::NoEditTriggers);
    m_fileList->setSelectionMode(QAbstractItemView::SingleSelection);
    m_fileList->setAlternatingRowColors(true);
    m_fileList->setMaximumHeight(200);
    layout->addWidget(m_fileList);

    return groupBox;
}

QWidget* MainWindow::createRightPanel()
{
    QGroupBox *groupBox = new QGroupBox(tr("Воспроизведение"), this);
    QVBoxLayout *layout = new QVBoxLayout(groupBox);

    // MPV виджет
    m_mpvWidget = new MpvWidget(this);
    m_mpvWidget->setMinimumHeight(400);
    layout->addWidget(m_mpvWidget, 1);

    // Панель управления
    layout->addWidget(createControlsPanel());

    // Панель комнат
    layout->addWidget(createRoomPanel());

    return groupBox;
}

QWidget* MainWindow::createControlsPanel()
{
    QWidget *panel = new QWidget(this);
    QVBoxLayout *layout = new QVBoxLayout(panel);
    layout->setContentsMargins(0, 0, 0, 0);

    // Слайдер перемотки
    const int seekSliderMax = 1000;
    m_seekSlider = new QSlider(Qt::Horizontal, this);
    m_seekSlider->setRange(0, seekSliderMax);
    m_seekSlider->setValue(0);
    m_seekSlider->setEnabled(false);
    layout->addWidget(m_seekSlider);

    // Метка времени
    m_timeLabel = new QLabel("00:00:00 / 00:00:00", this);
    m_timeLabel->setAlignment(Qt::AlignCenter);
    layout->addWidget(m_timeLabel);

    // Кнопки управления
    QHBoxLayout *buttonsLayout = new QHBoxLayout();
    m_playPauseButton = new QPushButton(QIcon(":/icons/play.png"), tr("Play"), this);
    m_playPauseButton->setEnabled(false);
    m_playPauseButton->setMinimumWidth(100);
    buttonsLayout->addWidget(m_playPauseButton);
    buttonsLayout->addStretch();

    // Прогресс буферизации
    m_bufferProgress = new QProgressBar(this);
    m_bufferProgress->setRange(0, 100);
    m_bufferProgress->setValue(0);
    m_bufferProgress->setTextVisible(false);
    m_bufferProgress->setMaximumHeight(5);
    layout->addWidget(m_bufferProgress);
    layout->addLayout(buttonsLayout);

    return panel;
}

QWidget* MainWindow::createRoomPanel()
{
    QGroupBox *roomGroup = new QGroupBox(tr("Синхронизация"), this);
    QHBoxLayout *roomLayout = new QHBoxLayout(roomGroup);

    m_createRoomButton = new QPushButton(QIcon(":/icons/room-create.png"), tr("Создать комнату"), this);
    roomLayout->addWidget(m_createRoomButton);

    m_joinRoomButton = new QPushButton(QIcon(":/icons/room-join.png"), tr("Присоединиться"), this);
    roomLayout->addWidget(m_joinRoomButton);

    m_leaveRoomButton = new QPushButton(QIcon(":/icons/room-leave.png"), tr("Покинуть"), this);
    m_leaveRoomButton->setEnabled(false);
    roomLayout->addWidget(m_leaveRoomButton);

    m_roomStatusLabel = new QLabel(tr("Не в комнате"), this);
    roomLayout->addWidget(m_roomStatusLabel, 1);

    return roomGroup;
}

// ── Подключение сигналов ────────────────────────────────────────────────

void MainWindow::setupConnections()
{
    // UI элементы
    connect(m_addButton, &QPushButton::clicked, this, &MainWindow::onAddTorrent);
    connect(m_addFileButton, &QPushButton::clicked, this, &MainWindow::onAddTorrentFile);
    connect(m_magnetInput, &QLineEdit::returnPressed, this, &MainWindow::onAddTorrent);
    connect(m_torrentList, &QListView::doubleClicked, this, &MainWindow::onTorrentSelected);
    connect(m_fileList, &QListView::doubleClicked, this, &MainWindow::onFileSelected);
    connect(m_removeTorrentButton, &QPushButton::clicked, this, &MainWindow::onRemoveTorrent);
    connect(m_playPauseButton, &QPushButton::clicked, this, &MainWindow::onPlayPause);
    connect(m_seekSlider, &QSlider::sliderMoved, this, &MainWindow::onSeek);
    connect(m_seekSlider, &QSlider::sliderPressed, this, &MainWindow::onSeekSliderPressed);
    connect(m_seekSlider, &QSlider::sliderReleased, this, &MainWindow::onSeekSliderReleased);
    connect(m_createRoomButton, &QPushButton::clicked, this, &MainWindow::onCreateRoom);
    connect(m_joinRoomButton, &QPushButton::clicked, this, &MainWindow::onJoinRoom);
    connect(m_leaveRoomButton, &QPushButton::clicked, this, &MainWindow::onLeaveRoom);

    // TorrentManager
    connect(m_torrentManager, &TorrentManager::filesReceived, this, &MainWindow::onFilesReceived);
    connect(m_torrentManager, &TorrentManager::fileSelected, this, &MainWindow::onFileSelectedByManager);
    connect(m_torrentManager, &TorrentManager::error, this, &MainWindow::onNetworkError);

    // RoomManager
    connect(m_roomManager, &RoomManager::roomCreated, this, &MainWindow::onRoomCreated);
    connect(m_roomManager, &RoomManager::roomJoined, this, &MainWindow::onRoomJoined);
    connect(m_roomManager, &RoomManager::roomLeft, this, &MainWindow::onRoomLeft);
    connect(m_roomManager, &RoomManager::syncAction, this, &MainWindow::onSyncAction);
    connect(m_roomManager, &RoomManager::peerJoined, this, &MainWindow::onPeerJoined);
    connect(m_roomManager, &RoomManager::peerLeft, this, &MainWindow::onPeerLeft);
    connect(m_roomManager, &RoomManager::error, this, &MainWindow::onNetworkError);

    // NetworkManager
    connect(m_network, &NetworkManager::torrentAdded, m_torrentModel, &TorrentModel::addTorrentFromJson);
    connect(m_network, &NetworkManager::torrentRemoved, m_torrentModel, &TorrentModel::removeTorrent);
    connect(m_network, &NetworkManager::torrentListReceived, m_torrentManager, &TorrentManager::onTorrentListReceived);
    connect(m_network, &NetworkManager::torrentListReceived, this, &MainWindow::onTorrentListReceived);
    connect(m_network, &NetworkManager::streamTicketReceived, this, &MainWindow::onStreamTicketReceived);
    connect(m_network, &NetworkManager::filesReceived, m_torrentManager, &TorrentManager::onFilesReceived);
    connect(m_network, &NetworkManager::roomEvent, m_roomManager, &RoomManager::onRoomEvent);
    connect(m_network, &NetworkManager::signalReceived, m_roomManager, &RoomManager::onSignalReceived);
    connect(m_network, &NetworkManager::authenticated, this, &MainWindow::onLoginSuccess);
    connect(m_network, &NetworkManager::error, this, &MainWindow::onNetworkError);

    // Graceful degradation - обработка состояния сервера
    connect(m_network, &NetworkManager::serverUnavailable, this, &MainWindow::onServerUnavailable);
    connect(m_network, &NetworkManager::serverAvailable, this, &MainWindow::onServerAvailable);
    connect(m_network, &NetworkManager::bufferInfoReceived, this, &MainWindow::onBufferInfoReceived);

    // Опрос состояния буферизации во время воспроизведения (реальный процент буфера).
    m_bufferPollTimer = new QTimer(this);
    m_bufferPollTimer->setInterval(2000);
    connect(m_bufferPollTimer, &QTimer::timeout, this, [this]() {
        if (m_isPlaying && !m_currentTorrentId.isEmpty()) {
            m_network->getBufferInfo(m_currentTorrentId);
        }
    });

    // MpvWidget
    connect(m_mpvWidget, &MpvWidget::positionChanged, this, &MainWindow::onPositionChanged);
    connect(m_mpvWidget, &MpvWidget::durationChanged, this, &MainWindow::onDurationChanged);
    connect(m_mpvWidget, &MpvWidget::playbackFinished, this, &MainWindow::onPlaybackFinished);
    connect(m_mpvWidget, &MpvWidget::error, this, &MainWindow::onPlaybackError);
}

// ── Слоты управления торрентами ─────────────────────────────────────────

void MainWindow::onAddTorrent()
{
    QString magnetUri = m_magnetInput->text().trimmed();
    if (magnetUri.isEmpty()) {
        QMessageBox::warning(this, tr("Ошибка"), tr("Введите magnet-ссылку"));
        return;
    }
    m_torrentManager->addTorrent(magnetUri);
    m_magnetInput->clear();
    updateStatus(tr("Добавление торрента..."));
}

void MainWindow::onAddTorrentFile()
{
    QString fileName = QFileDialog::getOpenFileName(
        this,
        tr("Выберите .torrent файл"),
        QString(),
        tr("Torrent files (*.torrent);;All files (*.*)")
    );

    if (fileName.isEmpty()) {
        return; // Пользователь отменил выбор файла
    }

    QFile file(fileName);
    if (!file.open(QIODevice::ReadOnly)) {
        QMessageBox::warning(this, tr("Ошибка"),
            tr("Не удалось открыть файл: %1").arg(file.errorString()));
        return;
    }

    QByteArray fileContent = file.readAll();
    file.close();

    if (fileContent.isEmpty()) {
        QMessageBox::warning(this, tr("Ошибка"), tr("Файл .torrent пуст"));
        return;
    }

    m_torrentManager->addTorrentFile(fileContent);
    updateStatus(tr("Добавление торрента из файла..."));
}

void MainWindow::onTorrentSelected(const QModelIndex &index)
{
    if (!index.isValid()) return;

    QString torrentId = index.data(TorrentModel::IdRole).toString();
    m_torrentManager->setCurrentTorrentId(torrentId);
    m_removeTorrentButton->setEnabled(true);
    m_torrentManager->getFiles(torrentId);

    updateStatus(tr("Выбран торрент: %1").arg(index.data(TorrentModel::NameRole).toString()));
}

void MainWindow::onFileSelected(const QModelIndex &index)
{
    if (!index.isValid() || m_torrentManager->currentTorrentId().isEmpty()) return;

    int fileIndex = index.row();
    m_torrentManager->selectFile(m_torrentManager->currentTorrentId(), fileIndex);
    updateStatus(tr("Воспроизведение: %1").arg(index.data().toString()));
}

void MainWindow::onRemoveTorrent()
{
    QModelIndex index = m_torrentList->currentIndex();
    if (index.isValid()) {
        m_torrentManager->removeTorrent(index.data(TorrentModel::IdRole).toString());
    }
}

void MainWindow::onFilesReceived(const QString &torrentId, const QJsonArray &files)
{
    Q_UNUSED(torrentId)

    // Создаём модель для списка файлов с parent для автоматического удаления
    QStandardItemModel *model = new QStandardItemModel(this);

    m_fileSizes.clear();
    for (int i = 0; i < files.size(); ++i) {
        QJsonObject file = files[i].toObject();
        QString name = file["name"].toString();
        qint64 size = file["size"].toVariant().toLongLong();
        m_fileSizes.append(size);

        QString displayText = QString("%1 (%2)").arg(name).arg(Utils::formatBytes(size));
        QStandardItem *item = new QStandardItem(displayText);
        item->setData(i, Qt::UserRole);
        model->appendRow(item);
    }

    // Безопасное удаление старой модели с проверкой на nullptr
    QAbstractItemModel *oldModel = m_fileList->model();
    m_fileList->setModel(model);
    if (oldModel != nullptr) {
        oldModel->deleteLater();
    }

    updateStatus(tr("Файлов в торренте: %1").arg(files.size()));
}

// ── Слоты TorrentManager ────────────────────────────────────────────────

void MainWindow::onFileSelectedByManager(const QString &torrentId, int fileIndex, const QString &url)
{
    Q_UNUSED(url)
    if (torrentId.isEmpty()) {
        onPlaybackError(tr("Ошибка: ID торрента пуст"));
        return;
    }
    // /stream — публичный эндпоинт, аутентифицируемый подписанным тикетом,
    // т.к. libmpv не может прикрепить JWT-заголовок к своему HTTP-фетчу.
    // Запрашиваем тикет и запускаем play по его получении (onStreamTicketReceived).
    m_pendingStreamTorrentId = torrentId;
    m_pendingStreamFileIndex = fileIndex;
    if (fileIndex >= 0 && fileIndex < m_fileSizes.size()) {
        m_currentFileSize = m_fileSizes[fileIndex];
    } else {
        m_currentFileSize = 0;
    }
    m_network->requestStreamTicket(torrentId);
    updateStatus(tr("Запрос тикета потока..."));
}

void MainWindow::onStreamTicketReceived(const QString &torrentId, const QString &ticket)
{
    if (torrentId != m_pendingStreamTorrentId) {
        qWarning() << "MainWindow: получен тикет для неожиданного torrentId, игнорируем";
        return;
    }
    const QString url = m_network->streamUrl(torrentId) +
                        QString("?ticket=%1").arg(ticket);
    if (url.isEmpty()) {
        onPlaybackError(tr("Ошибка: URL потока пуст"));
        m_pendingStreamTorrentId.clear();
        m_pendingStreamFileIndex = -1;
        return;
    }
    m_mpvWidget->play(url);
#ifndef HAS_MPV
    // Сборка без libmpv: видео не воспроизведётся. Честно сообщаем пользователю,
    // вместо того чтобы молча делать вид, что播放 начался.
    QMessageBox::warning(this, tr("Воспроизведение недоступно"),
        tr("Эта сборка собрана без поддержки libmpv. Воспроизведение видео отключено. "
           "Установите libmpv и пересоберите приложение для просмотра."));
    updateStatus(tr("libmpv не подключён — воспроизведение отключено"));
    m_pendingStreamTorrentId.clear();
    m_pendingStreamFileIndex = -1;
    return;
#endif // HAS_MPV
    m_isPlaying = true;
    m_currentTorrentId = torrentId;
    m_playPauseButton->setIcon(QIcon(":/icons/pause.png"));
    m_playPauseButton->setText(tr("Pause"));
    m_playPauseButton->setEnabled(true);
    updateStatus(tr("Загрузка потока..."));
    m_pendingStreamTorrentId.clear();
    m_pendingStreamFileIndex = -1;
    m_bufferPollTimer->start();
}

// ── Слоты управления комнатами ──────────────────────────────────────────

void MainWindow::onCreateRoom()
{
    RoomDialog dialog(RoomDialog::CreateMode, this);
    if (dialog.exec() != QDialog::Accepted) return;

    m_roomManager->createRoom(dialog.roomName(), dialog.password());
    updateStatus(tr("Создание комнаты..."));
}

void MainWindow::onJoinRoom()
{
    RoomDialog dialog(RoomDialog::JoinMode, this);
    if (dialog.exec() != QDialog::Accepted) return;

    m_roomManager->joinRoom(dialog.roomId(), dialog.password());
    updateStatus(tr("Присоединение к комнате..."));
}

void MainWindow::onLeaveRoom()
{
    m_roomManager->leaveRoom();
}

// ── Слоты RoomManager ───────────────────────────────────────────────────

void MainWindow::onRoomCreated(const QString &roomId)
{
    updateRoomUI(roomId, true);
    updateStatus(tr("Комната создана: %1").arg(roomId));
}

void MainWindow::onRoomJoined(const QString &roomId)
{
    updateRoomUI(roomId, false);
    updateStatus(tr("Присоединились к комнате: %1").arg(roomId));
}

void MainWindow::onRoomLeft()
{
    clearRoomUI();
    updateStatus(tr("Покинули комнату"));
}

void MainWindow::onLogin()
{
    AuthDialog dialog(this);
    if (dialog.exec() == QDialog::Accepted) {
        QString username = dialog.username();
        QString password = dialog.password();
        AuthDialog::Mode mode = dialog.mode();
        
        if (mode == AuthDialog::LoginMode) {
            m_network->login(username, password);
        } else {
            m_network->registerUser(username, password);
        }
    }
}

void MainWindow::onLoginSuccess(const QString &token)
{
    updateStatus(tr("Авторизация выполнена успешно"));
    QMessageBox::information(this, tr("Успех"), tr("Добро пожаловать!"));
}

void MainWindow::onSyncAction(const QString &action, double position)
{
    if (action == "play") {
        m_mpvWidget->resume();
        m_isPlaying = true;
        m_playPauseButton->setIcon(QIcon(":/icons/pause.png"));
    m_playPauseButton->setText(tr("Pause"));
    } else if (action == "pause") {
        m_mpvWidget->pause();
        m_isPlaying = false;
        m_playPauseButton->setIcon(QIcon(":/icons/play.png"));
    m_playPauseButton->setText(tr("Play"));
    } else if (action == "seek") {
        m_mpvWidget->seek(position);
    }
}

void MainWindow::onPeerJoined(const QString &peerId)
{
    updateStatus(tr("Пир присоединился: %1").arg(peerId));
}

void MainWindow::onPeerLeft(const QString &peerId)
{
    updateStatus(tr("Пир покинул комнату: %1").arg(peerId));
}

// ── Слоты управления воспроизведением ───────────────────────────────────

void MainWindow::onPlayPause()
{
    if (m_isPlaying) {
        m_mpvWidget->pause();
        m_isPlaying = false;
        m_playPauseButton->setIcon(QIcon(":/icons/play.png"));
    m_playPauseButton->setText(tr("Play"));
        if (m_roomManager->isInRoom()) m_roomManager->syncPause();
    } else {
        m_mpvWidget->resume();
        m_isPlaying = true;
        m_playPauseButton->setIcon(QIcon(":/icons/pause.png"));
    m_playPauseButton->setText(tr("Pause"));
        if (m_roomManager->isInRoom()) m_roomManager->syncPlay();
    }
}

void MainWindow::onSeek(int value)
{
    if (m_duration <= 0) return;
    const int seekSliderMax = 1000;
    double position = (static_cast<double>(value) / seekSliderMax) * m_duration;
    m_mpvWidget->seek(position);
    if (!m_isSeeking && m_roomManager->isInRoom()) m_roomManager->syncSeek(position);
    // Сообщаем backend-буферу новую позицию воспроизведения (в байтах) для
    // корректной расстановки приоритетов piece.
    if (!m_currentTorrentId.isEmpty() && m_duration > 0 && m_currentFileSize > 0) {
        const qint64 bytePos = static_cast<qint64>((position / m_duration) * m_currentFileSize);
        m_network->setBufferPosition(m_currentTorrentId, bytePos);
    }
}

void MainWindow::onSeekSliderPressed()
{
    m_isSeeking = true;
}

void MainWindow::onSeekSliderReleased()
{
    m_isSeeking = false;
    onSeek(m_seekSlider->value());
}

void MainWindow::onPositionChanged(double position)
{
    if (!m_isSeeking && m_duration > 0) {
        const int seekSliderMax = 1000;
        m_seekSlider->setValue(static_cast<int>((position / m_duration) * seekSliderMax));
    }
    updateTimeLabel(position, m_duration);
}

void MainWindow::onBufferInfoReceived(const QJsonObject &info)
{
    // Отображаем реальный процент буферизации из backend (честный прогресс-бар).
    const double percent = info["buffered_percent"].toDouble();
    m_bufferProgress->setValue(static_cast<int>(percent));
}

void MainWindow::onDurationChanged(double duration)
{
    m_duration = duration;
    m_seekSlider->setEnabled(duration > 0);
    updateTimeLabel(m_mpvWidget->position(), duration);
}

void MainWindow::onPlaybackFinished()
{
    m_isPlaying = false;
    m_bufferPollTimer->stop();
    m_currentTorrentId.clear();
    m_bufferProgress->setValue(0);
    m_playPauseButton->setIcon(QIcon(":/icons/play.png"));
    m_playPauseButton->setText(tr("Play"));
    updateStatus(tr("Воспроизведение завершено"));
}

void MainWindow::onPlaybackError(const QString &message)
{
    m_bufferPollTimer->stop();
    QMessageBox::critical(this, tr("Ошибка воспроизведения"), message);
    updateStatus(tr("Ошибка: %1").arg(message));
}

void MainWindow::onNetworkError(const QString &message)
{
    QMessageBox::warning(this, tr("Ошибка сети"), message);
    updateStatus(tr("Ошибка: %1").arg(message));
}

// ── Вспомогательные методы ──────────────────────────────────────────────

void MainWindow::updateStatus(const QString &message)
{
    m_statusLabel->setText(message);
    qDebug() << "Status:" << message;
}

void MainWindow::updateTimeLabel(double position, double duration)
{
    m_timeLabel->setText(QString("%1 / %2").arg(formatTime(position)).arg(formatTime(duration)));
}

QString MainWindow::formatTime(double seconds) const
{
    if (seconds < 0) seconds = 0;
    int totalSeconds = static_cast<int>(seconds);
    int hours = totalSeconds / 3600;
    int minutes = (totalSeconds % 3600) / 60;
    int secs = totalSeconds % 60;
    return QString("%1:%2:%3")
        .arg(hours, 2, 10, QChar('0'))
        .arg(minutes, 2, 10, QChar('0'))
        .arg(secs, 2, 10, QChar('0'));
}

void MainWindow::updateRoomUI(const QString &roomId, bool isHost)
{
    m_roomManager->setHost(isHost);
    m_roomStatusLabel->setText(isHost ? tr("Комната: %1 (хост)").arg(roomId) : tr("Комната: %1").arg(roomId));
    m_createRoomButton->setEnabled(false);
    m_joinRoomButton->setEnabled(false);
    m_leaveRoomButton->setEnabled(true);
}

void MainWindow::clearRoomUI()
{
    m_roomStatusLabel->setText(tr("Не в комнате"));
    m_createRoomButton->setEnabled(true);
    m_joinRoomButton->setEnabled(true);
    m_leaveRoomButton->setEnabled(false);
}

// ── Graceful degradation ────────────────────────────────────────────────

void MainWindow::onServerUnavailable()
{
    m_serverConnected = false;
    qWarning() << "MainWindow: сервер недоступен, переключение в offline режим";

    // Показываем уведомление пользователю
    updateStatus(tr("⚠ Сервер недоступен - работа в offline режиме"));

    // Отключаем кнопки, требующие связи с сервером
    m_addButton->setEnabled(false);
    m_removeTorrentButton->setEnabled(false);
    m_createRoomButton->setEnabled(false);
    m_joinRoomButton->setEnabled(false);

    // Показываем кэшированные данные если есть
    if (!m_cachedTorrents.isEmpty()) {
        m_torrentModel->loadFromJson(m_cachedTorrents);
        qDebug() << "MainWindow: восстановлен кэш торрентов (" << m_cachedTorrents.size() << ")";
    }
}

void MainWindow::onTorrentListReceived(const QJsonArray &torrents)
{
    // Кэшируем последний успешный список для graceful degradation при потере сервера.
    m_cachedTorrents = torrents;
}

void MainWindow::onServerAvailable()
{
    m_serverConnected = true;
    qWarning() << "MainWindow: сервер доступен, восстановление работы";

    // Восстанавливаем кнопки
    m_addButton->setEnabled(true);
    m_createRoomButton->setEnabled(true);
    m_joinRoomButton->setEnabled(true);

    // Обновляем данные с сервера
    m_torrentManager->listTorrents();

    updateStatus(tr("✓ Подключение восстановлено"));
}

// ── Graceful Shutdown ──────────────────────────────────────────────────

void MainWindow::closeEvent(QCloseEvent *event)
{
    qDebug() << "MainWindow: начало graceful shutdown";

    if (m_roomManager->isInRoom()) {
        qDebug() << "MainWindow: выход из комнаты перед закрытием";
        m_roomManager->leaveRoom();
    }

    if (m_mpvWidget) {
        qDebug() << "MainWindow: остановка MpvWidget";
        m_mpvWidget->pause();
    }

    qDebug() << "MainWindow: graceful shutdown завершён";
    event->accept();
}
