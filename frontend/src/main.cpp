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

#include "mainwindow.h"
#include "systemtray.h"

// ── Graceful Shutdown Support ──────────────────────────────────────────

#ifdef Q_OS_WIN
#include <windows.h>
#else
#include <signal.h>
#include <unistd.h>
#endif

// Глобальный указатель на процесс сервера для корректного завершения
static QProcess *g_serverProcess = nullptr;

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
    
    // Запускаем процесс с родительским объектом для автоматического удаления
    g_serverProcess = new QProcess(parent);
    g_serverProcess->setProgram(serverPath);
    g_serverProcess->setArguments(QStringList() << "--port" << "8889");
    
    // Подключаем сигналы для логирования
    QObject::connect(g_serverProcess, &QProcess::readyReadStandardOutput, []() {
        if (g_serverProcess) {
            qDebug() << "Server stdout:" << g_serverProcess->readAllStandardOutput();
        }
    });
    
    QObject::connect(g_serverProcess, &QProcess::readyReadStandardError, []() {
        if (g_serverProcess) {
            qWarning() << "Server stderr:" << g_serverProcess->readAllStandardError();
        }
    });
    
    QObject::connect(g_serverProcess, QOverload<int, QProcess::ExitStatus>::of(&QProcess::finished),
        [](int exitCode, QProcess::ExitStatus exitStatus) {
            qWarning() << "Server завершился с кодом:" << exitCode
                      << "статус:" << exitStatus;
            // Очищаем глобальный указатель после завершения
            g_serverProcess = nullptr;
        });
    
    // Запускаем
    g_serverProcess->start();
    
    if (!g_serverProcess->waitForStarted(5000)) {
        qWarning() << "Не удалось запустить Go backend:" << g_serverProcess->errorString();
        delete g_serverProcess;
        g_serverProcess = nullptr;
        return false;
    }
    
    qDebug() << "Go backend запущен, PID:" << g_serverProcess->processId();
    
    // Асинхронное ожидание готовности сервера (не блокируем UI-поток)
    qDebug() << "Ожидание готовности сервера...";
    QTimer::singleShot(2000, []() {
        if (g_serverProcess && g_serverProcess->state() == QProcess::Running) {
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
    
    if (g_serverProcess && g_serverProcess->state() == QProcess::Running) {
        qDebug() << "Завершение серверного процесса...";
        
        // Отключаем сигналы чтобы избежать вызовов на удаляемом объекте
        g_serverProcess->disconnect();
        
        // Мягкое завершение
        g_serverProcess->terminate();
        
        // Ждём завершения до 5 секунд
        if (!g_serverProcess->waitForFinished(5000)) {
            qWarning() << "Сервер не завершился, принудительное убийство...";
            g_serverProcess->kill();
            g_serverProcess->waitForFinished(3000);
        }
        
        // Очищаем указатель (объект удалится автоматически через parent)
        g_serverProcess = nullptr;
    }
}

/**
 * @brief Настройка стиля приложения
 */
void setupStyle()
{
    // Устанавливаем стиль Fusion для кроссплатформенного вида
    QApplication::setStyle(QStyleFactory::create("Fusion"));
    
    // Тёмная палитра (опционально)
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
    
    // Применяем палитру (раскомментировать для тёмной темы)
    // QApplication::setPalette(darkPalette);
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
    
    // Включаем поддержку высокого DPI
    // QApplication::setAttribute(Qt::AA_EnableHighDpiScaling);  // deprecated in Qt 6
    // QApplication::setAttribute(Qt::AA_UseHighDpiPixmaps);     // deprecated in Qt 6
    
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
    parser.setApplicationDescription("TorrPlayer - Видеоплеер с торрентами и синхронизацией");
    
    // Опция: автозапуск Go backend
    QCommandLineOption startServerOption(
        QStringList() << "s" << "start-server",
        "Автоматически запустить Go backend сервер"
    );
    parser.addOption(startServerOption);
    
    // Опция: URL сервера
    QCommandLineOption serverUrlOption(
        QStringList() << "u" << "server-url",
        "URL сервера TorrServer",
        "url", "http://localhost:8889"
    );
    parser.addOption(serverUrlOption);
    
    // Опция: без трея
    QCommandLineOption noTrayOption(
        QStringList() << "t" << "no-tray",
        "Не использовать системный трей"
    );
    parser.addOption(noTrayOption);
    
    // Опция: версия
    QCommandLineOption versionOption(
        QStringList() << "v" << "version",
        "Показать версию"
    );
    parser.addOption(versionOption);
    
    parser.process(app);
    
    // Показать версию
    if (parser.isSet(versionOption)) {
        QTextStream out(stdout);
        out << "TorrPlayer v1.0.0\n";
        out << "Qt " << QT_VERSION_STR << "\n";
        out << "Собрано: " << __DATE__ << " " << __TIME__ << "\n";
        return 0;
    }
    
    // ── Настройка стиля ───────────────────────────────────────────────
    setupStyle();
    
    // ── Запуск Go backend (если указан флаг) ──────────────────────────
    if (parser.isSet(startServerOption)) {
        if (!startGoServer(&app)) {
            qWarning() << "Не удалось запустить Go backend, продолжаем без него";
        }
    }
    
    // ── Создание главного окна ────────────────────────────────────────
    MainWindow mainWindow;
    
    // Устанавливаем URL сервера если указан
    if (parser.isSet(serverUrlOption)) {
        QString serverUrl = parser.value(serverUrlOption);
        // TODO: передать URL в NetworkManager
        qDebug() << "URL сервера:" << serverUrl;
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
    
    // Сначала останавливаем серверный процесс
    stopGoServer();
    
    if (tray) {
        tray->setVisible(false);
        delete tray;
    }
    
    qDebug() << "Код возврата:" << result;
    return result;
}
