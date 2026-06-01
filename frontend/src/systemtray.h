/**
 * @file systemtray.h
 * @brief Системный трей для приложения TorrPlayer
 * 
 * Обеспечивает:
 * - Иконку в системном трее
 * - Контекстное меню (Показать, Скрыть, Выход)
 * - Двойной клик для показа/скрытия окна
 * - Уведомления
 */

#ifndef SYSTEMTRAY_H
#define SYSTEMTRAY_H

#include <QSystemTrayIcon>
#include <QMenu>
#include <QAction>
#include <QObject>

class MainWindow;

/**
 * @class SystemTray
 * @brief Управление иконкой в системном трее
 * 
 * Позволяет свернуть приложение в трей и управлять им
 * через контекстное меню. Поддерживает уведомления
 * и двойной клик для показа/скрытия главного окна.
 */
class SystemTray : public QObject
{
    Q_OBJECT

public:
    /**
     * @brief Конструктор системного трея
     * Создаёт иконку и контекстное меню
     * @param mainWindow Указатель на главное окно
     * @param parent Родительский объект
     */
    explicit SystemTray(MainWindow *mainWindow, QObject *parent = nullptr);
    
    /**
     * @brief Деструктор
     * Удаляет иконку и меню
     */
    ~SystemTray();

    /**
     * @brief Инициализация иконки трея
     * Проверяет доступность системного трея и показывает иконку
     * @return true если трей доступен и инициализирован
     */
    bool init();
    
    /**
     * @brief Проверка доступности системного трея
     * @return true если трей доступен
     */
    bool isAvailable() const;
    
    /**
     * @brief Показать/скрыть иконку в трее
     * @param visible true для показа
     */
    void setVisible(bool visible);
    
    /**
     * @brief Проверить, видна ли иконка
     * @return true если иконка видна
     */
    bool isVisible() const;

    /**
     * @brief Показать уведомление
     * Отображает всплывающее уведомление в системном трее
     * @param title Заголовок
     * @param message Текст сообщения
     * @param icon Иконка уведомления
     * @param timeout Таймаут в мс
     */
    void showNotification(const QString &title, const QString &message,
                         QSystemTrayIcon::MessageIcon icon = QSystemTrayIcon::Information,
                         int timeout = 3000);

    /**
     * @brief Установить иконку
     * @param icon Новая иконка
     */
    void setIcon(const QIcon &icon);
    
    /**
     * @brief Установить подсказку
     * @param tooltip Текст подсказки
     */
    void setToolTip(const QString &tooltip);

signals:
    /**
     * @brief Сигнал запроса показа окна
     * Испускается при выборе "Показать" в меню или двойном клике
     */
    void showRequested();
    
    /**
     * @brief Сигнал запроса скрытия окна
     * Испускается при выборе "Скрыть" в меню
     */
    void hideRequested();
    
    /**
     * @brief Сигнал запроса выхода
     * Испускается при выборе "Выход" в меню
     */
    void quitRequested();

private slots:
    /**
     * @brief Обработка активации иконки (клик)
     * Обрабатывает одиночный и двойной клик
     * @param reason Причина активации
     */
    void onActivated(QSystemTrayIcon::ActivationReason reason);
    
    /**
     * @brief Обработка выбора "Показать"
     * Показывает и активирует главное окно
     */
    void onShowAction();
    
    /**
     * @brief Обработка выбора "Скрыть"
     * Скрывает главное окно в трей
     */
    void onHideAction();
    
    /**
     * @brief Обработка выбора "Выход"
     * Инициирует завершение приложения
     */
    void onQuitAction();

private:
    /**
     * @brief Создание контекстного меню
     * Создаёт меню с действиями: Показать, Скрыть, Выход
     */
    void createMenu();
    
    /**
     * @brief Обновление состояния меню
     * Обновляет доступность действий в зависимости от состояния окна
     */
    void updateMenuState();

    MainWindow *m_mainWindow;           ///< Главное окно приложения
    QSystemTrayIcon *m_trayIcon;        ///< Иконка в трее
    QMenu *m_menu;                      ///< Контекстное меню
    
    // Действия меню
    QAction *m_showAction;              ///< Показать окно
    QAction *m_hideAction;              ///< Скрыть окно
    QAction *m_quitAction;              ///< Выход
    
    bool m_trayAvailable = false;       ///< Флаг доступности трея
};

#endif // SYSTEMTRAY_H
