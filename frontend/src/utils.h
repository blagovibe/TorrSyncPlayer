/**
 * @file utils.h
 * @brief Общие утилитарные функции для frontend
 * 
 * Содержит функции форматирования данных:
 * - Размер в байтах
 * - Длительность
 * - Скорость передачи
 */

#ifndef UTILS_H
#define UTILS_H

#include <QtCore>
#include <QString>

// ── Shared API Constants ──────────────────────────────────────────
// These are mirrored from the backend. Keep in sync.
namespace APIConstants {
    const int MaxMagnetUriLength = 8192;
    const int MaxTorrentIdLength = 40;
    const int MaxRoomIdLength = 32;
    const int MaxRoomNameLength = 50;
    const int MinRoomNameLength = 2;
}

namespace Utils {

/**
 * @brief Форматирование размера в байтах в читаемый вид
 * @param bytes Размер в байтах
 * @return Строка вида "1.5 GB"
 */
QString formatBytes(qint64 bytes);

/**
 * @brief Форматирование длительности в миллисекундах
 * @param ms Длительность в миллисекундах
 * @return Строка вида "01:30:45" или "01:30:45.123" если есть миллисекунды
 */
QString formatDuration(qint64 ms);

/**
 * @brief Форматирование длительности в секундах
 * @param seconds Длительность в секундах
 * @return Строка вида "01:30:45"
 */
QString formatDurationSeconds(qint64 seconds);

/**
 * @brief Форматирование скорости передачи данных
 * @param bytesPerSecond Скорость в байтах в секунду
 * @return Строка вида "2.5 MB/s"
 */
QString formatSpeed(qint64 bytesPerSecond);

} // namespace Utils

#endif // UTILS_H
