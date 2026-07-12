cmake_minimum_required(VERSION 3.20)

# ── Auto-detect version from Git tag or GitHub Actions ───────────────────────
# Priority: CI_TAG > CI_COMMIT_TAG > git describe > default
set(PROJECT_VERSION_FROM_CI "1.1.0") # Default fallback

# Try GitHub Actions tag (refs/tags/v1.1.1 -> 1.1.1)
if(DEFINED ENV{GITHUB_REF_NAME} AND ENV{GITHUB_REF_NAME} MATCHES "^v?[0-9]+\\.[0-9]+\\..+$")
    set(PROJECT_VERSION_FROM_CI "$ENV{GITHUB_REF_NAME}")
elseif(DEFINED ENV{CI_COMMIT_TAG} AND ENV{CI_COMMIT_TAG} MATCHES "^v?[0-9]+\\.[0-9]+\\..+$")
    set(PROJECT_VERSION_FROM_CI "$ENV{CI_COMMIT_TAG}")
endif()

# Extract major.minor for VERSION (1.1.1 -> 1.1)
string(REGEX REPLACE "^v?([0-9]+\\.[0-9]+)\\..*$" "\\1" VERSION_MAJOR_MINOR "${PROJECT_VERSION_FROM_CI}")

project(TorrSyncPlayer VERSION "${VERSION_MAJOR_MINOR}" LANGUAGES CXX)

# Export for C++ preprocessor
add_compile_definitions(APP_VERSION="${PROJECT_VERSION_FROM_CI}")

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

# ── macOS AGL framework handling (removed in macOS 14+) ────────────────────
# AGL framework was removed in macOS 14+. We create stub framework to satisfy linker.
# Qt6::Widgets may transitively add -framework AGL even when OpenGLWidgets is not linked.
if(APPLE)
    # Create AGL.framework structure in build directory
    set(AGL_FW_PATH "${CMAKE_BINARY_DIR}/AGL.framework")
    set(AGL_LIB_PATH "${AGL_FW_PATH}/AGL")
    
    # Create framework directory structure
    file(MAKE_DIRECTORY "${AGL_FW_PATH}/Versions/A/Headers")
    file(MAKE_DIRECTORY "${AGL_FW_PATH}/Versions/A/Resources")
    file(MAKE_DIRECTORY "${AGL_FW_PATH}/Headers")
    file(MAKE_DIRECTORY "${AGL_FW_PATH}/Resources")
    
    # Write minimal Info.plist for framework
    file(WRITE "${AGL_FW_PATH}/Versions/A/Resources/Info.plist"
"<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\">\n<dict>\n    <key>CFBundlePackageType</key>\n    <string>FMWK</string>\n</dict>\n</plist>"
    )
    
    # Create stub source file (minimal C file)
    set(AGL_STUB_SRC "${CMAKE_BINARY_DIR}/agl_stub.c")
    file(WRITE "${AGL_STUB_SRC}" "void aglDummySymbol(){}")
    
    # Try to create dylib using clang from PATH
    find_program(CLANG_EXE "clang")
    if(CLANG_EXE)
        execute_process(
            COMMAND ${CLANG_EXE} -dynamiclib -install_name AGL.framework/AGL
                    -o "${AGL_LIB_PATH}" "${AGL_STUB_SRC}" -lc
            RESULT_VARIABLE AGL_RESULT
            ERROR_VARIABLE AGL_ERROR
            OUTPUT_VARIABLE AGL_OUTPUT
        )
        if(EXISTS "${AGL_LIB_PATH}")
            message(STATUS "Created AGL.framework stub at ${AGL_FW_PATH}")
        else()
            message(STATUS "AGL.framework stub creation failed: ${AGL_ERROR}")
        endif()
    endif()
endif()

# ── Поиск зависимостей ────────────────────────────────────────────────
# Note: OpenGL/OpenGLWidgets cause AGL linker errors on macOS 14+ (AGL removed)
# Use optional OpenGL for macOS, required for other platforms
set(QT_REQUIRED_COMPONENTS Core Gui Widgets Network Svg)
if(BUILD_TESTS AND NOT STATIC_BUILD)
    list(APPEND QT_REQUIRED_COMPONENTS Test)
endif()
if(NOT APPLE)
    list(APPEND QT_REQUIRED_COMPONENTS OpenGL OpenGLWidgets)
endif()
find_package(Qt6 REQUIRED COMPONENTS ${QT_REQUIRED_COMPONENTS})
# Note: On macOS, we don't link OpenGL/OpenGLWidgets at all - using NO_OPENGL stub mode
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
    src/interfaces/inetworkmanager.h
    src/networkmanager.h
    src/roomdialog.h
    src/roommanager.h
    src/torrentmodel.h
    src/torrentmanager.h
    src/systemtray.h
    src/utils.h
)

# ── Sanitizers ───────────────────────────────────────────────────────────
option(ENABLE_SANITIZERS "Enable AddressSanitizer, ThreadSanitizer, UBSan" OFF)

if(ENABLE_SANITIZERS AND NOT MSVC)
    add_compile_options(
        -fsanitize=address
        -fsanitize=thread
        -fsanitize=undefined
        -fno-omit-frame-pointer
    )
    add_link_options(
        -fsanitize=address
        -fsanitize=thread
        -fsanitize=undefined
    )
    message(STATUS "Sanitizers enabled: ASan + TSan + UBSan")
elseif(ENABLE_SANITIZERS AND MSVC)
    message(WARNING "Sanitizers not supported on MSVC, skipping")
endif()

# ── Coverage ──────────────────────────────────────────────────────────────
option(ENABLE_COVERAGE "Enable code coverage flags" OFF)

if(ENABLE_COVERAGE)
    if(NOT MSVC)
        add_compile_options(--coverage)
        add_link_options(--coverage)
        message(STATUS "Code coverage enabled")
    else()
        message(WARNING "Coverage not supported on MSVC, skipping")
    endif()
endif()

# ── Цель сборки ───────────────────────────────────────────────────────
add_executable(${PROJECT_NAME} ${SOURCES} ${HEADERS})

# ── Include directories for main target ─────────────────────────────────
target_include_directories(${PROJECT_NAME} PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/src
    ${CMAKE_CURRENT_SOURCE_DIR}/src/interfaces
)

# ── Подключение библиотек ─────────────────────────────────────────────
target_link_libraries(${PROJECT_NAME} PRIVATE
    Qt6::Core
    Qt6::Gui
    Qt6::Network
)

if(NOT APPLE)
    # Not macOS - use OpenGL/OpenGLWidgets normally
    target_link_libraries(${PROJECT_NAME} PRIVATE
        Qt6::Widgets
        Qt6::Svg
        Qt6::OpenGL
        Qt6::OpenGLWidgets
    )
else()
    # On macOS 14+, AGL framework was removed - use NO_OPENGL mode
    target_link_libraries(${PROJECT_NAME} PRIVATE
        Qt6::Widgets
        Qt6::Svg
    )
    target_compile_definitions(${PROJECT_NAME} PRIVATE NO_OPENGL)
    # Add framework search path for our AGL.framework stub
    target_link_options(${PROJECT_NAME} PRIVATE "-F${CMAKE_BINARY_DIR}")
endif()

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

# ── Sanitizers ──────────────────────────────────────────────────────────
option(ENABLE_SANITIZERS "Enable AddressSanitizer, ThreadSanitizer, UBSan" OFF)

if(ENABLE_SANITIZERS AND NOT MSVC)
    # AddressSanitizer + ThreadSanitizer + UBSan
    # Note: ASan and TSan cannot be used together in the same binary
    # We'll build separate test targets for each
    add_compile_options(-fsanitize=undefined -fno-omit-frame-pointer)
    add_link_options(-fsanitize=undefined)
    message(STATUS "Sanitizers enabled: AddressSanitizer, ThreadSanitizer, UBSan")
elseif(ENABLE_SANITIZERS AND MSVC)
    message(WARNING "Sanitizers not fully supported on MSVC, only UBSan available")
    add_compile_options(/fsanitize=undefined)
endif()

# ── Google Mock / Google Test via FetchContent ──────────────────────────
option(BUILD_GMOCK "Build Google Mock for unit testing" ON)

if(BUILD_GMOCK AND BUILD_TESTS)
    include(FetchContent)
    FetchContent_Declare(
        googletest
        GIT_REPOSITORY https://github.com/google/googletest.git
        GIT_TAG v1.14.0
    )
    FetchContent_MakeAvailable(googletest)
    
    # gtest/gmock targets will be available as:
    # gtest, gtest_main, gmock, gmock_main
    message(STATUS "Google Test/Mock ${googletest_VERSION} available")
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
        src/interfaces/inetworkmanager.h
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
            Qt6::Test
            ${MPV_LIBRARIES}
        )

        # OpenGL/OpenGLWidgets not available on macOS 14+ due to AGL removal
        if(NOT APPLE)
            target_link_libraries(test_mpvwidget PRIVATE
                Qt6::OpenGL
                Qt6::OpenGLWidgets
            )
            target_compile_definitions(test_mpvwidget PRIVATE
                HAS_MPV
                HAS_MPV_RENDER
            )
        else()
            # macOS: NO_OPENGL mode - test will have limited functionality
            target_compile_definitions(test_mpvwidget PRIVATE
                HAS_MPV
                HAS_MPV_RENDER
                NO_OPENGL
            )
        endif()

        target_include_directories(test_mpvwidget PRIVATE
            ${MPV_INCLUDE_DIRS}
        )

        add_test(NAME MpvWidgetTest COMMAND test_mpvwidget)
        set_tests_properties(MpvWidgetTest PROPERTIES
            ENVIRONMENT "QT_QPA_PLATFORM=offscreen"
        )
    endif()

    message(STATUS "Unit tests enabled")

    # ── Sanitizer-specific test targets ────────────────────────────────────
    # These are separate executables because ASan and TSan cannot be combined
    
    if(ENABLE_SANITIZERS AND NOT MSVC AND BUILD_GMOCK)
        # AddressSanitizer test target
        add_executable(test_asan ${TEST_NETWORK_MANAGER_SOURCES}
            src/test_networkmanager_gmock.cpp
        )
        target_link_libraries(test_asan PRIVATE
            Qt6::Core
            Qt6::Network
            gmock_main
        )
        target_compile_options(test_asan PRIVATE -fsanitize=address -fno-omit-frame-pointer)
        target_link_options(test_asan PRIVATE -fsanitize=address)
        set_target_properties(test_asan PROPERTIES AUTOMOC ON)
        target_include_directories(test_asan PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            ${CMAKE_CURRENT_SOURCE_DIR}/src/mocks
        )
        add_test(NAME NetworkManagerTestASan COMMAND test_asan)
        set_tests_properties(NetworkManagerTestASan PROPERTIES ENVIRONMENT "ASAN_OPTIONS=detect_leaks=1:halt_on_error=1")

        # ThreadSanitizer test target
        add_executable(test_tsan ${TEST_NETWORK_MANAGER_SOURCES}
            src/test_networkmanager_gmock.cpp
        )
        target_link_libraries(test_tsan PRIVATE
            Qt6::Core
            Qt6::Network
            gmock_main
        )
        target_compile_options(test_tsan PRIVATE -fsanitize=thread -fno-omit-frame-pointer)
        target_link_options(test_tsan PRIVATE -fsanitize=thread)
        set_target_properties(test_tsan PROPERTIES AUTOMOC ON)
        target_include_directories(test_tsan PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            ${CMAKE_CURRENT_SOURCE_DIR}/src/mocks
        )
        add_test(NAME NetworkManagerTestTSan COMMAND test_tsan)
        set_tests_properties(NetworkManagerTestTSan PROPERTIES ENVIRONMENT "TSAN_OPTIONS=halt_on_error=1:second_deadlock_stack=1")
    endif()

    # ── gmock-based unit tests ────────────────────────────────────────────
    if(BUILD_GMOCK)
        # NetworkManager gmock tests
        add_executable(test_networkmanager_gmock
            src/test_networkmanager_gmock.cpp
            src/mocks/mock_networkmanager.cpp
        )
        target_link_libraries(test_networkmanager_gmock PRIVATE
            Qt6::Core
            Qt6::Network
            Qt6::Test
            gmock_main
        )
        set_target_properties(test_networkmanager_gmock PROPERTIES AUTOMOC ON)
        target_include_directories(test_networkmanager_gmock PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            ${CMAKE_CURRENT_SOURCE_DIR}/src/mocks
            ${CMAKE_CURRENT_SOURCE_DIR}/src/interfaces
        )
        
        if(ENABLE_COVERAGE)
            target_compile_options(test_networkmanager_gmock PRIVATE --coverage)
            target_link_options(test_networkmanager_gmock PRIVATE --coverage)
        endif()
        
        add_test(NAME NetworkManagerGMockTest COMMAND test_networkmanager_gmock)
        
        # TorrentManager gmock tests
        add_executable(test_torrentmanager_gmock
            src/test_torrentmanager_gmock.cpp
            src/mocks/mock_networkmanager.cpp
            src/mocks/mock_torrentmanager.cpp
        )
        target_link_libraries(test_torrentmanager_gmock PRIVATE
            Qt6::Core
            Qt6::Test
            gmock_main
        )
        set_target_properties(test_torrentmanager_gmock PROPERTIES AUTOMOC ON)
        target_include_directories(test_torrentmanager_gmock PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            ${CMAKE_CURRENT_SOURCE_DIR}/src/mocks
            ${CMAKE_CURRENT_SOURCE_DIR}/src/interfaces
        )
        
        if(ENABLE_COVERAGE)
            target_compile_options(test_torrentmanager_gmock PRIVATE --coverage)
            target_link_options(test_torrentmanager_gmock PRIVATE --coverage)
        endif()
        
        add_test(NAME TorrentManagerGMockTest COMMAND test_torrentmanager_gmock)
        
        # RoomManager gmock tests
        add_executable(test_roommanager_gmock
            src/test_roommanager_gmock.cpp
            src/mocks/mock_networkmanager.cpp
        )
        target_link_libraries(test_roommanager_gmock PRIVATE
            Qt6::Core
            Qt6::Test
            gmock_main
        )
        set_target_properties(test_roommanager_gmock PROPERTIES AUTOMOC ON)
        target_include_directories(test_roommanager_gmock PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            ${CMAKE_CURRENT_SOURCE_DIR}/src/mocks
            ${CMAKE_CURRENT_SOURCE_DIR}/src/interfaces
        )
        
        if(ENABLE_COVERAGE)
            target_compile_options(test_roommanager_gmock PRIVATE --coverage)
            target_link_options(test_roommanager_gmock PRIVATE --coverage)
        endif()
        
        add_test(NAME RoomManagerGMockTest COMMAND test_roommanager_gmock)
    endif()

    # ── Integration tests ─────────────────────────────────────────────────
    option(BUILD_INTEGRATION_TESTS "Build integration tests (require running backend)" OFF)
    
    if(BUILD_INTEGRATION_TESTS AND BUILD_TESTS)
        add_executable(test_integration
            src/test_integration.cpp
        )
        target_link_libraries(test_integration PRIVATE
            Qt6::Core
            Qt6::Network
            Qt6::Test
        )
        set_target_properties(test_integration PROPERTIES AUTOMOC ON)
        target_include_directories(test_integration PRIVATE
            ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/src
        )
        
        if(ENABLE_COVERAGE)
            target_compile_options(test_integration PRIVATE --coverage)
            target_link_options(test_integration PRIVATE --coverage)
        endif()
        
        add_test(NAME IntegrationTest COMMAND test_integration)
    endif()

endif()

# ── Информация о сборке ───────────────────────────────────────────────
message(STATUS "TorrSyncPlayer ${PROJECT_VERSION}")
message(STATUS "Qt6 найден: ${Qt6Core_VERSION}")
message(STATUS "libmpv найден: ${MPV_VERSION}")

# ── Установка ──────────────────────────────────────────────────────────
install(TARGETS ${PROJECT_NAME} RUNTIME DESTINATION bin)