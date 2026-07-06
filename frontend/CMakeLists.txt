cmake_minimum_required(VERSION 3.20)
project(TorrSyncPlayer VERSION 1.0 LANGUAGES CXX)

# ── Опция статической сборки ───────────────────────────────────────────
option(STATIC_BUILD "Build static executable" OFF)

# ── Стандарт и автоматизация ──────────────────────────────────────────
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)
set(CMAKE_AUTOUIC ON)

# ── Флаги безопасности (hardening) ────────────────────────────────────
if(MSVC)
    add_compile_options(/guard:cf /DYNAMICBASE /NXCOMPAT)
    add_link_options(/GUARD:CF /DYNAMICBASE /NXCOMPAT /HIGHENTROPYVA)
    add_compile_options($<$<CONFIG:Debug>:/W4>)
    if(DEFINED ENV{CI})
        add_compile_options(/WX)
    endif()
else()
    add_compile_options(-fstack-protector-strong -D_FORTIFY_SOURCE=2)
    if(NOT WIN32 AND NOT APPLE)
        add_link_options(-Wl,-z,relro,-z,now)
    endif()
endif()

# ── Статическая сборка ─────────────────────────────────────────────────
if(STATIC_BUILD)
    set(BUILD_SHARED_LIBS OFF)

    if(WIN32)
        set(CMAKE_FIND_LIBRARY_SUFFIXES ".a" ".lib")
        set(Qt6_STATIC ON)
    endif()
endif()

# ── Поиск зависимостей ────────────────────────────────────────────────
set(QT_REQUIRED_COMPONENTS Core Gui Widgets Network Svg OpenGL OpenGLWidgets)
if(BUILD_TESTS AND NOT STATIC_BUILD)
    list(APPEND QT_REQUIRED_COMPONENTS Test)
endif()
find_package(Qt6 REQUIRED COMPONENTS ${QT_REQUIRED_COMPONENTS})
find_package(PkgConfig)
if(PkgConfig_FOUND)
    pkg_check_modules(MPV QUIET mpv)
endif()

# ── Исходные файлы ────────────────────────────────────────────────────
set(SOURCES
    src/main.cpp
    src/mainwindow.cpp
    src/mpvwidget.cpp
    src/networkmanager.cpp
    src/roomdialog.cpp
    src/roommanager.cpp
    src/torrentmodel.cpp
    src/torrentmanager.cpp
    src/systemtray.cpp
    src/utils.cpp
    resources/resources.qrc
)

# ── Встраивание backend exe ────────────────────────────────────────────
option(EMBED_BACKEND "Embed torrsyncplayer.exe as Qt resource" OFF)
set(EMBED_BACKEND_PATH "" CACHE PATH "Path to torrsyncplayer.exe for embedding")

if(EMBED_BACKEND AND EMBED_BACKEND_PATH)
    set(EMBED_QRC "${CMAKE_CURRENT_BINARY_DIR}/embedded.qrc")
    file(WRITE "${EMBED_QRC}" "<!DOCTYPE RCC>\n<RCC version=\"1.0\">\n    <qresource prefix=\"/embedded\">\n        <file alias=\"torrsyncplayer.exe\">${EMBED_BACKEND_PATH}</file>\n    </qresource>\n</RCC>\n")
    list(APPEND SOURCES "${EMBED_QRC}")
    add_compile_definitions(HAS_EMBEDDED_BACKEND)

    set(EMBED_BACKEND_SHA256 "" CACHE STRING "SHA-256 hash of embedded backend binary for integrity verification (hex)")
    if(EMBED_BACKEND_SHA256)
        add_compile_definitions(EMBEDDED_BACKEND_SHA256="${EMBED_BACKEND_SHA256}")
        message(STATUS "Embedding backend with integrity verification: ${EMBED_BACKEND_PATH}")
    else()
        message(STATUS "Embedding backend (no integrity hash): ${EMBED_BACKEND_PATH}")
        message(WARNING "Set EMBED_BACKEND_SHA256 to enable binary integrity verification")
    endif()
endif()

set(HEADERS
    src/mainwindow.h
    src/mpvwidget.h
    src/inetworkmanager.h
    src/networkmanager.h
    src/roomdialog.h
    src/roommanager.h
    src/torrentmodel.h
    src/torrentmanager.h
    src/systemtray.h
    src/utils.h
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
    Qt6::OpenGL
    Qt6::OpenGLWidgets
)

if(MPV_FOUND)
    target_include_directories(${PROJECT_NAME} PRIVATE ${MPV_INCLUDE_DIRS})
    target_link_libraries(${PROJECT_NAME} PRIVATE ${MPV_LIBRARIES})
    target_compile_definitions(${PROJECT_NAME} PRIVATE HAS_MPV HAS_MPV_RENDER)
    message(STATUS "libmpv found: enabling OpenGL rendering support")
else()
    message(WARNING "libmpv not found, building without mpv support")
endif()

# ── Платформенные зависимости ──────────────────────────────────────────
if(WIN32)
    target_link_libraries(${PROJECT_NAME} PRIVATE
        opengl32
        gdi32
        user32
    )

    if(STATIC_BUILD)
        target_link_libraries(${PROJECT_NAME} PRIVATE
            ws2_32
            winmm
            imm32
            version
            setupapi
            cfgmgr32
            ole32
            oleaut32
            uuid
            advapi32
            shell32
            d3d11
            dxgi
            dxguid
        )

        if(CMAKE_CXX_COMPILER_ID STREQUAL "GNU")
            target_link_options(${PROJECT_NAME} PRIVATE
                -static
                -static-libgcc
                -static-libstdc++
            )
        elseif(MSVC)
            set_target_properties(${PROJECT_NAME} PROPERTIES
                MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>"
            )
        endif()
    endif()
endif()

# ── Установка RPATH для Linux/macOS ───────────────────────────────────
if(UNIX AND NOT APPLE)
    set_target_properties(${PROJECT_NAME} PROPERTIES
        INSTALL_RPATH "$ORIGIN/../lib"
    )
endif()

# ── Coverage ────────────────────────────────────────────────────────────
option(ENABLE_COVERAGE "Enable code coverage flags" OFF)

if(ENABLE_COVERAGE)
    target_compile_options(${PROJECT_NAME} PRIVATE --coverage)
    target_link_options(${PROJECT_NAME} PRIVATE --coverage)
endif()

# ── Unit-тесты ─────────────────────────────────────────────────────────
option(BUILD_TESTS "Build unit tests" ON)

if(BUILD_TESTS AND NOT STATIC_BUILD)
    enable_testing()
    
    # Тесты для TorrentModel
    set(TEST_TORRENT_MODEL_SOURCES
        src/test_torrentmodel.cpp
        src/torrentmodel.cpp
        src/torrentmodel.h
        src/utils.cpp
    )
    
    add_executable(test_torrentmodel ${TEST_TORRENT_MODEL_SOURCES})
    target_link_libraries(test_torrentmodel PRIVATE
        Qt6::Core
        Qt6::Test
    )
    
    if(ENABLE_COVERAGE)
        target_compile_options(test_torrentmodel PRIVATE --coverage)
        target_link_options(test_torrentmodel PRIVATE --coverage)
    endif()
    
    set_target_properties(test_torrentmodel PROPERTIES
        AUTOMOC ON
    )

    target_include_directories(test_torrentmodel PRIVATE
        ${CMAKE_CURRENT_BINARY_DIR}
        ${CMAKE_CURRENT_SOURCE_DIR}/src
    )

    add_test(NAME TorrentModelTest COMMAND test_torrentmodel)

    # Тесты для NetworkManager
    set(TEST_NETWORK_MANAGER_SOURCES
        src/test_networkmanager.cpp
        src/networkmanager.cpp
        src/networkmanager.h
        src/inetworkmanager.h
    )

    add_executable(test_networkmanager ${TEST_NETWORK_MANAGER_SOURCES})
    target_link_libraries(test_networkmanager PRIVATE
        Qt6::Core
        Qt6::Network
        Qt6::Test
    )

    if(ENABLE_COVERAGE)
        target_compile_options(test_networkmanager PRIVATE --coverage)
        target_link_options(test_networkmanager PRIVATE --coverage)
    endif()

    set_target_properties(test_networkmanager PROPERTIES
        AUTOMOC ON
    )

    target_include_directories(test_networkmanager PRIVATE
        ${CMAKE_CURRENT_BINARY_DIR}
        ${CMAKE_CURRENT_SOURCE_DIR}/src
    )

    add_test(NAME NetworkManagerTest COMMAND test_networkmanager)

    # Тесты для Utils
    set(TEST_UTILS_SOURCES
        src/test_utils.cpp
        src/utils.cpp
        src/utils.h
    )

    add_executable(test_utils ${TEST_UTILS_SOURCES})
    target_link_libraries(test_utils PRIVATE
        Qt6::Core
        Qt6::Test
    )

    if(ENABLE_COVERAGE)
        target_compile_options(test_utils PRIVATE --coverage)
        target_link_options(test_utils PRIVATE --coverage)
    endif()

    set_target_properties(test_utils PROPERTIES
        AUTOMOC ON
    )

    target_include_directories(test_utils PRIVATE
        ${CMAKE_CURRENT_BINARY_DIR}
        ${CMAKE_CURRENT_SOURCE_DIR}/src
    )

    add_test(NAME UtilsTest COMMAND test_utils)

    # Тесты для MpvWidget (требуют libmpv)
    if(MPV_FOUND)
        set(TEST_MPVWIDGET_SOURCES
            src/test_mpvwidget.cpp
            src/mpvwidget.cpp
            src/mpvwidget.h
        )

        add_executable(test_mpvwidget ${TEST_MPVWIDGET_SOURCES})
        target_link_libraries(test_mpvwidget PRIVATE
            Qt6::Core
            Qt6::Gui
            Qt6::OpenGL
            Qt6::OpenGLWidgets
            Qt6::Test
        )

        if(ENABLE_COVERAGE)
            target_compile_options(test_mpvwidget PRIVATE --coverage)
            target_link_options(test_mpvwidget PRIVATE --coverage)
        endif()

        set_target_properties(test_mpvwidget PROPERTIES
            AUTOMOC ON
        )

        target_include_directories(test_mpvwidget PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
        )

        add_test(NAME MpvWidgetTest COMMAND test_mpvwidget)
    endif()

    message(STATUS "Unit tests enabled")
endif()

# ── Информация о сборке ───────────────────────────────────────────────
message(STATUS "TorrSyncPlayer ${PROJECT_VERSION}")
message(STATUS "Qt6 найден: ${Qt6Core_VERSION}")
message(STATUS "libmpv найден: ${MPV_VERSION}")

# ── Установка ──────────────────────────────────────────────────────────
install(TARGETS ${PROJECT_NAME} RUNTIME DESTINATION bin)