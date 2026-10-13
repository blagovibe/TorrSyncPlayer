// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

#include "backendargs.h"

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

QStringList build(const QString &port)
{
    // Явно передаём порт, хотя он совпадает с умолчанием бэкенда: значение
    // видно в логах запуска, и «откуда взялся порт» не приходится выяснять.
    //
    // Токен сюда не передаётся. Его генерирует сам бэкенд при старте и
    // печатает в свой лог, а игрок получает его от пользователя — из
    // `--access-token` либо из настроек. Передавать секрет аргументом
    // командной строки означало бы показать его в списке процессов, где его
    // видит любой, кто может запускать `ps`.
    return {QStringLiteral("--port"), port};
}

} // namespace BackendArgs