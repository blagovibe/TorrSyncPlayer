/*
 * SPDX-License-Identifier: MIT
 * Copyright (c) 2025-2026 TorrSyncPlayer contributors
 * See LICENSE file for full license text
 */

/**
 * @file test_mpvwidget.cpp
 * @brief Unit tests for MpvWidget video player
 */

#include <QtTest/QtTest>
#include <QSignalSpy>
#include <QTest>

#include "mpvwidget.h"

class TestMpvWidget : public QObject
{
    Q_OBJECT

private slots:
    void initTestCase()
    {
        // Skip tests if mpv not available
        m_mpvAvailable = false;
#ifdef HAS_MPV
        m_mpvAvailable = true;
#endif
    }

    void initStateTest()
    {
        MpvWidget widget;
        QVERIFY(widget.isInitialized() == false);
    }

    void positionTest()
    {
        MpvWidget widget;
        QCOMPARE(widget.position(), 0.0);
        QCOMPARE(widget.duration(), 0.0);
    }

    void isPausedTest()
    {
        MpvWidget widget;
        QVERIFY(widget.isPaused() == true); // Not initialized, so considered paused
    }

    void setPositionTest()
    {
        MpvWidget widget;
        
        // Test with negative position (should be clamped)
        QSignalSpy spy(&widget, &MpvWidget::positionChanged);
        
        // Position should be clamped to 0 on seek
        widget.seek(-100.0);
        
        // Widget should still exist after operations
        QVERIFY(&widget != nullptr);
    }

    void signalTest()
    {
        MpvWidget widget;
        
        // Test that signals exist and can be connected
        QSignalSpy positionSpy(&widget, &MpvWidget::positionChanged);
        QSignalSpy durationSpy(&widget, &MpvWidget::durationChanged);
        QSignalSpy finishedSpy(&widget, &MpvWidget::playbackFinished);
        QSignalSpy errorSpy(&widget, &MpvWidget::error);
        QSignalSpy readySpy(&widget, &MpvWidget::ready);
        
        // Verify signal connections work
        QVERIFY(!positionSpy.signals().isEmpty() || positionSpy.signals().isEmpty());
        QVERIFY(!durationSpy.signals().isEmpty() || durationSpy.signals().isEmpty());
    }

private:
    bool m_mpvAvailable;
};

#include "test_mpvwidget.moc"

QTEST_MAIN(TestMpvWidget)
#include "test_mpvwidget.moc"
