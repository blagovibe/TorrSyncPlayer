/*
 * SPDX-License-Identifier: MIT
 * Copyright (c) 2025-2026 TorrSyncPlayer contributors
 * See LICENSE file for full license text
 */

/**
 * @file main.cpp
 * @brief Точка входа приложения TorrPlayer
 *
 * Инициализирует:
 * - QApplication с настройками
 * - Главное окно MainWindow
 * - Системный трей (если доступен)
 * - Опционально: автозапуск Go backend
 * - Обработчики сигналов для graceful shutdown
 */

#include <QApplication>
#include <QDebug>
#include <QDir>
#include <QProcess>
#include <QStandardPaths>
#include <QCommandLineParser>
#include <QStyleFactory>
#include <QFile>
#include <QTextStream>
#include <QTimer>
#include <QPointer>

#include "mainwindow.h"
#include "systemtray.h"

// ── Graceful Shutdown Support ──────────────────────────────────────────

#ifdef Q_OS_WIN
#include <windows.h>
#else
#include <signal.h>
#include <unistd.h>
#endif

// QPointer для потокобезопасного доступа к процессу сервера
// QPointer автоматически становится nullptr при удалении объекта
static QPointer<QProcess> g_serverProcess;

// Флаг для предотвращения повторного завершения
static volatile bool g_shuttingDown = false;

#ifdef Q_OS_WIN
/**
 * @brief Обработчик консольных событий Windows (Ctrl+C, Close, Logoff, Shutdown)
 */
static BOOL WINAPI consoleHandler(DWORD signal)
{
    switch (signal) {
    case CTRL_C_EVENT:
    case CTRL_CLOSE_EVENT:
    case CTRL_LOGOFF_EVENT:
    case CTRL_SHUTDOWN_EVENT:
        qDebug() << "Получен сигнал завершения (Windows):" << signal;
        g_shuttingDown = true;
        QApplication::quit();
        return TRUE;
    default:
        return FALSE;
    }
}
#else
/**
 * @brief Обработчик POSIX сигналов (SIGINT, SIGTERM)
 */
static void signalHandler(int signal)
{
    qDebug() << "Получен сигнал завершения (POSIX):" << signal;
    g_shuttingDown = true;
    QApplication::quit();
}
#endif

/**
 * @brief Настройка обработчиков сигналов для graceful shutdown
 * Поддерживает Windows (Ctrl+C, Close) и POSIX (SIGINT, SIGTERM)
 */
static void setupSignalHandlers()
{
#ifdef Q_OS_WIN
    if (!SetConsoleCtrlHandler(consoleHandler, TRUE)) {
        qWarning() << "Не удалось установить обработчик консольных событий Windows";
    } else {
        qDebug() << "Обработчик консольных событий Windows установлен";
    }
#else
    struct sigaction sa;
    sa.sa_handler = signalHandler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;

    if (sigaction(SIGINT, &sa, nullptr) != 0) {
        qWarning() << "Не удалось установить обработчик SIGINT";
    } else {
        qDebug() << "Обработчик SIGINT установлен";
    }

    if (sigaction(SIGTERM, &sa, nullptr) != 0) {
        qWarning() << "Не удалось установить обработчик SIGTERM";
    } else {
        qDebug() << "Обработчик SIGTERM установлен";
    }
#endif
}

/**
 * @brief Запуск Go backend сервера
 * @param parent Родительский объект для QProcess (обеспечивает автоматическое удаление)
 * @return true если сервер запущен успешно
 */
bool startGoServer(QObject *parent)
{
    // Путь к исполняемому файлу сервера
    QString serverPath = QApplication::applicationDirPath() + "/torrserver/torrserver.exe";

    // Проверяем существование файла
    if (!QFile::exists(serverPath)) {
        // Пробуем альтернативный путь
        serverPath = QApplication::applicationDirPath() + "/../torrserver/torrserver.exe";

        if (!QFile::exists(serverPath)) {
            qWarning() << "Go backend не найден:" << serverPath;
            return false;
        }
    }

    qDebug() << "Запуск Go backend:" << serverPath;

    // Создаём процесс с родительским объектом для автоматического удаления
    QProcess *process = new QProcess(parent);
    process->setProgram(serverPath);
    process->setArguments(QStringList() << "--port" << "8889");

    // Подключаем сигналы для логирования
    QPointer<QProcess> p = process;
    QObject::connect(process, &QProcess::readyReadStandardOutput, [p]() {
        if (p) qDebug() << "Server stdout:" << p->readAllStandardOutput();
    });

    QObject::connect(process, &QProcess::readyReadStandardError, [p]() {
        if (p) qWarning() << "Server stderr:" << p->readAllStandardError();
    });

    QObject::connect(process, QOverload<int, QProcess::ExitStatus>::of(&QProcess::finished),
        [](int exitCode, QProcess::ExitStatus exitStatus) {
            qWarning() << "Server завершился с кодом:" << exitCode
                      << "статус:" << exitStatus;
            // QPointer автоматически станет nullptr при удалении объекта
        });

    // Сохраняем указатель через QPointer
    g_serverProcess = process;

    // Запускаем
    process->start();

    if (!process->waitForStarted(5000)) {
        qWarning() << "Не удалось запустить Go backend:" << process->errorString();
        g_serverProcess = nullptr;
        delete process;
        return false;
    }

    qDebug() << "Go backend запущен, PID:" << process->processId();

    // Асинхронное ожидание готовности сервера (не блокируем UI-поток)
    qDebug() << "Ожидание готовности сервера...";
    QTimer::singleShot(2000, []() {
        QProcess *p = g_serverProcess;
        if (p && p->state() == QProcess::Running) {
            qDebug() << "Сервер готов к работе";
        } else {
            qWarning() << "Сервер не готов после ожидания";
        }
    });

    return true;
}

/**
 * @brief Корректное завершение серверного процесса
 * Вызывается при выходе из приложения
 */
void stopGoServer()
{
    if (g_shuttingDown) {
        return; // Предотвращаем повторный вызов
    }
    g_shuttingDown = true;

    // QPointer безопасен от nullptr dereference
    QProcess *process = g_serverProcess;
    g_serverProcess = nullptr;
    
    if (process && process->state() == QProcess::Running) {
        qDebug() << "Завершение серверного процесса...";

        // Отключаем сигналы чтобы избежать вызовов на удаляемом объекте
        process->disconnect();

        // Мягкое завершение
        process->terminate();

        // Ждём завершения до 5 секунд
        if (!process->waitForFinished(5000)) {
            qWarning() << "Сервер не завершился, принудительное убийство...";
            process->kill();
            process->waitForFinished(3000);
        }
    }
}

/**
 * @brief Настройка стиля приложения
 */
void setupStyle()
{
    QApplication::setStyle(QStyleFactory::create("Fusion"));
}

/**
 * @brief Главная функция
 * @param argc Количество аргументов
 * @param argv Аргументы командной строки
 * @return Код возврата
 */
int main(int argc, char *argv[])
{
    // ── Настройка обработчиков сигналов (до создания QApplication) ──────
    setupSignalHandlers();

    // ── Настройка приложения ──────────────────────────────────────────

    // Создаём приложение
    QApplication app(argc, argv);

    // Метаданные приложения
    QApplication::setApplicationName("TorrPlayer");
    QApplication::setApplicationVersion("1.0.0");
    QApplication::setOrganizationName("TorrPlayer");
    QApplication::setOrganizationDomain("torrplayer.app");

    // Логирование
    qDebug() << "=== TorrPlayer v1.0.0 ===";
    qDebug() << "Qt версия:" << QT_VERSION_STR;
    qDebug() << "Директория приложения:" << QApplication::applicationDirPath();

    // ── Парсинг аргументов ────────────────────────────────────────────
    QCommandLineParser parser;
    parser.setApplicationDescription("TorrPlayer — торрент-плеер с P2P синхронизацией");
    parser.addHelpOption();
    parser.addVersionOption();

    // Опция: URL сервера
    QCommandLineOption serverUrlOption(
        QStringList() << "s" << "server-url",
        "URL сервера (например, http://localhost:8889)",
        "url");
    parser.addOption(serverUrlOption);

    // Опция: без трея
    QCommandLineOption noTrayOption(
        QStringList() << "no-tray",
        "Не использовать системный трей");
    parser.addOption(noTrayOption);

    // Опция: тёмная тема
    QCommandLineOption darkThemeOption(
        QStringList() << "dark-theme",
        "Использовать тёмную тему");
    parser.addOption(darkThemeOption);

    // Парсим
    parser.process(app);

    // ── Настройка стиля ───────────────────────────────────────────────
    setupStyle();

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

    // ── Запуск Go backend ─────────────────────────────────────────────
    bool serverStarted = false;
    if (!parser.isSet(serverUrlOption)) {
        // Запускаем встроенный сервер если URL не указан
        serverStarted = startGoServer(&app);
        if (!serverStarted) {
            qWarning() << "Не удалось запустить встроенный сервер";
        }
    }

    // ── Создание главного окна ────────────────────────────────────────
    MainWindow mainWindow;

    // Устанавливаем URL сервера если указан
    if (parser.isSet(serverUrlOption)) {
        QUrl serverUrl(parser.value(serverUrlOption));
        mainWindow.setServerUrl(serverUrl);
        qDebug() << "URL сервера:" << serverUrl.toString();
    }

    // Показываем окно
    mainWindow.show();

    // ── Создание системного трея ──────────────────────────────────────
    SystemTray *tray = nullptr;

    if (!parser.isSet(noTrayOption)) {
        tray = new SystemTray(&mainWindow);

        if (tray->init()) {
            qDebug() << "Системный трей инициализирован";

            // Подключаем сигналы трея
            QObject::connect(tray, &SystemTray::quitRequested, &app, &QApplication::quit);
        } else {
            qDebug() << "Системный трей недоступен";
            delete tray;
            tray = nullptr;
        }
    }

    // ── Запуск цикла событий ─────────────────────────────────────────
    qDebug() << "Запуск цикла событий...";

    int result = app.exec();

    // ── Очистка ───────────────────────────────────────────────────────
    qDebug() << "Завершение приложения...";

    if (tray) {
        tray->setVisible(false);
        delete tray;
    }

    stopGoServer();

    qDebug() << "Код возврата:" << result;
    return result;
}
