/**
 * @file roomdialog.cpp
 * @brief Реализация диалога создания/присоединения к комнате
 */

#include "roomdialog.h"

#include <QDebug>
#include <QMessageBox>
#include <QRegularExpression>

RoomDialog::RoomDialog(DialogMode mode, QWidget *parent)
    : QDialog(parent)
    , m_mode(mode)
{
    setWindowTitle(tr("Управление комнатой"));
    setMinimumWidth(400);
    setModal(true);
    
    // ── Основной layout ───────────────────────────────────────────────
    QVBoxLayout *mainLayout = new QVBoxLayout(this);
    
    // ── Вкладки ───────────────────────────────────────────────────────
    m_tabWidget = new QTabWidget(this);
    m_tabWidget->addTab(createCreateTab(), tr("Создать комнату"));
    m_tabWidget->addTab(createJoinTab(), tr("Присоединиться"));
    
    // Устанавливаем начальную вкладку в зависимости от режима
    m_tabWidget->setCurrentIndex(mode == CreateMode ? 0 : 1);
    
    mainLayout->addWidget(m_tabWidget);
    
    // ── Метка статуса ─────────────────────────────────────────────────
    m_statusLabel = new QLabel(this);
    m_statusLabel->setStyleSheet("color: red;");
    m_statusLabel->setWordWrap(true);
    m_statusLabel->hide();
    mainLayout->addWidget(m_statusLabel);
    
    // ── Кнопки ────────────────────────────────────────────────────────
    QHBoxLayout *buttonsLayout = new QHBoxLayout();
    buttonsLayout->addStretch();
    
    m_okButton = new QPushButton(mode == CreateMode ? tr("Создать") : tr("Войти"), this);
    m_okButton->setDefault(true);
    m_okButton->setMinimumWidth(100);
    buttonsLayout->addWidget(m_okButton);
    
    m_cancelButton = new QPushButton(tr("Отмена"), this);
    m_cancelButton->setMinimumWidth(100);
    buttonsLayout->addWidget(m_cancelButton);
    
    mainLayout->addLayout(buttonsLayout);
    
    // ── Подключения ───────────────────────────────────────────────────
    connect(m_okButton, &QPushButton::clicked, this, &RoomDialog::onAccept);
    connect(m_cancelButton, &QPushButton::clicked, this, &QDialog::reject);
    connect(m_tabWidget, &QTabWidget::currentChanged, this, &RoomDialog::onTabChanged);
    
    // Фокус на первое поле
    if (mode == CreateMode) {
        m_nameEdit->setFocus();
    } else {
        m_idEdit->setFocus();
    }
    
    qDebug() << "RoomDialog: создан в режиме" << (mode == CreateMode ? "Create" : "Join");
}

RoomDialog::~RoomDialog()
{
    qDebug() << "RoomDialog: уничтожен";
}

QWidget* RoomDialog::createCreateTab()
{
    QWidget *tab = new QWidget(this);
    QFormLayout *layout = new QFormLayout(tab);
    layout->setContentsMargins(10, 10, 10, 10);
    layout->setSpacing(10);
    
    // ── Имя комнаты ───────────────────────────────────────────────────
    QLabel *nameLabel = new QLabel(tr("Имя комнаты:"), tab);
    m_nameEdit = new QLineEdit(tab);
    m_nameEdit->setPlaceholderText(tr("Введите имя комнаты..."));
    m_nameEdit->setMaxLength(50);
    m_nameEdit->setClearButtonEnabled(true);
    layout->addRow(nameLabel, m_nameEdit);
    
    // ── Подсказка ─────────────────────────────────────────────────────
    QLabel *hintLabel = new QLabel(tr("Имя будет отображаться для других участников"), tab);
    hintLabel->setStyleSheet("color: gray; font-size: 11px;");
    layout->addRow("", hintLabel);
    
    // ── Пароль (опционально) ──────────────────────────────────────────
    QLabel *passwordLabel = new QLabel(tr("Пароль:"), tab);
    QHBoxLayout *passwordLayout = new QHBoxLayout();
    
    m_createPasswordEdit = new QLineEdit(tab);
    m_createPasswordEdit->setPlaceholderText(tr("Необязательно"));
    m_createPasswordEdit->setEchoMode(QLineEdit::Password);
    m_createPasswordEdit->setMaxLength(100);
    m_createPasswordEdit->setClearButtonEnabled(true);
    passwordLayout->addWidget(m_createPasswordEdit);
    
    m_showCreatePassword = new QCheckBox(tr("Показать"), tab);
    passwordLayout->addWidget(m_showCreatePassword);
    
    layout->addRow(passwordLabel, passwordLayout);
    
    // ── Подключение показа пароля ─────────────────────────────────────
    connect(m_showCreatePassword, &QCheckBox::toggled, this, [this](bool checked) {
        m_createPasswordEdit->setEchoMode(checked ? QLineEdit::Normal : QLineEdit::Password);
    });
    
    // ── Информация ────────────────────────────────────────────────────
    QLabel *infoLabel = new QLabel(tr("После создания комнаты вы станете хостом и "
                                     "сможете управлять воспроизведением"), tab);
    infoLabel->setWordWrap(true);
    infoLabel->setStyleSheet("color: #666; font-size: 11px; margin-top: 10px;");
    layout->addRow(infoLabel);
    
    return tab;
}

QWidget* RoomDialog::createJoinTab()
{
    QWidget *tab = new QWidget(this);
    QFormLayout *layout = new QFormLayout(tab);
    layout->setContentsMargins(10, 10, 10, 10);
    layout->setSpacing(10);
    
    // ── ID комнаты ────────────────────────────────────────────────────
    QLabel *idLabel = new QLabel(tr("ID комнаты:"), tab);
    m_idEdit = new QLineEdit(tab);
    m_idEdit->setPlaceholderText(tr("Введите ID комнаты..."));
    m_idEdit->setMaxLength(100);
    m_idEdit->setClearButtonEnabled(true);
    layout->addRow(idLabel, m_idEdit);
    
    // ── Подсказка ─────────────────────────────────────────────────────
    QLabel *hintLabel = new QLabel(tr("ID комнаты можно получить у хоста"), tab);
    hintLabel->setStyleSheet("color: gray; font-size: 11px;");
    layout->addRow("", hintLabel);
    
    // ── Пароль (если требуется) ───────────────────────────────────────
    QLabel *passwordLabel = new QLabel(tr("Пароль:"), tab);
    QHBoxLayout *passwordLayout = new QHBoxLayout();
    
    m_joinPasswordEdit = new QLineEdit(tab);
    m_joinPasswordEdit->setPlaceholderText(tr("Если требуется"));
    m_joinPasswordEdit->setEchoMode(QLineEdit::Password);
    m_joinPasswordEdit->setMaxLength(100);
    m_joinPasswordEdit->setClearButtonEnabled(true);
    passwordLayout->addWidget(m_joinPasswordEdit);
    
    m_showJoinPassword = new QCheckBox(tr("Показать"), tab);
    passwordLayout->addWidget(m_showJoinPassword);
    
    layout->addRow(passwordLabel, passwordLayout);
    
    // ── Подключение показа пароля ─────────────────────────────────────
    connect(m_showJoinPassword, &QCheckBox::toggled, this, [this](bool checked) {
        m_joinPasswordEdit->setEchoMode(checked ? QLineEdit::Normal : QLineEdit::Password);
    });
    
    // ── Информация ────────────────────────────────────────────────────
    QLabel *infoLabel = new QLabel(tr("Присоединившись к комнате, вы будете "
                                     "синхронизированы с хостом воспроизведения"), tab);
    infoLabel->setWordWrap(true);
    infoLabel->setStyleSheet("color: #666; font-size: 11px; margin-top: 10px;");
    layout->addRow(infoLabel);
    
    return tab;
}

// ── Геттеры ───────────────────────────────────────────────────────────

QString RoomDialog::roomName() const
{
    return m_nameEdit->text().trimmed();
}

QString RoomDialog::roomId() const
{
    return m_idEdit->text().trimmed();
}

QString RoomDialog::password() const
{
    if (m_tabWidget->currentIndex() == 0) {
        return m_createPasswordEdit->text();
    } else {
        return m_joinPasswordEdit->text();
    }
}

bool RoomDialog::isHost() const
{
    return m_tabWidget->currentIndex() == 0;
}

// ── Слоты ─────────────────────────────────────────────────────────────

void RoomDialog::onAccept()
{
    if (validateInput()) {
        accept();
    }
}

void RoomDialog::onTabChanged(int index)
{
    m_statusLabel->hide();
    
    // Обновляем текст кнопки OK
    m_okButton->setText(index == 0 ? tr("Создать") : tr("Войти"));
    
    // Устанавливаем фокус
    if (index == 0) {
        m_nameEdit->setFocus();
    } else {
        m_idEdit->setFocus();
    }
}

// ── Валидация ─────────────────────────────────────────────────────────

bool RoomDialog::validateInput()
{
    m_statusLabel->hide();
    
    if (m_tabWidget->currentIndex() == 0) {
        // Валидация создания комнаты
        QString name = roomName();
        
        if (name.isEmpty()) {
            showValidationError(tr("Введите имя комнаты"));
            m_nameEdit->setFocus();
            return false;
        }
        
        if (name.length() < 2) {
            showValidationError(tr("Имя комнаты должно содержать минимум 2 символа"));
            m_nameEdit->setFocus();
            return false;
        }
        
        // Проверка на недопустимые символы
        QRegularExpression validName("^[a-zA-Zа-яА-ЯёЁ0-9\\s\\-_]+$");
        if (!validName.match(name).hasMatch()) {
            showValidationError(tr("Имя комнаты содержит недопустимые символы"));
            m_nameEdit->setFocus();
            return false;
        }
        
    } else {
        // Валидация присоединения
        QString id = roomId();
        
        if (id.isEmpty()) {
            showValidationError(tr("Введите ID комнаты"));
            m_idEdit->setFocus();
            return false;
        }
        
        if (id.length() < 4) {
            showValidationError(tr("ID комнаты слишком короткий"));
            m_idEdit->setFocus();
            return false;
        }
    }
    
    return true;
}

void RoomDialog::showValidationError(const QString &message)
{
    m_statusLabel->setText(message);
    m_statusLabel->show();
    
    qDebug() << "RoomDialog: ошибка валидации:" << message;
}
