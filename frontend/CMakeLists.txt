cmake_minimum_required(VERSION 3.16)
project(TorrSyncPlayer VERSION 1.0 LANGUAGES CXX)

# ── Стандарт и автоматизация ──────────────────────────────────────────
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

# ── Поиск зависимостей ────────────────────────────────────────────────
find_package(Qt6 REQUIRED COMPONENTS Core Gui Widgets Network Svg Test)
find_package(PkgConfig REQUIRED)
pkg_check_modules(MPV REQUIRED IMPORTED_TARGET libmpv)

# ── Исходные файлы ────────────────────────────────────────────────────
set(SOURCES
    src/main.cpp
    src/mainwindow.cpp
    src/mpvwidget.cpp
    src/networkmanager.cpp
    src/torrentmodel.cpp
    src/torrentmanager.cpp
    src/roommanager.cpp
    src/roomdialog.cpp
    src/systemtray.cpp
    resources/resources.qrc
)

set(HEADERS
    src/mainwindow.h
    src/mpvwidget.h
    src/networkmanager.h
    src/torrentmodel.h
    src/torrentmanager.h
    src/roommanager.h
    src/roomdialog.h
    src/systemtray.h
)

# ── Цель сборки ───────────────────────────────────────────────────────
add_executable(${PROJECT_NAME} ${SOURCES} ${HEADERS})

# ── Подключение библиотек ─────────────────────────────────────────────
target_link_libraries(${PROJECT_NAME} PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Widgets
    Qt6::Network
    Qt6::Svg
    PkgConfig::MPV
)

# ── Включение WinAPI для корректной работы mpv на Windows ────────────
if(WIN32)
    target_link_libraries(${PROJECT_NAME} PRIVATE
        opengl32
        gdi32
        user32
    )
endif()

# ── Установка RPATH для Linux/macOS ───────────────────────────────────
if(UNIX AND NOT APPLE)
    set_target_properties(${PROJECT_NAME} PROPERTIES
        INSTALL_RPATH "$ORIGIN/../lib"
    )
endif()

# ── Unit-тесты ─────────────────────────────────────────────────────────
option(BUILD_TESTS "Build unit tests" ON)

if(BUILD_TESTS)
    enable_testing()
    
    # Тесты для TorrentModel
    set(TEST_TORRENT_MODEL_SOURCES
        src/test_torrentmodel.cpp
        src/torrentmodel.cpp
        src/torrentmodel.h
    )
    
    add_executable(test_torrentmodel ${TEST_TORRENT_MODEL_SOURCES})
    target_link_libraries(test_torrentmodel PRIVATE
        Qt6::Core
        Qt6::Test
    )
    
    # Автоматическая регистрация moc
    set_target_properties(test_torrentmodel PROPERTIES
        AUTOMOC ON
    )
    
    add_test(NAME TorrentModelTest COMMAND test_torrentmodel)
    
    # Тесты для NetworkManager
    set(TEST_NETWORK_MANAGER_SOURCES
        src/test_networkmanager.cpp
        src/networkmanager.cpp
        src/networkmanager.h
    )
    
    add_executable(test_networkmanager ${TEST_NETWORK_MANAGER_SOURCES})
    target_link_libraries(test_networkmanager PRIVATE
        Qt6::Core
        Qt6::Network
        Qt6::Test
    )
    
    set_target_properties(test_networkmanager PROPERTIES
        AUTOMOC ON
    )
    
    add_test(NAME NetworkManagerTest COMMAND test_networkmanager)
    
    message(STATUS "Unit tests enabled")
endif()

# ── Информация о сборке ───────────────────────────────────────────────
message(STATUS "TorrSyncPlayer ${PROJECT_VERSION}")
message(STATUS "Qt6 найден: ${Qt6Core_VERSION}")
message(STATUS "libmpv найден: ${MPV_VERSION}")
