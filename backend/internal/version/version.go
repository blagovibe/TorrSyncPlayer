// Package version предоставляет информацию о версии сервера.
// Переменные устанавливаются при сборке через -ldflags.
package version

import "runtime"

// Версионирование (устанавливается при сборке через -ldflags)
var (
	// Version версия приложения
	Version = "dev"

	// Commit хеш коммита git
	Commit = "unknown"

	// BuildTime время сборки
	BuildTime = "unknown"
)

// Info возвращает информацию о версии
func Info() map[string]interface{} {
	return map[string]interface{}{
		"version": Version,
		"commit":  Commit,
		"build":   BuildTime,
		"runtime": map[string]string{
			"go":   runtime.Version(),
			"arch": runtime.GOARCH,
			"os":   runtime.GOOS,
		},
	}
}
