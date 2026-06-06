// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package main точка входа сервера
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// @title           TorrSyncPlayer API
// @version         1.0
// @description     HTTP API для TorrSyncPlayer — торрент-плеера с P2P синхронизацией воспроизведения.
// @termsOfService  https://github.com/blagovibe/TorrSyncPlayer

// @contact.name   TorrSyncPlayer Support
// @contact.url    https://github.com/blagovibe/TorrSyncPlayer/issues
// @contact.email  support@torrsyncplayer.local

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8889
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Введите JWT токен в формате: Bearer <token>

const (
	defaultPort     = "8889"
	defaultDir      = "./data"
	shutdownTimeout = 30 * time.Second
)

// Версионирование (устанавливается при сборке через -ldflags)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Config конфигурация сервера
type Config struct {
	Port                  string
	MemoryStorageCapacity int64 // Максимальный размер in-memory хранилища в байтах
	TLSCert               string
	TLSKey                string
	UseTLS                bool
	AutoTLS               bool // Генерировать self-signed сертификат
	JWTSecret             string
	EnableProfiling       bool // Включить pprof профилирование
}

func main() {
	config := parseFlags()

	logLevel := getEnv("LOG_LEVEL", "info")
	logFormat := getEnv("LOG_FORMAT", "text")
	logger.Init(logLevel, logFormat)

	if err := validateConfig(config); err != nil {
		logger.Error("Ошибка конфигурации", "error", err)
		os.Exit(1)
	}

	logger.Info("Запуск TorrServer",
		"version", Version,
		"commit", Commit,
		"build_time", BuildTime,
	)

	// Создаём сервис аутентификации
	authService, err := auth.NewAuthService([]byte(config.JWTSecret))
	if err != nil {
		logger.Error("Ошибка создания auth service", "error", err)
		os.Exit(1)
	}

	// Инициализация сервиса буферизации
	bufferSvc := buffer.NewService(constants.DefaultMaxBufferSize)
	bufferSvc.StartPeriodicUpdate(constants.BufferUpdateInterval)

	// Опции торрент-сервиса (всегда in-memory)
	torrentOpts := torrent.ServiceOptions{
		MemoryStorageCapacity: config.MemoryStorageCapacity,
	}

	// Инициализация торрент-сервиса с буферизацией и in-memory хранилищем
	torrentSvc, err := torrent.NewServiceWithOptions(bufferSvc, torrentOpts)
	if err != nil {
		logger.Error("Ошибка инициализации торрент-сервиса", "error", err)
		os.Exit(1)
	}

	logger.Info("Торрент-сервис инициализирован",
		"memory_capacity", config.MemoryStorageCapacity,
	)

	p2pSvc, err := p2p.NewService(authService)
	if err != nil {
		logger.Error("Ошибка инициализации P2P-сервиса", "error", err)
		os.Exit(1)
	}

	syncSvc := sync.NewService()

	// Создаём хранилище пользователей
	authStore := auth.NewUserStore()

	// Создаём роутер
	router := api.NewRouter(api.RouterConfig{
		TorrentSvc:  torrentSvc,
		P2pSvc:      p2pSvc,
		SyncSvc:     syncSvc,
		AuthStore:   authStore,
		AuthService: authService,
	})

	// Настраиваем HTTP сервер
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", config.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Настраиваем TLS если включён
	if config.UseTLS {
		tlsConfig, err := configureTLS(config)
		if err != nil {
			logger.Error("Ошибка настройки TLS", "error", err)
			os.Exit(1)
		}
		server.TLSConfig = tlsConfig
	}

	// Настраиваем pprof если включён
	if config.EnableProfiling {
		setupPprof()
		logger.Info("pprof профилирование включено", "endpoint", "/debug/pprof/")
	}

	// Запускаем сервер в горутине
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("HTTP сервер: горутина завершилась с паникой", "error", r)
			}
		}()
		logger.Info("HTTP сервер запущен", "port", config.Port, "tls", config.UseTLS)
		var err error
		if config.UseTLS {
			err = server.ListenAndServeTLS(config.TLSCert, config.TLSKey)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Ошибка HTTP сервера", "error", err)
			os.Exit(1)
		}
	}()

	// Ожидаем сигнала для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	logger.Info("Получен сигнал завершения", "signal", sig.String())

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Останавливаем HTTP сервер
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Ошибка при остановке HTTP сервера", "error", err)
	}
	logger.Info("HTTP сервер остановлен")

	// Останавливаем sync сервис
	syncSvc.Close()

	// Останавливаем P2P сервис
	if err := p2pSvc.Close(); err != nil {
		logger.Error("Ошибка при остановке P2P-сервиса", "error", err)
	}

	// Останавливаем торрент-сервис
	if err := torrentSvc.Close(); err != nil {
		logger.Error("Ошибка при остановке торрент-сервиса", "error", err)
	}

	// Останавливаем буферный сервис
	bufferSvc.Close()

	// Останавливаем auth service (включая revocation store cleanup)
	authService.Stop()

	// Останавливаем CSRF cleanup горутину
	api.CSRFStore.Stop()

	logger.Info("Сервер остановлен")
}

// validateConfig проверяет корректность конфигурации сервера.
// Возвращает ошибка если конфигурация некорректна.
func validateConfig(config Config) error {
	if config.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET не задан — установите переменную окружения или флаг -jwt-secret (минимум 32 символа)")
	}
	if len(config.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET слишком короткий (получено %d символов, минимум 32)", len(config.JWTSecret))
	}

	port, err := strconv.Atoi(config.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("некорректный порт: %s (должен быть 1-65535)", config.Port)
	}

	if config.MemoryStorageCapacity < 0 {
		return fmt.Errorf("memory-capacity не может быть отрицательным: %d", config.MemoryStorageCapacity)
	}

	if !config.UseTLS {
		logger.Warn("Запуск без TLS — все данные передаются в открытом виде. Не используйте в production!")
	}

	return nil
}

// parseFlags парсит флаги командной строки
func parseFlags() Config {
	var config Config

	flag.StringVar(&config.Port, "port", getEnv("PORT", defaultPort), "Порт сервера")
	flag.Int64Var(&config.MemoryStorageCapacity, "memory-capacity", func() int64 {
		if v := os.Getenv("MEMORY_CAPACITY"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
		return constants.DefaultMaxBufferSize
	}(), "Максимальный размер in-memory хранилища в байтах")
	flag.StringVar(&config.TLSCert, "tls-cert", getEnv("TLS_CERT", ""), "Путь к TLS сертификату")
	flag.StringVar(&config.TLSKey, "tls-key", getEnv("TLS_KEY", ""), "Путь к TLS ключу")
	flag.BoolVar(&config.UseTLS, "tls", false, "Включить TLS")
	flag.BoolVar(&config.AutoTLS, "auto-tls", false, "Генерировать self-signed сертификат")
	flag.StringVar(&config.JWTSecret, "jwt-secret", getEnv("JWT_SECRET", ""), "Секрет для JWT токенов")
	flag.BoolVar(&config.EnableProfiling, "enable-profiling", false, "Включить pprof профилирование")

	flag.Parse()

	// Если включён auto-tls, генерируем сертификат
	if config.AutoTLS {
		config.UseTLS = true
		if config.TLSCert == "" || config.TLSKey == "" {
			cert, key, err := generateSelfSignedCert()
			if err != nil {
				logger.Error("Ошибка генерации self-signed сертификата", "error", err)
				os.Exit(1)
			}
			config.TLSCert = cert
			config.TLSKey = key
			defer func() { _ = os.Remove(cert) }()
		defer func() { _ = os.Remove(key) }()
		}
	}

	// Если указаны сертификаты, включаем TLS
	if config.TLSCert != "" && config.TLSKey != "" {
		config.UseTLS = true
	}

	return config
}

// configureTLS настраивает TLS конфигурацию
func configureTLS(config Config) (*tls.Config, error) {
	// Если сертификаты не указаны, но TLS включён - генерируем self-signed
	if config.TLSCert == "" || config.TLSKey == "" {
		logger.Warn("TLS сертификаты не указаны, генерируем self-signed сертификат")
		cert, key, err := generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("ошибка генерации сертификата: %w", err)
		}
		config.TLSCert = cert
		config.TLSKey = key
		defer func() { _ = os.Remove(cert) }()
		defer func() { _ = os.Remove(key) }()
	}

	// Загружаем сертификат
	cert, err := tls.LoadX509KeyPair(config.TLSCert, config.TLSKey)
	if err != nil {
		return nil, fmt.Errorf("ошибка загрузки сертификата: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}, nil
}

// generateSelfSignedCert генерирует self-signed сертификат для разработки
func generateSelfSignedCert() (certPath, keyPath string, err error) {
	// Генерируем приватный ключ
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("ошибка генерации ключа: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("ошибка генерации serial number: %w", err)
	}

	// Создаём шаблон сертификата
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"TorrSyncPlayer"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	// Генерируем сертификат
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", fmt.Errorf("ошибка создания сертификата: %w", err)
	}

	// Создаём временные файлы
	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		return "", "", fmt.Errorf("ошибка создания файла сертификата: %w", err)
	}

	keyFile, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		return "", "", fmt.Errorf("ошибка создания файла ключа: %w", err)
	}

	// Записываем сертификат
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return "", "", fmt.Errorf("ошибка записи сертификата: %w", err)
	}

	// Записываем ключ
	privateKeyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("ошибка маршалинга ключа: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyDER}); err != nil {
		return "", "", fmt.Errorf("ошибка записи ключа: %w", err)
	}

	certPath = certFile.Name()
	keyPath = keyFile.Name()

	if err := certFile.Close(); err != nil {
		logger.Error("Ошибка закрытия файла сертификата", "error", err)
	}
	if err := keyFile.Close(); err != nil {
		logger.Error("Ошибка закрытия файла ключа", "error", err)
	}

	logger.Info("Сгенерирован self-signed сертификат", "cert", certPath, "key", keyPath)
	return certPath, keyPath, nil
}

// setupPprof настраивает pprof маршруты для профилирования
// ВНИМАНИЕ: pprof слушает ТОЛЬКО на 127.0.0.1:6060 для безопасности
// Не выставляйте pprof в публичную сеть без аутентификации!
func setupPprof() {
	// Создаём отдельный мультиплексор для pprof
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Запускаем pprof сервер ТОЛЬКО на localhost для безопасности
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("pprof: горутина завершилась с паникой", "error", r)
			}
		}()
		// Важно: слушаем только на 127.0.0.1, не на 0.0.0.0
		pprofAddr := "127.0.0.1:6060"
		logger.Info("pprof сервер запущен (только localhost)", "addr", pprofAddr)
		pprofServer := &http.Server{
			Addr:         pprofAddr,
			Handler:      pprofMux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Ошибка pprof сервера", "error", err)
		}
	}()
}

// getEnv возвращает значение переменной окружения или значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
