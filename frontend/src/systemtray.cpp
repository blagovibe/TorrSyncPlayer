/**
 * @file systemtray.cpp
 * @brief Реализация системного трея для приложения TorrPlayer
 */

#include "systemtray.h"
#include "mainwindow.h"

#include <QDebug>
#include <QApplication>
#include <QStyle>
#include <QIcon>

SystemTray::SystemTray(MainWindow *mainWindow, QObject *parent)
    : QObject(parent)
    , m_mainWindow(mainWindow)
    , m_trayIcon(new QSystemTrayIcon(this))
    , m_menu(new QMenu())
    , m_showAction(nullptr)   // Инициализация указателей
    , m_hideAction(nullptr)   // Инициализация указателей
    , m_quitAction(nullptr)   // Инициализация указателей
{
    qDebug() << "SystemTray: создан";
}

SystemTray::~SystemTray()
{
    qDebug() << "SystemTray: уничтожен";
}

bool SystemTray::init()
{
    // Проверяем доступность системного трея
    m_trayAvailable = QSystemTrayIcon::isSystemTrayAvailable();
    
    if (!m_trayAvailable) {
        qWarning() << "SystemTray: системный трей недоступен";
        return false;
    }
    
    // Устанавливаем иконку
    QIcon appIcon = QApplication::style()->standardIcon(QStyle::SP_ComputerIcon);
    
    // Пробуем загрузить иконку из ресурсов
    QIcon resourceIcon(":/icons/app-icon.png");
    if (!resourceIcon.isNull()) {
        appIcon = resourceIcon;
    }
    
    m_trayIcon->setIcon(appIcon);
    m_trayIcon->setToolTip(tr("TorrPlayer - Видеоплеер"));
    
    // Создаём меню
    createMenu();
    
    // Устанавливаем меню
    m_trayIcon->setContextMenu(m_menu);
    
    // Подключаем сигналы
    connect(m_trayIcon, &QSystemTrayIcon::activated,
            this, &SystemTray::onActivated);
    
    // Показываем иконку
    m_trayIcon->show();
    
    qDebug() << "SystemTray: инициализирован";
    return true;
}

bool SystemTray::isAvailable() const
{
    return m_trayAvailable;
}

void SystemTray::setVisible(bool visible)
{
    if (m_trayAvailable) {
        m_trayIcon->setVisible(visible);
    }
}

bool SystemTray::isVisible() const
{
    return m_trayAvailable && m_trayIcon->isVisible();
}

void SystemTray::showNotification(const QString &title, const QString &message,
                                  QSystemTrayIcon::MessageIcon icon, int timeout)
{
    if (m_trayAvailable && QSystemTrayIcon::supportsMessages()) {
        m_trayIcon->showMessage(title, message, icon, timeout);
        qDebug() << "SystemTray: уведомление" << title << message;
    }
}

void SystemTray::setIcon(const QIcon &icon)
{
    if (m_trayAvailable) {
        m_trayIcon->setIcon(icon);
    }
}

void SystemTray::setToolTip(const QString &tooltip)
{
    if (m_trayAvailable) {
        m_trayIcon->setToolTip(tooltip);
    }
}

// ── Private slots ─────────────────────────────────────────────────────

void SystemTray::onActivated(QSystemTrayIcon::ActivationReason reason)
{
    switch (reason) {
    case QSystemTrayIcon::DoubleClick:
        // Двойной клик - показываем/скрываем окно
        if (m_mainWindow->isVisible()) {
            onHideAction();
        } else {
            onShowAction();
        }
        break;
        
    case QSystemTrayIcon::Trigger:
        // Одиночный клик - показываем окно
        onShowAction();
        break;
        
    case QSystemTrayIcon::MiddleClick:
        // Клик средней кнопкой - показываем меню
        break;
        
    default:
        break;
    }
}

void SystemTray::onShowAction()
{
    if (!m_mainWindow) return;
    
    // Показываем окно
    m_mainWindow->show();
    m_mainWindow->raise();
    m_mainWindow->activateWindow();
    
    // Если окно было свёрнуто - разворачиваем
    if (m_mainWindow->isMinimized()) {
        m_mainWindow->showNormal();
    }
    
    updateMenuState();
    emit showRequested();
    
    qDebug() << "SystemTray: показ окна";
}

void SystemTray::onHideAction()
{
    if (!m_mainWindow) return;
    
    // Скрываем окно
    m_mainWindow->hide();
    
    updateMenuState();
    emit hideRequested();
    
    // Показываем уведомление
    showNotification(tr("TorrPlayer"), 
                    tr("Приложение свёрнуто в трей. Двойной клик для восстановления."));
    
    qDebug() << "SystemTray: скрытие окна";
}

void SystemTray::onQuitAction()
{
    qDebug() << "SystemTray: запрос на выход";
    emit quitRequested();
    // QApplication::quit() вызывается через сигнал quitRequested в main.cpp
}

// ── Private methods ───────────────────────────────────────────────────

void SystemTray::createMenu()
{
    // Очищаем меню
    m_menu->clear();
    
    // Действие "Показать"
    m_showAction = m_menu->addAction(
        QApplication::style()->standardIcon(QStyle::SP_ComputerIcon),
        tr("Показать")
    );
    connect(m_showAction, &QAction::triggered, this, &SystemTray::onShowAction);
    
    // Действие "Скрыть"
    m_hideAction = m_menu->addAction(
        QApplication::style()->standardIcon(QStyle::SP_TitleBarMinButton),
        tr("Скрыть в трей")
    );
    connect(m_hideAction, &QAction::triggered, this, &SystemTray::onHideAction);
    
    m_menu->addSeparator();
    
    // Действие "Выход"
    m_quitAction = m_menu->addAction(
        QApplication::style()->standardIcon(QStyle::SP_DialogCloseButton),
        tr("Выход")
    );
    connect(m_quitAction, &QAction::triggered, this, &SystemTray::onQuitAction);
    
    // Обновляем состояние
    updateMenuState();
}

void SystemTray::updateMenuState()
{
    if (!m_mainWindow) return;
    
    bool isVisible = m_mainWindow->isVisible();
    m_showAction->setEnabled(!isVisible);
    m_hideAction->setEnabled(isVisible);
}
