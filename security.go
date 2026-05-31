package main

import (
	"log"
	"strings"
)

// SanitizeLogValue очищает значение для безопасного логирования
func SanitizeLogValue(value string) string {
	// Убираем потенциально чувствительные данные
	if strings.HasPrefix(value, "magnet:") {
		// Логируем только хеш торрента
		parts := strings.Split(value, "&")
		if len(parts) > 0 {
			return parts[0] + "&..."
		}
	}
	return value
}

// LogSecurityEvent логирует события безопасности
func LogSecurityEvent(event string, details map[string]interface{}) {
	log.Printf("[SECURITY] %s: %v", event, details)
}
