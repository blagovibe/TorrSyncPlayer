/**
 * @file imediaplayer.h
 * @brief Абстрактный интерфейс для MpvWidget / MediaPlayer
 * 
 * Позволяет подменять реализацию для тестирования через gmock.
 */

#ifndef IMEDIAPLAYER_H
#define IMEDIAPLAYER_H

#include <QObject>
#include <QWidget>

/**
 * @class IMediaPlayer
 * @brief Абстрактный интерфейс для медиаплеера
 * 
 * Наследуется от QObject (через QWidget) для поддержки сигналов/слотов.
 * Реализации должны обеспечивать базовое управление воспроизведением.
 */
class IMediaPlayer : public QObject
{
    Q_OBJECT

public:
    explicit IMediaPlayer(QObject *parent = nullptr) : QObject(parent) {}
    virtual ~IMediaPlayer() = default;

    /**
     * @brief Начать воспроизведение URL
     * @param url Адрес медиа (http://, file:// и т.д.)
     */
    virtual void play(const QString &url) = 0;

    /**
     * @brief Поставить на паузу
     * Приостанавливает воспроизведение без сброса позиции
     */
    virtual void pause() = 0;

    /**
     * @brief Возобновить воспроизведение
     * Продолжает воспроизведение с текущей позиции
     */
    virtual void resume() = 0;

    /**
     * @brief Перемотать на позицию
     * @param position Позиция в секундах
     */
    virtual void seek(double position) = 0;

    /**
     * @brief Получить текущую позицию воспроизведения
     * @return Позиция в секундах
     */
    virtual double position() const = 0;

    /**
     * @brief Получить длительность медиа
     * @return Длительность в секундах
     */
    virtual double duration() const = 0;

    /**
     * @brief Проверить, стоит ли воспроизведение на паузе
     * @return true если на паузе
     */
    virtual bool isPaused() const = 0;

    /**
     * @brief Проверить, инициализирован ли плеер
     * @return true если плеер готов к работе
     */
    virtual bool isInitialized() const = 0;

signals:
    /**
     * @brief Сигнал изменения позиции воспроизведения
     * Испускается при изменении текущей позиции
     * @param position Новая позиция в секундах
     */
    void positionChanged(double position);

    /**
     * @brief Сигнал изменения длительности медиа
     * Испускается при получении информации о длительности
     * @param duration Новая длительность в секундах
     */
    void durationChanged(double duration);

    /**
     * @brief Сигнал завершения воспроизведения
     * Испускается когда видео доходит до конца
     */
    void playbackFinished();

    /**
     * @brief Сигнал ошибки
     * Испускается при возникновении ошибки воспроизведения
     * @param message Описание ошибки
     */
    void error(const QString &message);

    /**
     * @brief Сигнал готовности плеера к воспроизведению
     * Испускается после успешной инициализации
     */
    void ready();
};

#endif // IMEDIAPLAYER_H