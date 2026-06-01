/**
 * @file roomdialog.h
 * @brief Диалог создания/присоединения к комнате
 * 
 * Предоставляет интерфейс для:
 * - Создания новой комнаты (имя, пароль)
 * - Присоединения к существующей комнате (ID, пароль)
 */

#ifndef ROOMDIALOG_H
#define ROOMDIALOG_H

#include <QDialog>
#include <QLineEdit>
#include <QPushButton>
#include <QCheckBox>
#include <QLabel>
#include <QTabWidget>
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QGroupBox>
#include <QFormLayout>

/**
 * @class RoomDialog
 * @brief Диалог для работы с комнатами синхронизации
 * 
 * Содержит две вкладки:
 * - "Создать" - для создания новой комнаты
 * - "Присоединиться" - для входа в существующую комнату
 * Поддерживает валидацию введённых данных.
 */
class RoomDialog : public QDialog
{
    Q_OBJECT

public:
    /**
     * @brief Режим диалога
     */
    enum DialogMode {
        CreateMode,     ///< Режим создания комнаты
        JoinMode        ///< Режим присоединения к комнате
    };

    /**
     * @brief Конструктор диалога
     * Создаёт интерфейс с вкладками для создания/присоединения
     * @param mode Режим работы диалога
     * @param parent Родительский виджет
     */
    explicit RoomDialog(DialogMode mode = CreateMode, QWidget *parent = nullptr);
    
    /**
     * @brief Деструктор
     */
    ~RoomDialog();

    // ── Геттеры для получения данных ──────────────────────────────────
    
    /**
     * @brief Получить имя комнаты (для создания)
     * @return Имя комнаты
     */
    QString roomName() const;
    
    /**
     * @brief Получить ID комнаты (для присоединения)
     * @return ID комнаты
     */
    QString roomId() const;
    
    /**
     * @brief Получить пароль
     * @return пароль (может быть пустым)
     */
    QString password() const;
    
    /**
     * @brief Проверить, установлен ли флаг "хост"
     * @return true если пользователь является хостом
     */
    bool isHost() const;

private slots:
    /**
     * @brief Обработка нажатия OK
     * Валидирует введённые данные перед закрытием
     */
    void onAccept();
    
    /**
     * @brief Обработка переключения вкладок
     * Обновляет режим диалога при смене вкладки
     * @param index Индекс выбранной вкладки
     */
    void onTabChanged(int index);
    
    /**
     * @brief Переключение видимости пароля
     * @param checked true для показа пароля
     */
    void onTogglePassword(bool checked);

private:
    /**
     * @brief Создание вкладки "Создать комнату"
     * Содержит поля для имени и пароля комнаты
     * @return Виджет вкладки
     */
    QWidget* createCreateTab();
    
    /**
     * @brief Создание вкладки "Присоединиться"
     * Содержит поля для ID и пароля комнаты
     * @return Виджет вкладки
     */
    QWidget* createJoinTab();
    
    /**
     * @brief Валидация введённых данных
     * Проверяет заполнение обязательных полей
     * @return true если данные корректны
     */
    bool validateInput();
    
    /**
     * @brief Показать ошибку валидации
     * Отображает сообщение об ошибке в статусной метке
     * @param message Сообщение об ошибке
     */
    void showValidationError(const QString &message);

    // ── Элементы UI ───────────────────────────────────────────────────
    
    QTabWidget *m_tabWidget;        ///< Вкладки диалога
    
    // Вкладка "Создать"
    QLineEdit *m_nameEdit;          ///< Поле имени комнаты
    QLineEdit *m_createPasswordEdit; ///< Поле пароля для создания
    QCheckBox *m_showCreatePassword; ///< Показать пароль (создание)
    
    // Вкладка "Присоединиться"
    QLineEdit *m_idEdit;            ///< Поле ID комнаты
    QLineEdit *m_joinPasswordEdit;  ///< Поле пароля для входа
    QCheckBox *m_showJoinPassword;  ///< Показать пароль (вход)
    
    // Кнопки
    QPushButton *m_okButton;        ///< Кнопка OK
    QPushButton *m_cancelButton;    ///< Кнопка Отмена
    
    // Статус
    QLabel *m_statusLabel;          ///< Метка статуса/ошибки
    
    // Данные
    DialogMode m_mode;              ///< Текущий режим
};

#endif // ROOMDIALOG_H
