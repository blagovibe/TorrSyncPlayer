/**
 * @file mock_networkmanager.cpp
 * @brief Реализация MockNetworkManager
 * 
 * Пустая реализация для линковки. Все методы мокируются через gmock
 * в заголовочном файле с помощью MOCK_METHOD.
 */

#include "mocks/mock_networkmanager.h"

#include <QJsonDocument>

// Конструктор/деструктор уже определены в заголовке как inline
// Этот файл нужен только для корректной сборки при линковке
// отдельных тестовых исполняемых файлов

// Экспорт для Google Mock
// Все методы уже объявлены через MOCK_METHOD в .h файле