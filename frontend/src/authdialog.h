/**
 * @file authdialog.h
 * @brief Диалог авторизации и регистрации пользователя
 */

#ifndef AUTHDIALOG_H
#define AUTHDIALOG_H

#include <QDialog>
#include <QLineEdit>
#include <QPushButton>
#include <QLabel>
#include <QComboBox>
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QFormLayout>

/**
 * @class AuthDialog
 * @brief Диалог для входа и регистрации
 *
 * Позволяет пользователю войти (login) или зарегистрироваться (register).
 * При подтверждении возвращает имя пользователя и пароль через геттеры;
 * вызывающая сторона отправляет их в NetworkManager.
 */
class AuthDialog : public QDialog
{
    Q_OBJECT

public:
    enum Mode {
        LoginMode,
        RegisterMode
    };

    explicit AuthDialog(QWidget *parent = nullptr);

    QString username() const;
    QString password() const;
    Mode mode() const;

private slots:
    void onAccept();
    void onModeChanged(int index);

private:
    bool validateInput();
    void showError(const QString &message);

    QComboBox   *m_modeCombo;
    QLineEdit   *m_usernameEdit;
    QLineEdit   *m_passwordEdit;
    QLabel      *m_statusLabel;
    QPushButton *m_okButton;
    QPushButton *m_cancelButton;
};

#endif // AUTHDIALOG_H
