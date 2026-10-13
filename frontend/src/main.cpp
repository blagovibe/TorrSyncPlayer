/*
 * SPDX-License-Identifier: MIT
 * Copyright (c) 2025-2026 TorrSyncPlayer contributors
 * See LICENSE file for full license text
 */

/**
 * @file main.cpp
 * @brief Application entry point for TorrPlayer
 *
 * Initializes:
 * - QApplication with settings
 * - MainWindow window
 * - System tray (if available)
 * - Optional: auto-start Go backend
 * - Signal handlers for graceful shutdown
 */

#include <QApplication>
#include <QDebug>
#include <QDir>
#include <QLibraryInfo>
#include <QLocale>
#include <QProcess>
#include <QSettings>
#include <QStandardPaths>
#include <QCommandLineParser>
#include <QStyleFactory>
#include <QFile>
#include <QFileDevice>
#include <QFileInfo>
#include <QTextStream>
#include <QTimer>
#include <QTranslator>
#include <QUuid>
#include <QPointer>
#include <QAtomicInt>
#include <QCryptographicHash>
#include <QSharedPointer>
#include <QTemporaryFile>
#include <QTemporaryDir>
#include <utility>

#include "mainwindow.h"
#include "systemtray.h"
#include "backendargs.h"

// ── Graceful Shutdown Support ──────────────────────────────────────────

#ifdef Q_OS_WIN
#include <windows.h>
#else
#include <signal.h>
#include <unistd.h>
#endif

// QPointer for thread-safe access to server process
// QPointer automatically becomes nullptr when object is deleted
static QPointer<QProcess> g_serverProcess;

// Язык, запрошенный ключом --lang. Пустая строка означает «не задан», и тогда
// выбор делает resolveLanguage(): настройка → язык системы.
static QString g_requestedLanguage;

// Atomic flag to prevent re-entrant shutdown across threads (signal handlers)
// Signal handlers must only set this flag, NOT call Qt API (not async-signal-safe)
static QAtomicInt g_shuttingDown{0};

#ifdef Q_OS_WIN
/**
 * @brief Set up UTF-8 console output on Windows to prevent garbled output
 * Windows console uses CP866/CP1251 by default, but Qt outputs UTF-8.
 */
static void setupConsoleUTF8()
{
    SetConsoleOutputCP(CP_UTF8);
    SetConsoleCP(CP_UTF8);
}

/**
 * @brief Windows console event handler (Ctrl+C, Close, Logoff, Shutdown)
 * Async-signal-safe: only sets an atomic flag, does not call Qt API.
 */
static BOOL WINAPI consoleHandler(DWORD signal)
{
    switch (signal) {
    case CTRL_C_EVENT:
    case CTRL_CLOSE_EVENT:
    case CTRL_LOGOFF_EVENT:
    case CTRL_SHUTDOWN_EVENT:
        g_shuttingDown.storeRelaxed(1);
        return TRUE;
    default:
        return FALSE;
    }
}
#else
/**
 * @brief POSIX signal handler (SIGINT, SIGTERM)
 * Async-signal-safe: only sets an atomic flag, does not call Qt API.
 */
static void signalHandler(int)
{
    g_shuttingDown.storeRelaxed(1);
}
#endif

/**
 * @brief Setup signal handlers for graceful shutdown
 * Supports Windows (Ctrl+C, Close) and POSIX (SIGINT, SIGTERM)
 */
static void setupSignalHandlers()
{
#ifdef Q_OS_WIN
    if (!SetConsoleCtrlHandler(consoleHandler, TRUE)) {
        qWarning() << "Failed to set Windows console event handler";
    } else {
        qDebug() << "Windows console event handler installed";
    }
#else
    struct sigaction sa;
    sa.sa_handler = signalHandler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;

    if (sigaction(SIGINT, &sa, nullptr) != 0) {
        qWarning() << "Failed to set SIGINT handler";
    } else {
        qDebug() << "SIGINT handler installed";
    }

    if (sigaction(SIGTERM, &sa, nullptr) != 0) {
        qWarning() << "Failed to set SIGTERM handler";
    } else {
        qDebug() << "SIGTERM handler installed";
    }
#endif
}

/**
 * @brief Start Go backend server
 * @param parent Parent object for QProcess (ensures automatic deletion)
 * @return true if server started successfully
 */
// Uses QTemporaryDir + QTemporaryFile for safe extraction — eliminates TOCTOU symlink race
// by creating a uniquely-named directory and file that cannot be predicted by an attacker.
// The file is marked executable, the directory auto-deletes on application exit.
static QSharedPointer<QTemporaryDir> g_embeddedBackendDir;
static QSharedPointer<QTemporaryFile> g_embeddedBackendFile;

bool extractEmbeddedBackend()
{
    QFile resourceFile(":/embedded/torrsyncplayer.exe");
    if (!resourceFile.exists()) {
        return false;
    }

#ifdef EMBEDDED_BACKEND_SHA256
    QByteArray expectedHash = QByteArray::fromHex(EMBEDDED_BACKEND_SHA256);
#else
    QByteArray expectedHash;
#endif
    if (expectedHash.isEmpty()) {
        qWarning() << "EMBEDDED_BACKEND_SHA256 not defined — skipping integrity verification";
    }

    if (!resourceFile.open(QIODevice::ReadOnly)) {
        qWarning() << "Failed to open embedded backend resource";
        return false;
    }
    QByteArray resourceData = resourceFile.readAll();
    resourceFile.close();

    if (!expectedHash.isEmpty()) {
        QByteArray actualHash = QCryptographicHash::hash(resourceData, QCryptographicHash::Sha256);
        if (actualHash != expectedHash) {
            qWarning() << "Embedded backend integrity check FAILED — hash mismatch";
            return false;
        }
        qDebug() << "Embedded backend integrity verified (SHA-256)";
    }

    g_embeddedBackendDir = QSharedPointer<QTemporaryDir>::create(
        QStandardPaths::writableLocation(QStandardPaths::TempLocation) + "/TorrSyncPlayer_XXXXXX");
    if (!g_embeddedBackendDir || !g_embeddedBackendDir->isValid()) {
        qWarning() << "Failed to create temp directory";
        return false;
    }
    g_embeddedBackendDir->setAutoRemove(true);

    g_embeddedBackendFile = QSharedPointer<QTemporaryFile>::create(
        g_embeddedBackendDir->filePath("torrsyncplayer_XXXXXX.exe"));
    g_embeddedBackendFile->setAutoRemove(false);

    if (!g_embeddedBackendFile->open()) {
        qWarning() << "Failed to create temp file:" << g_embeddedBackendFile->errorString();
        return false;
    }

    if (g_embeddedBackendFile->write(resourceData) != resourceData.size()) {
        qWarning() << "Failed to write embedded backend:" << g_embeddedBackendFile->errorString();
        return false;
    }

    g_embeddedBackendFile->flush();

    QFile::setPermissions(g_embeddedBackendFile->fileName(),
        QFileDevice::ExeOwner | QFileDevice::ReadOwner | QFileDevice::WriteOwner);

    qDebug() << "Embedded backend extracted:" << g_embeddedBackendFile->fileName();
    return true;
}

bool startGoServer(QObject *parent)
{
    QString serverPath;

#ifdef HAS_EMBEDDED_BACKEND
    if (extractEmbeddedBackend()) {
        serverPath = g_embeddedBackendFile->fileName();
    }
#endif

    if (serverPath.isEmpty()) {
        QString appDir = QApplication::applicationDirPath();
        QStringList candidatePaths;
        candidatePaths << appDir + "/torrserver/torrserver.exe";
        candidatePaths << appDir + "/../torrserver/torrserver.exe";
        candidatePaths << appDir + "/torrsyncplayer.exe";
        candidatePaths << appDir + "/torrserver.exe";

        for (const QString &path : candidatePaths) {
            if (path.contains("..")) {
                continue;
            }
            if (QFile::exists(path)) {
                QFileDevice::Permissions perm = QFile::permissions(path);
                if (perm & QFileDevice::WriteOther) {
                    qWarning() << "Server binary has suspicious world-writable permissions:" << path;
                }
                serverPath = path;
                break;
            }
        }
    }

    if (serverPath.isEmpty()) {
        qWarning() << "Go backend not found";
        return false;
    }

    qDebug() << "Starting Go backend:" << serverPath;

    // Create process with parent object for automatic deletion
    QProcess *process = new QProcess(parent);
    process->setProgram(serverPath);
    
    // Desktop version uses HTTP (safe for localhost-only connections).
    //
    // The argument list lives in backendargs.cpp so it can be tested: main.cpp
    // is in no test target, and an unknown flag makes the Go backend exit(2),
    // which is exactly how --jwt-secret survived the removal of JWT auth and
    // broke the Windows release with a fully green pipeline.
    process->setArguments(BackendArgs::build(QStringLiteral("8889")));

    // Connect logging signals (with read limit)
    QPointer<QProcess> p = process;
    QObject::connect(process, &QProcess::readyReadStandardOutput, [p]() {
        if (p) {
            QByteArray data = p->readAllStandardOutput();
            if (data.size() > 1024 * 1024) {
                qDebug() << "Server stdout:" << data.left(1024) << "... [" << data.size() << "bytes total]";
            } else {
                qDebug() << "Server stdout:" << data;
            }
        }
    });

    QObject::connect(process, &QProcess::readyReadStandardError, [p]() {
        if (p) {
            QByteArray data = p->readAllStandardError();
            // Filter potential sensitive data: truncate to 512 bytes max
            if (data.size() > 512) {
                qWarning() << "Server stderr:" << data.left(512).trimmed() << "... [" << data.size() << "bytes total]";
            } else {
                qWarning() << "Server stderr:" << data.trimmed();
            }
        }
    });

    QObject::connect(process, QOverload<int, QProcess::ExitStatus>::of(&QProcess::finished),
        [](int exitCode, QProcess::ExitStatus exitStatus) {
            qWarning() << "Server exited with code:" << exitCode
                      << "status:" << exitStatus;
            // QPointer automatically becomes nullptr when object is deleted
        });

    // Save pointer via QPointer
    g_serverProcess = process;

    // Connect started signal (async, non-blocking UI)
    QObject::connect(process, &QProcess::started, [p]() {
        if (p) {
            qDebug() << "Go backend started, PID:" << p->processId();
        }

        // Async wait for server readiness
        QTimer::singleShot(2000, []() {
            QProcess *server = g_serverProcess;
            if (server && server->state() == QProcess::Running) {
                qDebug() << "Server ready for work";
            } else {
                qWarning() << "Server not ready after wait";
            }
        });
    });

    // Startup timeout (non-blocking UI)
    QTimer *startTimeout = new QTimer(process);
    startTimeout->setSingleShot(true);
    QObject::connect(startTimeout, &QTimer::timeout, [p, startTimeout]() {
        QProcess *proc = p;
        if (proc && proc->state() != QProcess::Running) {
            qWarning() << "Failed to start Go backend: timeout";
            g_serverProcess = nullptr;
            proc->kill();
            proc->deleteLater();
        }
        startTimeout->deleteLater();
    });
    startTimeout->start(5000);

    // Start process
    process->start();

    return true;
}

/**
 * @brief Gracefully stop server process
 * Called on application exit
 */
void stopGoServer()
{
    if (g_shuttingDown.loadRelaxed()) {
        return;
    }
    g_shuttingDown.storeRelaxed(1);

    // QPointer is safe for nullptr dereference
    QProcess *process = g_serverProcess;
    g_serverProcess = nullptr;
    
    if (process && process->state() == QProcess::Running) {
        qDebug() << "Terminating server process...";

        // Disconnect signals to avoid calls on deleted object
        process->disconnect();

        // Graceful termination
        process->terminate();

        // Async wait — non-blocking UI thread
        if (!process->waitForFinished(5000)) {
            qWarning() << "Server did not terminate in 5s, force killing...";
            process->kill();
            // Minimal kill wait (300ms instead of 3000ms)
            process->waitForFinished(300);
        }
    }

    // Clean up temp directory after server process ends
    if (g_embeddedBackendFile) {
        g_embeddedBackendFile->close();
        g_embeddedBackendFile.clear();
    }
    if (g_embeddedBackendDir) {
        g_embeddedBackendDir->setAutoRemove(true);
        g_embeddedBackendDir.clear();
    }
}

/**
 * @brief Setup application style
 */
void setupStyle()
{
    QApplication::setStyle(QStyleFactory::create("Fusion"));
}

// ── Интернационализация (i18n) ──────────────────────────────────────────

// Переводчик держится всё время работы приложения: если дать ему умереть,
// Qt продолжит показывать исходные строки вместо переведённых.
static QTranslator *g_appTranslator = nullptr;

// Язык, для которого удалось загрузить перевод. Пустая строка означает
// «используем язык исходников» — это штатный режим, а не ошибка.
static QString g_loadedLanguage;

/**
 * @brief Перевести путь в файловой системе приложения
 * @return true, если файл читается
 */
static bool translationsDirExists(const QString &dir)
{
    return !dir.isEmpty() && QFileInfo(dir).isDir();
}

/**
 * @brief Найти и загрузить перевод для указанного языка
 *
 * Перевод ищется в нескольких местах, потому что «правильного» нет:
 * запущенная из сборки копия, установленная в систему и AppDir у собранного
 * бинарника лежат в разных каталогах, а у portable-сборки переводы рядом с
 * exe — единственное разумное место.
 *
 * @param code код языка ("ru", "en") или пустая строка для языка исходников
 * @return true, если перевод загружен
 */
static bool loadTranslation(const QString &code)
{
    if (code.isEmpty()) {
        return true; // язык исходников — ничего загружать не нужно
    }

    delete g_appTranslator;
    g_appTranslator = new QTranslator();

    const QString qmName = QString("TorrSyncPlayer_%1.qm").arg(code);

    QStringList candidates;
    // Рядом с exe: portable-сборка, AppImage, распакованный AppDir
    candidates << QCoreApplication::applicationDirPath() + "/translations/" + qmName;
#ifdef Q_OS_MACOS
    candidates << QCoreApplication::applicationDirPath() + "/../Resources/translations/" + qmName;
#endif
    // Установленная сборка: cmake ставит в share/<имя>/translations
    candidates << QCoreApplication::applicationDirPath()
                     + "/../share/" + QCoreApplication::applicationName()
                     + "/translations/" + qmName;
    candidates << QCoreApplication::applicationDirPath()
                     + "/../share/TorrSyncPlayer/translations/" + qmName;
    // Qt-системные переводы — на случай, если мы переведём и стандартные
    // диалоги (QMessageBox и прочие), а не только свой текст.
    candidates << QLibraryInfo::path(QLibraryInfo::TranslationsPath) + "/" + qmName;

    for (const QString &path : std::as_const(candidates)) {
        if (!translationsDirExists(QFileInfo(path).absolutePath())) {
            continue;
        }
        if (g_appTranslator->load(path)) {
            QCoreApplication::installTranslator(g_appTranslator);
            qDebug() << "Loaded translation:" << path;
            g_loadedLanguage = code;
            return true;
        }
    }

    delete g_appTranslator;
    g_appTranslator = nullptr;

    // Нет перевода — не беда: приложение полностью работоспособно на
    // языке исходников. Молча падать здесь нельзя.
    qWarning() << "Translation not found for" << code << "- falling back to source language";
    return false;
}

/**
 * @brief Определить язык из настроек, аргументов или системы
 *
 * Порядок: явный --lang → сохранённая настройка → язык системы. Язык системы
 * используется, только если он реально поддержан: иначе пользователь с
 * немецкой локалью получил бы немецкий интерфейс из ничего.
 *
 * @return код языка или пустая строка для языка исходников
 */
static QString resolveLanguage()
{
    const QStringList supported = {"ru", "en"};
    auto isSupported = [&supported](const QString &code) {
        return !code.isEmpty() && supported.contains(code);
    };

    // 1. Явно запрошенный язык
    if (!g_requestedLanguage.isEmpty()) {
        return isSupported(g_requestedLanguage) ? g_requestedLanguage : QString();
    }

    // 2. Выбор пользователя из прошлого запуска
    QSettings settings;
    const QString saved = settings.value("ui/language").toString();
    if (isSupported(saved)) {
        return saved;
    }
    // Пустая настройка — не «русский по умолчанию», а «выбирать по системе».
    // Иначе немецкая система получила бы русский интерфейс без спроса.
    if (!saved.isEmpty()) {
        return QString();
    }

    // 3. Язык системы: ru_RU → ru, en_US → en
    const QString system = QLocale::system().name().left(2);
    if (isSupported(system)) {
        return system;
    }

    // Система на неподдерживаемом языке — берём английский: исходники
    // написаны по-русски, и для не-русской системы это честный fallback.
    return QStringLiteral("en");
}

/**
 * @brief Загрузить переводы до создания виджетов
 *
 * Порядок важен: QTranslator надо установить до того, какMainWindow создаст
 * подписи и надписи. Установленные позже переводы не применятся к уже
 * созданным строкам, и интерфейс останется смешанным.
 */
static void setupTranslations()
{
    g_loadedLanguage = resolveLanguage();
    loadTranslation(g_loadedLanguage);
}

/**
 * @brief Main function
 * @param argc argument count
 * @param argv arguments
 * @return exit code
 */
int main(int argc, char *argv[])
{
    // ── Setup signal handlers (before QApplication creation) ──────
    setupSignalHandlers();

#ifdef Q_OS_WIN
    setupConsoleUTF8();
#endif

    // ── Application setup ──────────────────────────────────────────

    // Create application
    QApplication app(argc, argv);

    // Application metadata
    QApplication::setApplicationName("TorrPlayer");
#ifndef APP_VERSION
#define APP_VERSION "1.1.0"
#endif
    QApplication::setApplicationVersion(APP_VERSION);
    QApplication::setOrganizationName("TorrPlayer");
    QApplication::setOrganizationDomain("torrplayer.app");

    // Logging
    qDebug() << "=== TorrPlayer v" << APP_VERSION << "===";
    qDebug() << "Qt version:" << QT_VERSION_STR;
    qDebug() << "Application directory:" << QApplication::applicationDirPath();

    // ── Parse arguments ────────────────────────────────────────────────
    QCommandLineParser parser;
    parser.setApplicationDescription("TorrPlayer - torrent player with P2P sync");
    parser.addHelpOption();
    parser.addVersionOption();

    // Option: server URL
    QCommandLineOption serverUrlOption(
        QStringList() << "s" << "server-url",
        "Server URL (e.g., http://localhost:8889)",
        "url");
    parser.addOption(serverUrlOption);

    // Option: access token (required for API access, generated by backend at startup)
    QCommandLineOption accessTokenOption(
        QStringList() << "a" << "access-token",
        "Access token for API authentication (generated by server at startup)",
        "token");
    parser.addOption(accessTokenOption);

    // Option: client ID (identifies this player instance in a room)
    QCommandLineOption clientIdOption(
        QStringList() << "c" << "client-id",
        "Client identifier for P2P room (auto-generated if not provided)",
        "id");
    parser.addOption(clientIdOption);

    // Option: no tray
    QCommandLineOption noTrayOption(
        QStringList() << "no-tray",
        "Don't use system tray");
    parser.addOption(noTrayOption);

    // Option: dark theme
    QCommandLineOption darkThemeOption(
        QStringList() << "dark-theme",
        "Use dark theme");
    parser.addOption(darkThemeOption);

    // Option: язык интерфейса. Без неё выбор делает resolveLanguage():
    // сохранённая настройка, иначе язык системы.
    // Именно QCoreApplication::translate, а не tr(): main() — не метод
    // QObject, у него нет контекста, и lupdate такую строку пропускает.
    QCommandLineOption langOption(
        QStringList() << "lang",
        QCoreApplication::translate("CommandLine", "Interface language: ru, en (default: saved setting or system language)"),
        "code");
    parser.addOption(langOption);

    // Parse
    parser.process(app);

    g_requestedLanguage = parser.value(langOption).trimmed().toLower();

    // ── Setup style ────────────────────────────────────────────────
    setupStyle();

    // Переводы — до создания любых виджетов: подписи и надписи берут текст
    // в конструкторах, и установленный позже перевод к ним уже не применится.
    setupTranslations();

    if (parser.isSet(darkThemeOption)) {
        QPalette darkPalette;
        darkPalette.setColor(QPalette::Window, QColor(53, 53, 53));
        darkPalette.setColor(QPalette::WindowText, Qt::white);
        darkPalette.setColor(QPalette::Base, QColor(25, 25, 25));
        darkPalette.setColor(QPalette::AlternateBase, QColor(53, 53, 53));
        darkPalette.setColor(QPalette::ToolTipBase, Qt::white);
        darkPalette.setColor(QPalette::ToolTipText, Qt::white);
        darkPalette.setColor(QPalette::Text, Qt::white);
        darkPalette.setColor(QPalette::Button, QColor(53, 53, 53));
        darkPalette.setColor(QPalette::ButtonText, Qt::white);
        darkPalette.setColor(QPalette::BrightText, Qt::red);
        darkPalette.setColor(QPalette::Link, QColor(42, 130, 218));
        darkPalette.setColor(QPalette::Highlight, QColor(42, 130, 218));
        darkPalette.setColor(QPalette::HighlightedText, Qt::black);
        app.setPalette(darkPalette);
    }

    // ── Start Go backend ─────────────────────────────────────────────
    bool serverStarted = false;
    if (!parser.isSet(serverUrlOption)) {
        // Start embedded server if URL not specified
        serverStarted = startGoServer(&app);
        if (!serverStarted) {
            qWarning() << "Failed to start embedded server";
        }
    }

    // ── Create main window ─────────────────────────────────────────────
    MainWindow mainWindow;

    // Set server URL if specified
    if (parser.isSet(serverUrlOption)) {
        QUrl serverUrl(parser.value(serverUrlOption));
        mainWindow.setServerUrl(serverUrl);
        qDebug() << "Server URL:" << serverUrl.toString();
    }

    // Apply access token and client ID if provided
    if (parser.isSet(accessTokenOption)) {
        QString token = parser.value(accessTokenOption);
        mainWindow.setAccessToken(token);
        qDebug() << "Access token provided at startup";
    }

    QString clientId;
    if (parser.isSet(clientIdOption)) {
        clientId = parser.value(clientIdOption);
    } else {
        // Generate a random client ID for this session
        QUuid uuid = QUuid::createUuid();
        clientId = uuid.toString(QUuid::WithoutBraces);
    }
    mainWindow.setClientId(clientId);
    qDebug() << "Client ID:" << clientId;

    // Initialize main window (loads torrent list)
    mainWindow.initialize();

    // Show window
    mainWindow.show();

    // ── Create system tray ─────────────────────────────────────────────
    SystemTray *tray = nullptr;

    if (!parser.isSet(noTrayOption)) {
        tray = new SystemTray(&mainWindow);

        if (tray->init()) {
            qDebug() << "System tray initialized";

            // Connect tray signals
            QObject::connect(tray, &SystemTray::quitRequested, &app, &QApplication::quit);
        } else {
            qDebug() << "System tray unavailable";
            delete tray;
            tray = nullptr;
        }
    }

    // ── Polling shutdown flag from signal handler ───────────────────
    // QApplication::quit() is NOT async-signal-safe, so we cannot call it
    // from signal handlers. Instead, signal handlers set g_shuttingDown,
    // and this timer checks it from the event loop.
    QTimer shutdownPollTimer;
    shutdownPollTimer.setInterval(200);
    QObject::connect(&shutdownPollTimer, &QTimer::timeout, [&app]() {
        if (g_shuttingDown.loadRelaxed()) {
            qDebug() << "Shutdown signal received, initiating shutdown";
            app.quit();
        }
    });
    shutdownPollTimer.start();

    // ── Run event loop ───────────────────────────────────────────
    qDebug() << "Launching event loop...";

    int result = app.exec();

    // ── Cleanup ───────────────────────────────────────────────────────
    qDebug() << "Application shutdown...";

    if (tray) {
        tray->setVisible(false);
        delete tray;
    }

    stopGoServer();

    qDebug() << "Exit code:" << result;
    return result;
}