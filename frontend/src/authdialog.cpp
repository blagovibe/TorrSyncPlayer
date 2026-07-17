/**
 * @file authdialog.cpp
 * @brief Реализация диалога авторизации и регистрации
 */

#include "authdialog.h"

#include <QDialogButtonBox>
#include <QRegularExpression>

AuthDialog::AuthDialog(QWidget *parent)
    : QDialog(parent)
{
    setWindowTitle(tr("Аккаунт"));
    setMinimumWidth(320);

    auto *form = new QFormLayout();

    m_modeCombo = new QComboBox(this);
    m_modeCombo->addItem(tr("Войти"), LoginMode);
    m_modeCombo->addItem(tr("Регистрация"), RegisterMode);
    form->addRow(tr("Действие:"), m_modeCombo);

    m_usernameEdit = new QLineEdit(this);
    m_usernameEdit->setPlaceholderText(tr("Имя пользователя"));
    form->addRow(tr("Пользователь:"), m_usernameEdit);

    m_passwordEdit = new QLineEdit(this);
    m_passwordEdit->setEchoMode(QLineEdit::Password);
    m_passwordEdit->setPlaceholderText(tr("Пароль"));
    form->addRow(tr("Пароль:"), m_passwordEdit);

    auto *mainLayout = new QVBoxLayout(this);
    mainLayout->addLayout(form);

    m_statusLabel = new QLabel(this);
    m_statusLabel->setWordWrap(true);
    mainLayout->addWidget(m_statusLabel);

    auto *buttonBox = new QDialogButtonBox(
        QDialogButtonBox::Ok | QDialogButtonBox::Cancel, this);
    m_okButton = buttonBox->button(QDialogButtonBox::Ok);
    m_cancelButton = buttonBox->button(QDialogButtonBox::Cancel);
    mainLayout->addWidget(buttonBox);

    connect(buttonBox, &QDialogButtonBox::accepted, this, &AuthDialog::onAccept);
    connect(buttonBox, &QDialogButtonBox::rejected, this, &QDialog::reject);
    connect(m_modeCombo, QOverload<int>::of(&QComboBox::currentIndexChanged),
            this, &AuthDialog::onModeChanged);
}

QString AuthDialog::username() const
{
    return m_usernameEdit->text().trimmed();
}

QString AuthDialog::password() const
{
    return m_passwordEdit->text();
}

AuthDialog::Mode AuthDialog::mode() const
{
    return static_cast<Mode>(m_modeCombo->currentData().toInt());
}

void AuthDialog::onModeChanged(int /*index*/)
{
    m_statusLabel->clear();
}

void AuthDialog::onAccept()
{
    if (!validateInput()) {
        return;
    }
    accept();
}

bool AuthDialog::validateInput()
{
    const QString user = username();
    if (user.isEmpty()) {
        showError(tr("Введите имя пользователя"));
        return false;
    }
    // Username: letters, digits, underscore, hyphen (matches backend validation).
    if (!QRegularExpression("^[a-zA-Z0-9_-]+$").match(user).hasMatch()) {
        showError(tr("Имя пользователя может содержать только буквы, цифры, _ и -"));
        return false;
    }
    if (user.length() < 3) {
        showError(tr("Имя пользователя должно быть не короче 3 символов"));
        return false;
    }
    if (password().isEmpty()) {
        showError(tr("Введите пароль"));
        return false;
    }
    if (password().length() < 8) {
        showError(tr("Пароль должен быть не короче 8 символов"));
        return false;
    }
    m_statusLabel->clear();
    return true;
}

void AuthDialog::showError(const QString &message)
{
    m_statusLabel->setText(message);
}
