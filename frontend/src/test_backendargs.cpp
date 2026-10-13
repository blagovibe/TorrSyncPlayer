/**
 * @file test_backendargs.cpp
 * @brief Контракт между фронтендом и флагами Go-бэкенда
 *
 * Существует из-за бага, который прошёл через полностью зелёный пайплайн:
 * фронтенд передавал встроенному бэкенду `--jwt-secret`, флага в Go больше не
 * было (его удалили вместе с JWT-аутентификацией), `flag.Parse` отвечал
 * `flag provided but not defined` и завершал процесс кодом 2. Релизный
 * Windows-бинарь не запускался, и ни один джоб CI этого не замечал, потому
 * что `main.cpp` не входит ни в один тестовый таргет.
 *
 * Набор флагов бэкенда написан здесь руками, и это осознанно: он и есть
 * утверждение. Добавили флаг в Go — добавьте и сюда, иначе сборка теста
 * упадёт. Смысл проверки не в том, что список полон, а в том, что фронтенд
 * не отправит бэкенду то, чего бэкенд не понимает.
 */

#include <QtTest>
#include "backendargs.h"

class TestBackendArgs : public QObject
{
    Q_OBJECT

private slots:
    void buildPassesOnlyKnownFlags();
    void buildPassesPortAndItsValue();
    void buildHasNoSecretArgument();
    void buildHasNoStrayPositionalArgument();
};

void TestBackendArgs::buildPassesOnlyKnownFlags()
{
    const QStringList args = BackendArgs::build(QStringLiteral("8889"));
    const QStringList &known = BackendArgs::knownBackendFlags();

    QStringList unknown;
    for (const QString &arg : args) {
        if (!arg.startsWith(QLatin1String("--"))) {
            continue;
        }
        const QString name = arg.mid(2);
        if (!known.contains(name)) {
            unknown << arg;
        }
    }

    QVERIFY2(unknown.isEmpty(),
             qPrintable(QStringLiteral("бэкенд не понимает: %1").arg(unknown.join(QLatin1String(", ")))));
}

void TestBackendArgs::buildPassesPortAndItsValue()
{
    const QStringList args = BackendArgs::build(QStringLiteral("9999"));

    QCOMPARE(args.size(), 2);
    QCOMPARE(args.at(0), QStringLiteral("--port"));
    QCOMPARE(args.at(1), QStringLiteral("9999"));
}

void TestBackendArgs::buildHasNoSecretArgument()
{
    // Секрет в командной строке виден в списке процессов. Токен генерирует
    // сам бэкенд, а игрок получает его от пользователя, поэтому передавать его
    // аргументом нельзя.
    const QStringList args = BackendArgs::build(QStringLiteral("8889"));

    for (const QString &arg : args) {
        QVERIFY2(!arg.contains(QLatin1String("secret"), Qt::CaseInsensitive),
                 qPrintable(QStringLiteral("аргумент выглядит как секрет: %1").arg(arg)));
        QVERIFY2(!arg.contains(QLatin1String("token"), Qt::CaseInsensitive),
                 qPrintable(QStringLiteral("аргумент выглядит как секрет: %1").arg(arg)));
    }
}

void TestBackendArgs::buildHasNoStrayPositionalArgument()
{
    // Позиционный аргумент Go тоже трактует как флаг и завершает разбор с
    // ошибкой, поэтому в списке не должно быть ничего, что не является
    // парой «флаг, значение» для флагов со значением.
    const QStringList args = BackendArgs::build(QStringLiteral("8889"));

    for (int i = 0; i < args.size(); i += 2) {
        QVERIFY2(args.at(i).startsWith(QLatin1String("--")),
                 qPrintable(QStringLiteral("позиционный аргумент: %1").arg(args.at(i))));
        QVERIFY2(i + 1 < args.size(),
                 qPrintable(QStringLiteral("флаг без значения: %1").arg(args.at(i))));
    }
}

QTEST_MAIN(TestBackendArgs)
#include "test_backendargs.moc"