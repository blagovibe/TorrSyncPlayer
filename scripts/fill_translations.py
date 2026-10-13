#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
# Copyright (c) 2025-2026 TorrSyncPlayer contributors
# See LICENSE file for full license text
"""
Наполняет translations/<lang>.ts переводами.

Использование:
    python3 scripts/fill_translations.py

Запускать ПОСЛЕ `cmake --build . --target update_translations`: lupdate
пересобирает .ts из исходников и сохраняет уже заполненные переводы, но
новые строки появляются пустыми.

Язык исходников — русский, поэтому ru.ts остаётся намеренно пустым: пустая
переводная строка означает «показывать исходник», и Qt так и делает. Наполнять
её копией исходника незачем — это 164 строки, которые будут молча расходиться
с кодом.
"""

import sys
import xml.etree.ElementTree as ET
from pathlib import Path

# Переводы: контекст → {исходная строка: перевод}.
# Строки, которые уже по-английски ('Play', 'Pause', фильтр файлов),
# сознательно НЕ перечислены: lupdate пометил бы их какfinished, и при
# следующем обновлении переводчик увидел бы «готово» там, где переводить
# нечего.
EN = {
    "CommandLine": {
        "Interface language: ru, en (default: saved setting or system language)":
            "Interface language: ru, en (default: saved setting or system language)",
    },
    "MainWindow": {
        "TorrPlayer - Видеоплеер с торрентами": "TorrPlayer - Torrent video player",
        "Готово к работе": "Ready",
        "Готово": "Ready",
        "&Файл": "&File",
        "Добавить торрент...": "Add torrent...",
        "Выход": "Quit",
        "&Комната": "&Room",
        "Создать комнату...": "Create room...",
        "Присоединиться...": "Join...",
        "Покинуть комнату": "Leave room",
        "Скопировать код приглашения": "Copy invite code",
        "Торренты": "Torrents",
        "Вставьте magnet-ссылку...": "Paste magnet link...",
        "Добавить": "Add",
        "Добавить торрент": "Add torrent",
        "Добавить .torrent файл": "Add .torrent file",
        "Удалить торрент": "Remove torrent",
        "Файлы торрента:": "Files in torrent:",
        "Воспроизведение": "Playback",
        "Синхронизация": "Sync",
        "Создать комнату": "Create room",
        "Присоединиться": "Join",
        "Покинуть": "Leave",
        "Не в комнате": "Not in a room",
        "Ошибка": "Error",
        "Введите magnet-ссылку": "Enter a magnet link",
        "Добавление торрента...": "Adding torrent...",
        "Выберите .torrent файл": "Select a .torrent file",
        "Не удалось открыть файл: %1": "Could not open file: %1",
        "Файл .torrent пуст": "The .torrent file is empty",
        "Добавление торрента из файла...": "Adding torrent from file...",
        "Выбран торрент: %1": "Selected torrent: %1",
        "Воспроизведение: %1": "Playing: %1",
        "Файлов в торренте: %1": "Files in torrent: %1",
        "Ошибка: ID торрента пуст": "Error: torrent ID is empty",
        "Запрос тикета потока...": "Requesting stream ticket...",
        "Ошибка: URL потока пуст": "Error: stream URL is empty",
        "Тикет потока обновлён": "Stream ticket refreshed",
        "Воспроизведение недоступно": "Playback unavailable",
        "Эта сборка собрана без поддержки libmpv. Воспроизведение видео отключено. "
        "Установите libmpv и пересоберите приложение для просмотра.":
            "This build was made without libmpv support, so video playback is "
            "disabled. Install libmpv and rebuild the application to watch videos.",
        "libmpv не подключён — воспроизведение отключено":
            "libmpv is not linked — playback is disabled",
        "Загрузка потока...": "Loading stream...",
        "Создание комнаты...": "Creating room...",
        "Присоединение к комнате...": "Joining room...",
        "Нет комнаты — код приглашения собирать не из чего":
            "No room — there is nothing to build an invite code from",
        "Код приглашения скопирован: %1": "Invite code copied: %1",
        "Комната создана: %1": "Room created: %1",
        "Присоединились к комнате: %1": "Joined room: %1",
        "Покинули комнату": "Left the room",
        "Ожидание: участник буферизует": "Waiting: a peer is buffering",
        "Воспроизведение синхронизировано": "Playback synchronised",
        "Пир присоединился: %1": "Peer joined: %1",
        "Воспроизведение синхронизировано: %1": "Playback synchronised: %1",
        "Участников в комнате: %1": "Peers in room: %1",
        "%1 (вы)": "%1 (you)",
        " · хост": " · host",
        " · буферизует": " · buffering",
        "В комнате: %1": "In room: %1",
        ", ": ", ",
        "отстаём на %1 с": "%1 s behind",
        "опережаем на %1 с": "%1 s ahead",
        "Пир покинул комнату: %1": "Peer left the room: %1",
        "Воспроизведение завершено": "Playback finished",
        "Воспроизведение прервано, переподключаемся…":
            "Playback interrupted, reconnecting…",
        "Ошибка воспроизведения": "Playback error",
        "Ошибка: %1": "Error: %1",
        "Ошибка сети": "Network error",
        "Комната: %1 (хост)": "Room: %1 (host)",
        "Комната: %1": "Room: %1",
        "⚠ Сервер недоступен - работа в offline режиме":
            "⚠ Server unavailable — running offline",
        "✓ Подключение восстановлено": "✓ Connection restored",
    },
    "MpvWidget": {
        "Не удалось создать mpv экземпляр": "Could not create the mpv instance",
        "Ошибка инициализации mpv: %1": "mpv initialisation failed: %1",
        "Ошибка загрузки файла: %1": "Could not load the file: %1",
        "Ошибка перемотки: %1": "Seek failed: %1",
        "Ошибка воспроизведения: код ошибки %1":
            "Playback error: error code %1",
        "MPV: %1": "MPV: %1",
    },
    "NetworkManager": {
        "Magnet-ссылка не может быть пустой": "The magnet link cannot be empty",
        "Magnet-ссылка слишком длинная (максимум %1 символов)":
            "The magnet link is too long (maximum %1 characters)",
        "Некорректная magnet-ссылка. Должна начинаться с 'magnet:?'":
            "Invalid magnet link. It must start with 'magnet:?'",
        "Файл .torrent пуст": "The .torrent file is empty",
        "Файл .torrent слишком большой (максимум %1 МБ)":
            "The .torrent file is too large (maximum %1 MB)",
        "ID торрента не может быть пустым": "The torrent ID cannot be empty",
        "ID торрента слишком длинный": "The torrent ID is too long",
        "Индекс файла не может быть отрицательным":
            "The file index cannot be negative",
        "Имя комнаты не может быть пустым": "The room name cannot be empty",
        "Имя комнаты должно содержать минимум %1 символа":
            "The room name must be at least %1 characters",
        "Имя комнаты слишком длинное (максимум %1 символов)":
            "The room name is too long (maximum %1 characters)",
        "Пароль не может быть отправлен по незащищённому соединению. "
        "Используйте HTTPS или localhost.":
            "The password cannot be sent over an unencrypted connection. "
            "Use HTTPS or localhost.",
        "ID комнаты не может быть пустым": "The room ID cannot be empty",
        "ID комнаты слишком длинный": "The room ID is too long",
        "Не в комнате": "Not in a room",
        "Позиция не может быть отрицательной": "The position cannot be negative",
        "Некорректный ID торрента": "Invalid torrent ID",
        "Ответ сервера слишком большой (максимум %1 байт)":
            "The server response is too large (maximum %1 bytes)",
        "Ответ сервера слишком большой": "The server response is too large",
        "Ошибка парсинга JSON от %1": "Could not parse JSON from %1",
        "Сервер не вернул тикет потока": "The server returned no stream ticket",
        "Превышен лимит буфера SSE": "SSE buffer limit exceeded",
        "Превышен лимит данных SSE": "SSE data limit exceeded",
        "Сервер недоступен после %1 попыток":
            "Server unavailable after %1 attempts",
        "Не удалось восстановить SSE соединение после %1 попыток":
            "Could not restore the SSE connection after %1 attempts",
        "SSE ошибка: %1": "SSE error: %1",
        "SSE ошибка: соединение потеряно": "SSE error: connection lost",
        "Не удалось открыть поток событий комнаты (HTTP %1)":
            "Could not open the room event stream (HTTP %1)",
        "Authentication required — please log in":
            "Authentication required — please log in",
        "Не удалось зарегистрироваться (проверьте имя и пароль)":
            "Could not sign in (check the name and password)",
        "Ошибка сервера при регистрации: %1": "Server error while signing in: %1",
        "Ошибка %1: %2": "Error %1: %2",
    },
    "RoomDialog": {
        "Управление комнатой": "Room management",
        "Создать комнату": "Create a room",
        "Присоединиться": "Join",
        "Создать": "Create",
        "Войти": "Join",
        "Отмена": "Cancel",
        "Имя комнаты:": "Room name:",
        "Введите имя комнаты...": "Enter a room name...",
        "Имя будет отображаться для других участников":
            "The name is shown to the other participants",
        "Пароль:": "Password:",
        "Необязательно": "Optional",
        "Показать": "Show",
        "После создания комнаты вы станете хостом и сможете управлять воспроизведением":
            "After creating a room you become the host and control playback",
        "Код приглашения:": "Invite code:",
        "Вставьте код от хоста — подставим ID и пароль":
            "Paste the host's code — the ID and password are filled in",
        "Подставить": "Fill in",
        "Код — это тот же ID и пароль через двоеточие, чтобы отправить одним сообщением":
            "The code is the same ID and password joined by a colon, so it can "
            "be sent as one message",
        "ID комнаты:": "Room ID:",
        "Введите ID комнаты...": "Enter the room ID...",
        "ID комнаты можно получить у хоста": "Get the room ID from the host",
        "Если требуется": "If required",
        "Присоединившись к комнате, вы будете синхронизированы с хостом воспроизведения":
            "By joining a room you are synchronised with the playback host",
        "Код не разобран: ожидается ID комнаты (32 hex-символа) и пароль после двоеточия":
            "Could not parse the code: expected a room ID (32 hex characters) "
            "and a password after the colon",
        "ID и пароль подставлены из кода": "ID and password filled in from the code",
        "Введите имя комнаты": "Enter a room name",
        "Имя комнаты должно содержать минимум 2 символа":
            "The room name must be at least 2 characters",
        "Имя комнаты содержит недопустимые символы":
            "The room name contains invalid characters",
        "Введите ID комнаты": "Enter a room ID",
        "ID комнаты слишком короткий": "The room ID is too short",
    },
    "RoomManager": {
        "Имя комнаты не может быть пустым": "The room name cannot be empty",
        "ID комнаты не может быть пустым": "The room ID cannot be empty",
        "Не в комнате": "Not in a room",
    },
    "SystemTray": {
        "TorrPlayer - Видеоплеер": "TorrPlayer - Video player",
        "TorrPlayer": "TorrPlayer",
        "Приложение свёрнуто в трей. Двойной клик для восстановления.":
            "The app was minimised to the tray. Double-click to restore it.",
        "Показать": "Show",
        "Скрыть в трей": "Hide to tray",
        "Выход": "Quit",
    },
    "TorrentManager": {
        "Magnet-ссылка не может быть пустой": "The magnet link cannot be empty",
        "Данные торрент-файла не могут быть пустыми":
            "The torrent file data cannot be empty",
        "Файл .torrent слишком большой (максимум %1 МБ)":
            "The .torrent file is too large (maximum %1 MB)",
        "ID торрента не может быть пустым": "The torrent ID cannot be empty",
        "Некорректный индекс файла": "Invalid file index",
    },
    "TorrentModel": {
        "Загрузка": "Downloading",
        "Раздача": "Seeding",
        "Пауза": "Paused",
        "Получение данных": "Fetching metadata",
        "Ошибка": "Error",
        "Неизвестно": "Unknown",
        "ID: %1\nИмя: %2\nПрогресс: %3%\nСтатус: %4\nРазмер: %5":
            "ID: %1\nName: %2\nProgress: %3%\nStatus: %4\nSize: %5",
        "Торрент": "Torrent",
    },
}

# Строки, уже готовые к переводу: они по-английски и в словаре EN
# отсутствуют. lupdate отметит их finished автоматически.
def fill(path: Path, lang: str, table: dict | None) -> int:
    tree = ET.parse(path)
    root = tree.getroot()
    root.set("language", lang)

    filled = 0
    missing = []

    for ctx in root.findall("context"):
        name = ctx.find("name").text
        for msg in ctx.findall("message"):
            source = msg.find("source").text
            translation = msg.find("translation")

            if translation is None:
                translation = ET.SubElement(msg, "translation")
                translation.text = ""

            # Уже переведено — не трогаем: новый запуск скрипта не должен затирать
            # работу переводчика. Но если перевод дословно совпадает с
            # исходником (это наши собственные identity-переводы для
            # 'Play'/'Pause'/фильтра файлов), флаг unfinished надо снять:
            # lrelease иначе вечно считает строку незавершённой.
            if (translation.text or "").strip():
                if translation.text == source:
                    translation.attrib.pop("type", None)
                continue

            if table is None:
                # Язык исходников: оставляем пустым.
                continue

            if source in table.get(name, {}):
                translation.text = table[name][source]
                translation.attrib.pop("type", None)
                filled += 1
            elif source.isascii():
                # Уже английский (Play, Pause, фильтр файлов): перевод
                # совпадает с исходником. Флаг unfinished снимаем — иначе
                # lrelease будет вечно считать строку незавершённой и
                # переводчики будут видеть в редакторе работу, которой нет.
                translation.text = source
                translation.attrib.pop("type", None)
                filled += 1
            else:
                missing.append(f"{name}: {source[:60]}")

    tree.write(path, encoding="utf-8", xml_declaration=True)
    # lupdate пишет DOCTYPE, ElementTree — нет. Без него Qt Linguist Tools
    # считает файл некорректным при следующем обновлении.
    body = path.read_text(encoding="utf-8")
    if "<!DOCTYPE TS>" not in body:
        body = body.replace(
            "<?xml version='1.0' encoding='utf-8'?>\n",
            '<?xml version="1.0" encoding="utf-8"?>\n<!DOCTYPE TS>\n',
            1,
        )
        path.write_text(body, encoding="utf-8")

    return filled, missing


def main() -> int:
    root = Path(__file__).resolve().parent.parent / "frontend" / "translations"

    en_path = root / "en.ts"
    if not en_path.exists():
        print(f"{en_path} не найден — сначала запустите update_translations")
        return 1

    filled, missing = fill(en_path, "en", EN)

    ru_path = root / "ru.ts"
    if ru_path.exists():
        fill(ru_path, "ru", None)  # язык исходников: пустые переводы

    print(f"en.ts: переведено {filled} строк(и)")
    if missing:
        print("НЕ переведено (нужно дописать в EN):")
        for item in missing:
            print(f"  {item}")

    return 1 if missing else 0


if __name__ == "__main__":
    sys.exit(main())