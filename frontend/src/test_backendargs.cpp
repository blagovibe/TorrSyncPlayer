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
#include <QTcpServer>
#include "backendargs.h"

class TestBackendArgs : public QObject
{
    Q_OBJECT

private slots:
    void buildPassesOnlyKnownFlags();
    void buildPassesPortAndItsValue();
    void buildPassesTheTokenItWasGiven();
    void buildOmitsTokenWhenThereIsNone();
    void buildHasNoStrayPositionalArgument();
    void tokenIsNotEmptyByDefault();
    void generatedTokenIsUsableAsAnArgument();
    void pickedPortIsFree();
    void pickedPortDiffersFromAnOccupiedOne();
    // bug-0066: on Linux and macOS the search list held only *.exe names, so
    // the app started with no backend at all — no torrent client, empty list,
    // no playback. Nothing noticed, because this logic lived in main.cpp, which
    // is in no test target.
    void test_candidatePathsCoverTheNonWindowsBuild();
    void test_candidatePathsKeepTheWindowsNames();
    void test_candidatePathsDoNotEscapeTheAppDirectory();
};

void TestBackendArgs::buildPassesOnlyKnownFlags()
{
    const QStringList args = BackendArgs::build(QStringLiteral("8889"), QStringLiteral("tok"));
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
    const QStringList args = BackendArgs::build(QStringLiteral("9999"), QStringLiteral("tok"));

    QCOMPARE(args.at(0), QStringLiteral("--port"));
    QCOMPARE(args.at(1), QStringLiteral("9999"));
}

// bug-0060: the app starts its own backend and then cannot talk to it.
//
// The backend generates a random token when none is given and prints it to
// stdout. The frontend forwarded stdout to qDebug() and never read the token,
// so with no --access-token on the command line every request to its own
// backend came back 401. The client now owns the secret and hands it over.
void TestBackendArgs::buildPassesTheTokenItWasGiven()
{
    const QString secret = QStringLiteral("a3f1c9e7b2d84f06");
    const QStringList args = BackendArgs::build(QStringLiteral("8889"), secret);

    const int at = args.indexOf(QStringLiteral("--token"));
    QVERIFY2(at >= 0, "встроенный бэкенд запущен без токена: клиент не сможет к нему обратиться");
    QCOMPARE(args.at(at + 1), secret);
}

void TestBackendArgs::buildOmitsTokenWhenThereIsNone()
{
    const QStringList args = BackendArgs::build(QStringLiteral("8889"), QString());

    QVERIFY2(args.indexOf(QStringLiteral("--token")) < 0,
             "пустой токен передавать нельзя: --token \"\" зафиксирует пустой секрет вместо случайного");
}

void TestBackendArgs::buildHasNoStrayPositionalArgument()
{
    // Позиционный аргумент Go тоже трактует как флаг и завершает разбор с
    // ошибкой, поэтому в списке не должно быть ничего, что не является
    // парой «флаг, значение» для флагов со значением.
    const QStringList args = BackendArgs::build(QStringLiteral("8889"), QStringLiteral("tok"));

    for (int i = 0; i < args.size(); i += 2) {
        QVERIFY2(args.at(i).startsWith(QLatin1String("--")),
                 qPrintable(QStringLiteral("позиционный аргумент: %1").arg(args.at(i))));
        QVERIFY2(i + 1 < args.size(),
                 qPrintable(QStringLiteral("флаг без значения: %1").arg(args.at(i))));
    }
}

void TestBackendArgs::tokenIsNotEmptyByDefault()
{
    // Пустой токен означал бы «бэкенд сгенерирует случайный», то есть ровно
    // то состояние, из-за которого клиент не мог обратиться к своему
    // бэкенду. Значение по умолчанию обязано быть пригодным секретом.
    QVERIFY2(!BackendArgs::generateToken().isEmpty(),
             "токен по умолчанию пуст — клиент снова останется без доступа к своему бэкенду");
}

void TestBackendArgs::generatedTokenIsUsableAsAnArgument()
{
    const QString secret = BackendArgs::generateToken();
    const QStringList args = BackendArgs::build(QStringLiteral("8889"), secret);

    QVERIFY(secret.contains(QRegularExpression(QStringLiteral("^[0-9a-f]{32,}$"))));
    QVERIFY(args.contains(secret));
}

void TestBackendArgs::pickedPortIsFree()
{
    const quint16 port = BackendArgs::pickFreePort();

    QVERIFY2(port != 0, "не удалось подобрать свободный порт");

    // Free means bindable right now: the same check the picker itself makes.
    QTcpServer probe;
    QVERIFY2(probe.listen(QHostAddress::Any, port),
             qPrintable(QStringLiteral("выбранный порт %1 уже занят").arg(port)));
}

void TestBackendArgs::pickedPortDiffersFromAnOccupiedOne()
{
    // Every instance starts its own backend. With a fixed port the second
    // instance on the same machine fails to bind and the app comes up talking
    // to somebody else's server.
    QTcpServer occupied;
    QVERIFY(occupied.listen(QHostAddress::Any, 0));
    const quint16 busy = occupied.serverPort();

    const quint16 picked = BackendArgs::pickFreePort();

    QVERIFY2(picked != busy,
             qPrintable(QStringLiteral("подобран занятый порт %1").arg(picked)));
}

void TestBackendArgs::test_candidatePathsCoverTheNonWindowsBuild()
{
    const QStringList paths = BackendArgs::candidateBackendPaths(QStringLiteral("/opt/app"));

    bool foundBare = false;
    for (const QString &p : paths) {
        if (p.endsWith(QStringLiteral("/torrsyncplayer"))) {
            foundBare = true;
        }
    }
    QVERIFY2(foundBare,
             "в списке нет имени без расширения: на Linux и macOS бэкенд не будет найден");
}

void TestBackendArgs::test_candidatePathsKeepTheWindowsNames()
{
    const QStringList paths = BackendArgs::candidateBackendPaths(QStringLiteral("C:/app"));

    bool foundExe = false;
    for (const QString &p : paths) {
        if (p.endsWith(QStringLiteral(".exe"))) {
            foundExe = true;
        }
    }
    QVERIFY(foundExe);
}

void TestBackendArgs::test_candidatePathsDoNotEscapeTheAppDirectory()
{
    // «..» в пути раньше отбрасывался целиком: путь отбрасывался, а не
    // очищался. Разыскивание backend/build рядом с приложением нужно в
    // разработческой сборке, поэтому запрещается не вся строка, а выход за
    // пределы каталога приложения.
    for (const QString &p : BackendArgs::candidateBackendPaths(QStringLiteral("/opt/app"))) {
        QVERIFY2(!p.contains(QStringLiteral("/../..")),
                 qPrintable(QStringLiteral("путь уходит выше каталога приложения: %1").arg(p)));
    }
}

QTEST_MAIN(TestBackendArgs)
#include "test_backendargs.moc"