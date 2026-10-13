// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

#include "backendargs.h"

#include <QHostAddress>
#include <QTcpServer>
#include <QUuid>

namespace BackendArgs {

const QStringList &knownBackendFlags()
{
    // Зеркало parseFlags в backend/cmd/server/main.go.
    //
    // Флаги, которые бэкенд читает из окружения (PORT, DATA_DIR,
    // DISK_STORAGE, TLS_CERT, TLS_KEY, GATEWAY_*), сюда не входят: у них нет
    // отдельного флага, и фронтенду незачем их дублировать — он запускает
    // бэкенд на своей же машине и наследует её окружение.
    static const QStringList flags = {
        QStringLiteral("port"),
        QStringLiteral("addr"),
        QStringLiteral("token"),
        QStringLiteral("data-dir"),
        QStringLiteral("disk-storage"),
        QStringLiteral("tls-cert"),
        QStringLiteral("tls-key"),
        QStringLiteral("tls"),
        QStringLiteral("auto-tls"),
        QStringLiteral("enable-profiling"),
        QStringLiteral("gateway-url"),
        QStringLiteral("gateway-user"),
        QStringLiteral("gateway-pass"),
        QStringLiteral("gateway-ca-cert"),
    };
    return flags;
}

QString generateToken()
{
    // 128 бит в виде 32 hex-символов: длиннее становится неуместно в выводе,
    // короче — уже не ощущается как секрет. Бэкенд принимает любую строку,
    // формат здесь задаёт только клиент.
    //
    // Сравнение в бэкенде — crypto/subtle.ConstantTimeCompare, поэтому
    // длину токена стоит держать постоянной: иначе длина утекала бы через
    // время ответа. QUuid без фигурных скобок даёт 32 hex-символа.
    return QUuid::createUuid().toString(QUuid::WithoutBraces).remove(QLatin1Char('-'));
}

quint16 pickFreePort()
{
    // Порт выбирается тем же способом, каким его потом займёт бэкенд:
    // слушателем на всех интерфейсах. Выбор на loopback дал бы порт, который
    // тут же окажется занят снаружи — и обратно.
    QTcpServer probe;
    if (!probe.listen(QHostAddress::Any, 0)) {
        return 0;
    }
    return probe.serverPort();
}

QStringList build(const QString &port, const QString &token)
{
    // Явно передаём порт, хотя он совпадает с умолчанием бэкенда: значение
    // видно в логах запуска, и «откуда взялся порт» не приходится выяснять.
    //
    // Токен передаётся флагом, а не оставляется бэкенду на генерацию.
    // Пустой бэкенд напечатал бы его в лог, откуда клиент его не забирает, и
    // приложение осталось бы без доступа к собственному бэкенду (bug-0060).
    // Цена решения — секрет виден в списке процессов; она зафиксирована как
    // открытое отклонение, потому что бэкенд слушает и внешние интерфейсы, а
    // не только loopback.
    QStringList args = {
        QStringLiteral("--port"), port,
    };
    if (!token.isEmpty()) {
        args << QStringLiteral("--token") << token;
    }
    return args;
}

} // namespace BackendArgs