@echo off
REM ── Скрипт сборки TorrPlayer для Windows ─────────────────────────────
REM Требования: Qt6, libmpv, CMake 3.16+, MinGW или MSVC

setlocal enabledelayedexpansion

REM ── Параметры по умолчанию ──────────────────────────────────────────
set BUILD_TYPE=Release
set BUILD_DIR=build
set GENERATOR="MinGW Makefiles"

REM ── Определение генератора ──────────────────────────────────────────
where cl >nul 2>nul
if %errorlevel% == 0 (
    set GENERATOR="Visual Studio 17 2022"
    echo [INFO] Обнаружен MSVC, используется Visual Studio генератор
) else (
    where g++ >nul 2>nul
    if %errorlevel% == 0 (
        set GENERATOR="MinGW Makefiles"
        echo [INFO] Обнаружен MinGW, используется MinGW генератор
    ) else (
        echo [ERROR] Не найден компилятор C++ (MSVC или MinGW)
        exit /b 1
    )
)

REM ── Проверка CMake ──────────────────────────────────────────────────
where cmake >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] CMake не найден. Установите CMake.
    exit /b 1
)

REM ── Проверка Qt6 ────────────────────────────────────────────────────
REM Приоритет поиска:
REM   1. Переменная окружения Qt6_DIR
REM   2. Переменная окружения QT_DIR
REM   3. Автоматический поиск в типичных местах
if defined Qt6_DIR (
    echo [INFO] Используется Qt6_DIR из переменной окружения: !Qt6_DIR!
) else if defined QT_DIR (
    if exist "!QT_DIR!\lib\cmake\Qt6" (
        set "Qt6_DIR=!QT_DIR!\lib\cmake\Qt6"
        echo [INFO] Используется QT_DIR из переменной окружения: !QT_DIR!
    ) else if exist "!QT_DIR!\msvc2019_64\lib\cmake\Qt6" (
        set "Qt6_DIR=!QT_DIR!\msvc2019_64\lib\cmake\Qt6"
        echo [INFO] Используется QT_DIR из переменной окружения: !QT_DIR!
    ) else (
        echo [WARN] QT_DIR установлен, но Qt6 не найден в ожидаемых подкаталогах
    )
)

REM Автоматический поиск если Qt6_DIR ещё не установлен
if not defined Qt6_DIR (
    echo [INFO] Поиск Qt6 в типичных местах установки...

    REM Проверяем различные версии Qt и компиляторы
    for %%V in (6.7.0 6.6.0 6.5.3 6.5.2 6.5.1 6.5.0 6.4.0 6.3.0 6.2.0) do (
        for %%C in (msvc2019_64 msvc2022_64 mingw_64) do (
            if exist "C:\Qt\%%V\%%C\lib\cmake\Qt6" (
                set "Qt6_DIR=C:\Qt\%%V\%%C\lib\cmake\Qt6"
                echo [INFO] Найден Qt6: !Qt6_DIR!
                goto :qt_found
            )
        )
    )

    REM Проверяем путь из переменной PATH
    where qmake >nul 2>nul
    if !errorlevel! == 0 (
        for /f "delims=" %%i in ('qmake -query QT_INSTALL_PREFIX') do (
            if exist "%%i\lib\cmake\Qt6" (
                set "Qt6_DIR=%%i\lib\cmake\Qt6"
                echo [INFO] Найден Qt6 через qmake: !Qt6_DIR!
                goto :qt_found
            )
        )
    )

    echo [ERROR] Qt6 не найден!
    echo [ERROR] Установите Qt6 или задайте переменную окружения Qt6_DIR:
    echo [ERROR]   set Qt6_DIR=C:\путь\к\Qt\версия\компилятор\lib\cmake\Qt6
    echo [ERROR] Или используйте QT_DIR:
    echo [ERROR]   set QT_DIR=C:\путь\к\Qt\версия\компилятор
    exit /b 1
)

:qt_found

REM ── Создание директории сборки ──────────────────────────────────────
if exist %BUILD_DIR% (
    echo [INFO] Очистка директории сборки...
    rmdir /s /q %BUILD_DIR%
)

mkdir %BUILD_DIR%
cd %BUILD_DIR%

REM ── Конфигурация CMake ──────────────────────────────────────────────
echo [INFO] Конфигурация CMake...
cmake .. -G %GENERATOR% -DCMAKE_BUILD_TYPE=%BUILD_TYPE%

if %errorlevel% neq 0 (
    echo [ERROR] Ошибка конфигурации CMake
    cd ..
    exit /b 1
)

REM ── Сборка ──────────────────────────────────────────────────────────
echo [INFO] Сборка...
cmake --build . --config %BUILD_TYPE% --parallel

if %errorlevel% neq 0 (
    echo [ERROR] Ошибка сборки
    cd ..
    exit /b 1
)

REM ── Результат ───────────────────────────────────────────────────────
if exist "%BUILD_TYPE%\TorrPlayer.exe" (
    echo [INFO] Сборка завершена успешно!
    echo [INFO] Исполняемый файл: %CD%\%BUILD_TYPE%\TorrPlayer.exe

    REM Копирование ресурсов
    if exist "..\resources" (
        echo [INFO] Копирование ресурсов...
        xcopy /e /i /y "..\resources" "%BUILD_TYPE%\resources"
    )

    REM Копирование DLL Qt (windeployqt)
    where windeployqt >nul 2>nul
    if %errorlevel% == 0 (
        echo [INFO] Запуск windeployqt...
        windeployqt "%BUILD_TYPE%\TorrPlayer.exe"
    )
) else if exist "TorrPlayer.exe" (
    echo [INFO] Сборка завершена успешно!
    echo [INFO] Исполняемый файл: %CD%\TorrPlayer.exe

    REM Копирование ресурсов
    if exist "..\resources" (
        echo [INFO] Копирование ресурсов...
        xcopy /e /i /y "..\resources" "resources"
    )
) else (
    echo [ERROR] Исполняемый файл не найден
    cd ..
    exit /b 1
)

cd ..
echo [INFO] Готово!

REM ── Запуск (опционально) ───────────────────────────────────────────
if "%1"=="run" (
    echo [INFO] Запуск TorrPlayer...
    if exist "%BUILD_DIR%\%BUILD_TYPE%\TorrPlayer.exe" (
        "%BUILD_DIR%\%BUILD_TYPE%\TorrPlayer.exe"
    ) else (
        "%BUILD_DIR%\TorrPlayer.exe"
    )
)

endlocal
