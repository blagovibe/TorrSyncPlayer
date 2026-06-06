/**
 * @file utils.cpp
 * @brief Реализация общих утилитарных функций для frontend
 */

#include "utils.h"

namespace Utils {

QString formatBytes(qint64 bytes)
{
    if (bytes < 0) {
        bytes = 0;
    }
    
    if (bytes < 1024) {
        return QString("%1 B").arg(bytes);
    } else if (bytes < 1024 * 1024) {
        return QString("%1 KB").arg(bytes / 1024.0, 0, 'f', 1);
    } else if (bytes < 1024LL * 1024LL * 1024LL) {
        return QString("%1 MB").arg(bytes / (1024.0 * 1024.0), 0, 'f', 1);
    } else {
        return QString("%1 GB").arg(bytes / (1024.0 * 1024.0 * 1024.0), 0, 'f', 2);
    }
}

// formatFileSize удалён как дубликат formatBytes
// Используйте formatBytes вместо formatFileSize

QString formatDuration(qint64 ms)
{
    if (ms < 0) {
        ms = 0;
    }
    
    qint64 totalSeconds = ms / 1000;
    qint64 milliseconds = ms % 1000;
    qint64 hours = totalSeconds / 3600;
    qint64 minutes = (totalSeconds % 3600) / 60;
    qint64 seconds = totalSeconds % 60;
    
    if (milliseconds > 0) {
        if (hours > 0) {
            return QString("%1:%2:%3.%4")
                .arg(hours, 2, 10, QChar('0'))
                .arg(minutes, 2, 10, QChar('0'))
                .arg(seconds, 2, 10, QChar('0'))
                .arg(milliseconds, 3, 10, QChar('0'));
        }
        return QString("%1:%2.%3")
            .arg(minutes, 2, 10, QChar('0'))
            .arg(seconds, 2, 10, QChar('0'))
            .arg(milliseconds, 3, 10, QChar('0'));
    }

    if (hours > 0) {
        return QString("%1:%2:%3")
            .arg(hours, 2, 10, QChar('0'))
            .arg(minutes, 2, 10, QChar('0'))
            .arg(seconds, 2, 10, QChar('0'));
    }
    return QString("%1:%2")
        .arg(minutes, 2, 10, QChar('0'))
        .arg(seconds, 2, 10, QChar('0'));
}

QString formatDurationSeconds(qint64 seconds)
{
    return formatDuration(seconds * 1000);
}

QString formatSpeed(qint64 bytesPerSecond)
{
    if (bytesPerSecond < 0) {
        bytesPerSecond = 0;
    }
    
    if (bytesPerSecond < 1024) {
        return QString("%1 B/s").arg(bytesPerSecond);
    } else if (bytesPerSecond < 1024 * 1024) {
        return QString("%1 KB/s").arg(bytesPerSecond / 1024.0, 0, 'f', 1);
    } else if (bytesPerSecond < 1024LL * 1024LL * 1024LL) {
        return QString("%1 MB/s").arg(bytesPerSecond / (1024.0 * 1024.0), 0, 'f', 1);
    } else {
        return QString("%1 GB/s").arg(bytesPerSecond / (1024.0 * 1024.0 * 1024.0), 0, 'f', 2);
    }
}

} // namespace Utils
