/**
 * @file test_utils.cpp
 * @brief Unit tests for Utils namespace
 *
 * Tests:
 * - formatBytes (0, small, large values)
 * - formatDuration (0, seconds only, minutes+seconds, hours+minutes+seconds)
 * - formatDurationSeconds
 * - formatSpeed
 */

#include <QtTest>
#include "utils.h"

class TestUtils : public QObject
{
    Q_OBJECT

private slots:
    // ── formatBytes ──────────────────────────────────────────────────────
    void testFormatBytesZero();
    void testFormatBytesSmall();
    void testFormatBytesKB();
    void testFormatBytesMB();
    void testFormatBytesGB();
    void testFormatBytesNegative();

    // ── formatDuration ───────────────────────────────────────────────────
    void testFormatDurationZero();
    void testFormatDurationSecondsOnly();
    void testFormatDurationMinutesSeconds();
    void testFormatDurationHoursMinutesSeconds();
    void testFormatDurationWithMilliseconds();
    void testFormatDurationNegative();

    // ── formatDurationSeconds ────────────────────────────────────────────
    void testFormatDurationSecondsZero();
    void testFormatDurationSecondsOnlySec();
    void testFormatDurationSecondsMinutes();
    void testFormatDurationSecondsHours();

    // ── formatSpeed ──────────────────────────────────────────────────────
    void testFormatSpeedZero();
    void testFormatSpeedSmall();
    void testFormatSpeedKB();
    void testFormatSpeedMB();
    void testFormatSpeedGB();
    void testFormatSpeedNegative();
};

// ── formatBytes ──────────────────────────────────────────────────────────

void TestUtils::testFormatBytesZero()
{
    QCOMPARE(Utils::formatBytes(0), QString("0 B"));
}

void TestUtils::testFormatBytesSmall()
{
    QCOMPARE(Utils::formatBytes(1), QString("1 B"));
    QCOMPARE(Utils::formatBytes(512), QString("512 B"));
    QCOMPARE(Utils::formatBytes(1023), QString("1023 B"));
}

void TestUtils::testFormatBytesKB()
{
    QCOMPARE(Utils::formatBytes(1024), QString("1.0 KB"));
    QCOMPARE(Utils::formatBytes(1536), QString("1.5 KB"));
    QCOMPARE(Utils::formatBytes(1024 * 1024 - 1), QString("1023.9 KB"));
}

void TestUtils::testFormatBytesMB()
{
    QCOMPARE(Utils::formatBytes(1024 * 1024), QString("1.0 MB"));
    QCOMPARE(Utils::formatBytes(5 * 1024 * 1024), QString("5.0 MB"));
    QCOMPARE(Utils::formatBytes(1024LL * 1024LL * 1024LL - 1), QString("1023.9 MB"));
}

void TestUtils::testFormatBytesGB()
{
    QCOMPARE(Utils::formatBytes(1024LL * 1024LL * 1024LL), QString("1.00 GB"));
    QCOMPARE(Utils::formatBytes(2LL * 1024LL * 1024LL * 1024LL), QString("2.00 GB"));
    QCOMPARE(Utils::formatBytes(10LL * 1024LL * 1024LL * 1024LL), QString("10.00 GB"));
}

void TestUtils::testFormatBytesNegative()
{
    QCOMPARE(Utils::formatBytes(-1), QString("0 B"));
    QCOMPARE(Utils::formatBytes(-1024), QString("0 B"));
}

// ── formatDuration ───────────────────────────────────────────────────────

void TestUtils::testFormatDurationZero()
{
    QCOMPARE(Utils::formatDuration(0), QString("00:00"));
}

void TestUtils::testFormatDurationSecondsOnly()
{
    QCOMPARE(Utils::formatDuration(0), QString("00:00"));
}

void TestUtils::testFormatDurationMinutesSeconds()
{
    QCOMPARE(Utils::formatDuration(60000), QString("01:00"));
    QCOMPARE(Utils::formatDuration(90000), QString("01:30"));
    QCOMPARE(Utils::formatDuration(3599000), QString("59:59"));
}

void TestUtils::testFormatDurationHoursMinutesSeconds()
{
    QCOMPARE(Utils::formatDuration(3600000), QString("01:00:00"));
    QCOMPARE(Utils::formatDuration(3661000), QString("01:01:01"));
    QCOMPARE(Utils::formatDuration(7384000), QString("02:03:04"));
}

void TestUtils::testFormatDurationWithMilliseconds()
{
    QCOMPARE(Utils::formatDuration(100), QString("00:00.100"));
    QCOMPARE(Utils::formatDuration(1100), QString("00:01.100"));
    QCOMPARE(Utils::formatDuration(61100), QString("01:01.100"));
}

void TestUtils::testFormatDurationNegative()
{
    QCOMPARE(Utils::formatDuration(-1), QString("00:00"));
    QCOMPARE(Utils::formatDuration(-1000), QString("00:00"));
}

void TestUtils::testFormatDurationSecondsZero()
{
    QCOMPARE(Utils::formatDurationSeconds(0), QString("00:00"));
}

void TestUtils::testFormatDurationSecondsOnlySec()
{
    QCOMPARE(Utils::formatDurationSeconds(1), QString("00:01"));
    QCOMPARE(Utils::formatDurationSeconds(30), QString("00:30"));
    QCOMPARE(Utils::formatDurationSeconds(59), QString("00:59"));
}

void TestUtils::testFormatDurationSecondsMinutes()
{
    QCOMPARE(Utils::formatDurationSeconds(60), QString("01:00"));
    QCOMPARE(Utils::formatDurationSeconds(90), QString("01:30"));
    QCOMPARE(Utils::formatDurationSeconds(3599), QString("59:59"));
}

void TestUtils::testFormatDurationSecondsHours()
{
    QCOMPARE(Utils::formatDurationSeconds(3600), QString("01:00:00"));
    QCOMPARE(Utils::formatDurationSeconds(3661), QString("01:01:01"));
    QCOMPARE(Utils::formatDurationSeconds(7384), QString("02:03:04"));
}

// ── formatSpeed ──────────────────────────────────────────────────────────

void TestUtils::testFormatSpeedZero()
{
    QCOMPARE(Utils::formatSpeed(0), QString("0 B/s"));
}

void TestUtils::testFormatSpeedSmall()
{
    QCOMPARE(Utils::formatSpeed(1), QString("1 B/s"));
    QCOMPARE(Utils::formatSpeed(512), QString("512 B/s"));
    QCOMPARE(Utils::formatSpeed(1023), QString("1023 B/s"));
}

void TestUtils::testFormatSpeedKB()
{
    QCOMPARE(Utils::formatSpeed(1024), QString("1.0 KB/s"));
    QCOMPARE(Utils::formatSpeed(1536), QString("1.5 KB/s"));
    QCOMPARE(Utils::formatSpeed(1024 * 1024 - 1), QString("1023.9 KB/s"));
}

void TestUtils::testFormatSpeedMB()
{
    QCOMPARE(Utils::formatSpeed(1024 * 1024), QString("1.0 MB/s"));
    QCOMPARE(Utils::formatSpeed(5 * 1024 * 1024), QString("5.0 MB/s"));
    QCOMPARE(Utils::formatSpeed(1024LL * 1024LL * 1024LL - 1), QString("1023.9 MB/s"));
}

void TestUtils::testFormatSpeedGB()
{
    QCOMPARE(Utils::formatSpeed(1024LL * 1024LL * 1024LL), QString("1.00 GB/s"));
    QCOMPARE(Utils::formatSpeed(2LL * 1024LL * 1024LL * 1024LL), QString("2.00 GB/s"));
}

void TestUtils::testFormatSpeedNegative()
{
    QCOMPARE(Utils::formatSpeed(-1), QString("0 B/s"));
    QCOMPARE(Utils::formatSpeed(-1024), QString("0 B/s"));
}

QTEST_MAIN(TestUtils)
#include "test_utils.moc"
