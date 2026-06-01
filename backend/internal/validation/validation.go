// Package validation предоставляет общие функции валидации.
// Содержит утилиты для проверки корректности данных.
package validation

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Регулярные выражения для валидации
var (
	// roomNameRegex допускает буквы, цифры, пробелы, дефисы и подчёркивания
	roomNameRegex = regexp.MustCompile(`^[\p{L}\p{N}\s\-_]{1,50}$`)
	// usernameRegex допускает буквы, цифры, подчёркивания и дефисы
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-]{3,30}$`)
	// magnetRegex для валидации magnet-ссылок
	magnetRegex = regexp.MustCompile(`^magnet:\?xt=urn:[a-z0-9]+:[a-zA-Z0-9]{32,40}`)
)

// Ограничения размеров
const (
	MaxUsernameLength    = 30
	MinUsernameLength    = 3
	MaxPasswordLength    = 72
	MinPasswordLength    = 8
	MaxRoomNameLength    = 50
	MinRoomNameLength    = 1
	MaxTorrentNameLength = 255
	MaxFileSize          = 100 * 1024 * 1024 * 1024 // 100 GB
	MaxRequestSize       = 1 << 20                  // 1 MB
)

// ValidatePosition валидирует позицию воспроизведения.
// Проверяет что позиция не NaN, не Inf, не отрицательная и не превышает 24 часа.
// Возвращает ошибку если позиция некорректна.
func ValidatePosition(position float64) error {
	if math.IsNaN(position) || math.IsInf(position, 0) {
		return fmt.Errorf("некорректное значение позиции: NaN или Inf")
	}
	if position < 0 {
		return fmt.Errorf("позиция не может быть отрицательной: %f", position)
	}
	if position > 86400 {
		return fmt.Errorf("позиция превышает максимальное значение (24 часа): %f", position)
	}
	return nil
}

// ValidateUsername валидирует имя пользователя.
// Проверяет длину, допустимые символы и отсутствие пробелов в начале/конце.
func ValidateUsername(username string) error {
	username = strings.TrimSpace(username)

	if utf8.RuneCountInString(username) < MinUsernameLength {
		return fmt.Errorf("имя пользователя слишком короткое (минимум %d символа)", MinUsernameLength)
	}
	if utf8.RuneCountInString(username) > MaxUsernameLength {
		return fmt.Errorf("имя пользователя слишком длинное (максимум %d символов)", MaxUsernameLength)
	}
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("имя пользователя содержит недопустимые символы (разрешены: a-z, A-Z, 0-9, _, -)")
	}
	return nil
}

// ValidatePassword валидирует пароль.
// Проверяет длину и сложность (наличие букв и цифр).
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("пароль слишком короткий (минимум %d символов)", MinPasswordLength)
	}
	if len(password) > MaxPasswordLength {
		return fmt.Errorf("пароль слишком длинный (максимум %d байт)", MaxPasswordLength)
	}

	// Проверяем наличие хотя бы одной буквы и одной цифры
	hasLetter := false
	hasDigit := false
	for _, r := range password {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return fmt.Errorf("пароль должен содержать хотя бы одну букву и одну цифру")
	}

	return nil
}

// ValidateRoomName валидирует название комнаты.
// Проверяет длину и допустимые символы.
func ValidateRoomName(name string) error {
	name = strings.TrimSpace(name)

	if utf8.RuneCountInString(name) < MinRoomNameLength {
		return fmt.Errorf("название комнаты не может быть пустым")
	}
	if utf8.RuneCountInString(name) > MaxRoomNameLength {
		return fmt.Errorf("название комнаты слишком длинное (максимум %d символов)", MaxRoomNameLength)
	}
	if !roomNameRegex.MatchString(name) {
		return fmt.Errorf("название комнаты содержит недопустимые символы")
	}
	return nil
}

// ValidateMagnetURI валидирует формат magnet-ссылки.
func ValidateMagnetURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("magnet-ссылка не может быть пустой")
	}
	if len(uri) > 2048 {
		return fmt.Errorf("magnet-ссылка слишком длинная")
	}
	if !magnetRegex.MatchString(uri) {
		return fmt.Errorf("неверный формат magnet-ссылки")
	}
	return nil
}

// ValidateTorrentName валидирует название торрента.
func ValidateTorrentName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("название торрента не может быть пустым")
	}
	if utf8.RuneCountInString(name) > MaxTorrentNameLength {
		return fmt.Errorf("название торрента слишком длинное (максимум %d символов)", MaxTorrentNameLength)
	}

	// Проверяем на наличие управляющих символов
	for _, r := range name {
		if r < 32 && r != '\t' && r != '\n' {
			return fmt.Errorf("название торрента содержит недопустимые символы")
		}
	}

	return nil
}

// ValidateFileSize валидирует размер файла.
func ValidateFileSize(size int64) error {
	if size < 0 {
		return fmt.Errorf("размер файла не может быть отрицательным")
	}
	if size > MaxFileSize {
		return fmt.Errorf("файл слишком большой (максимум %d GB)", MaxFileSize/(1024*1024*1024))
	}
	return nil
}

// ValidateFileIndex валидирует индекс файла.
func ValidateFileIndex(index, maxIndex int) error {
	if index < 0 {
		return fmt.Errorf("индекс файла не может быть отрицательным")
	}
	if maxIndex >= 0 && index > maxIndex {
		return fmt.Errorf("индекс файла выходит за допустимые пределы")
	}
	return nil
}

// SanitizeString очищает строку от потенциально опасных символов.
// Используется для вывода пользовательского ввода.
func SanitizeString(s string) string {
	// Удаляем управляющие символы
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' && r != '\n' {
			return -1
		}
		return r
	}, s)

	// Ограничиваем длину
	if utf8.RuneCountInString(s) > 1000 {
		s = string([]rune(s)[:1000])
	}

	return strings.TrimSpace(s)
}
