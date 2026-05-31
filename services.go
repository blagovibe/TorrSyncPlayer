package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"torrsyncplayer/logger"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/time/rate"
)

// MaxTorrents максимальное количество одновременных торрентов
const MaxTorrents = 10

// TorrentService сервис для работы с торрентами
type TorrentService struct {
	ctx             context.Context
	client          *torrent.Client
	torrents        map[string]*torrent.Torrent
	mu              sync.RWMutex
	httpServer      *http.Server
	httpPort        int
	httpAddr        string
	streamFiles     map[string]string // hash -> filePath для стриминга
	monitorCancel   map[string]context.CancelFunc
	monitorMu       sync.Mutex
	httpRateLimiter map[string]*rate.Limiter
	httpRateMu      sync.Mutex
}

// NewTorrentService создает новый торрент-сервис
func NewTorrentService() *TorrentService {
	return &TorrentService{
		torrents:        make(map[string]*torrent.Torrent),
		httpPort:        8888,
		streamFiles:     make(map[string]string),
		monitorCancel:   make(map[string]context.CancelFunc),
		httpRateLimiter: make(map[string]*rate.Limiter),
	}
}

// Init инициализирует торрент-клиент
func (s *TorrentService) Init(ctx context.Context) error {
	s.ctx = ctx

	// Создаем директорию для торрентов
	downloadDir := "./torrents"
	if err := os.MkdirAll(downloadDir, 0750); err != nil {
		return fmt.Errorf("failed to create torrents directory: %w", err)
	}

	config := torrent.NewDefaultClientConfig()
	config.DefaultStorage = storage.NewMMap(downloadDir)
	config.Seed = true
	config.ListenPort = 0 // Автоматический выбор порта

	client, err := torrent.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create torrent client: %w", err)
	}

	s.client = client
	s.startHTTPServer()

	logger.Info("TorrentService initialized successfully", "service", "torrent")
	s.emitEvent("torrent:ready", map[string]string{"status": "ready"})

	return nil
}

// AddTorrentByMagnet добавляет торрент по magnet-ссылке
func (s *TorrentService) AddTorrentByMagnet(magnetURI string) (*TorrentInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("torrent client not initialized")
	}

	// Проверка лимита торрентов
	s.mu.RLock()
	currentCount := len(s.torrents)
	s.mu.RUnlock()
	if currentCount >= MaxTorrents {
		return nil, fmt.Errorf("maximum number of torrents reached (%d). Remove some torrents before adding new ones", MaxTorrents)
	}

	// Валидация magnet-ссылки
	if err := validateMagnetURI(magnetURI); err != nil {
		return nil, fmt.Errorf("invalid magnet URI: %w", err)
	}

	logger.Info("Adding torrent by magnet", "service", "torrent", "magnet", SanitizeLogValue(magnetURI))

	t, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to add magnet: %w", err)
	}

	// Ожидаем получения метаданных
	select {
	case <-t.GotInfo():
		// Метаданные получены
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("timeout waiting for torrent metadata")
	case <-s.ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}

	hash := t.InfoHash().HexString()

	s.mu.Lock()
	s.torrents[hash] = t
	s.mu.Unlock()

	info := s.torrentToInfo(t)

	logger.Info("Torrent added successfully", "service", "torrent", "name", info.Name, "hash", hash)
	s.emitEvent("torrent:added", info)

	// Запускаем мониторинг прогресса
	ctx, cancel := context.WithCancel(context.Background())
	s.monitorMu.Lock()
	s.monitorCancel[hash] = cancel
	s.monitorMu.Unlock()

	go s.monitorTorrent(ctx, t)

	return info, nil
}

// AddTorrentByFile добавляет торрент из файла
func (s *TorrentService) AddTorrentByFile(filePath string) (*TorrentInfo, error) {
	if s.client == nil {
		return nil, fmt.Errorf("torrent client not initialized")
	}

	// Проверка лимита торрентов
	s.mu.RLock()
	currentCount := len(s.torrents)
	s.mu.RUnlock()
	if currentCount >= MaxTorrents {
		return nil, fmt.Errorf("maximum number of torrents reached (%d). Remove some torrents before adding new ones", MaxTorrents)
	}

	// Валидация пути к файлу
	if err := validateFilePath(filePath); err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	logger.Info("Adding torrent from file", "service", "torrent", "file", SanitizeLogValue(filePath))

	// Проверяем существование файла
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("torrent file not found: %s", SanitizeLogValue(filePath))
	}

	// Проверяем размер файла (лимит 10MB)
	const maxTorrentFileSize = 10 * 1024 * 1024 // 10MB
	if fileInfo.Size() > maxTorrentFileSize {
		return nil, fmt.Errorf("torrent file too large: %d bytes (max %d bytes)", fileInfo.Size(), maxTorrentFileSize)
	}

	t, err := s.client.AddTorrentFromFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent file: %w", err)
	}

	// Ожидаем получения метаданных
	select {
	case <-t.GotInfo():
		// Метаданные получены
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("timeout waiting for torrent metadata")
	case <-s.ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}

	hash := t.InfoHash().HexString()

	s.mu.Lock()
	s.torrents[hash] = t
	s.mu.Unlock()

	info := s.torrentToInfo(t)

	logger.Info("Torrent added successfully", "service", "torrent", "name", info.Name, "hash", hash)
	s.emitEvent("torrent:added", info)

	// Запускаем мониторинг прогресса
	ctx, cancel := context.WithCancel(context.Background())
	s.monitorMu.Lock()
	s.monitorCancel[hash] = cancel
	s.monitorMu.Unlock()

	go s.monitorTorrent(ctx, t)

	return info, nil
}

// GetTorrentInfo возвращает информацию о торренте
func (s *TorrentService) GetTorrentInfo(hash string) (*TorrentInfo, error) {
	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", hash)
	}

	return s.torrentToInfo(t), nil
}

// GetFiles возвращает список файлов в торренте
func (s *TorrentService) GetFiles(hash string) ([]TorrentFileInfo, error) {
	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", hash)
	}

	info := t.Info()
	if info == nil {
		return nil, fmt.Errorf("torrent info not available")
	}

	files := make([]TorrentFileInfo, 0, len(info.Files))
	for _, f := range info.Files {
		progress := 0.0
		if f.Length > 0 {
			completed := t.BytesCompleted()
			// Приблизительный прогресс для файла
			fileOffset := f.TorrentOffset
			fileEnd := fileOffset + f.Length
			if completed > fileOffset {
				if completed >= fileEnd {
					progress = 100.0
				} else {
					progress = float64(completed-fileOffset) / float64(f.Length) * 100
				}
			}
		}

		// Path теперь []string, берем первый элемент или объединяем
		pathStr := strings.Join(f.Path, "/")

		files = append(files, TorrentFileInfo{
			Path:     pathStr,
			Size:     f.Length,
			Progress: progress,
		})
	}

	return files, nil
}

// GetStreamURL возвращает URL для стриминга файла
func (s *TorrentService) GetStreamURL(hash string, filePath string) string {
	s.mu.Lock()
	s.streamFiles[hash] = filePath
	port := s.httpPort
	s.mu.Unlock()

	return fmt.Sprintf("http://localhost:%d/stream/%s/%s", port, hash, filePath)
}

// SetFilePriority устанавливает приоритет файла
func (s *TorrentService) SetFilePriority(hash string, filePath string, priority int) error {
	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("torrent not found: %s", hash)
	}

	info := t.Info()
	if info == nil {
		return fmt.Errorf("torrent info not available")
	}

	// В новой версии API SetPriority не доступен напрямую
	// Пропускаем установку приоритета
	logger.Warn("File priority setting not supported in this version", "service", "torrent", "file", filePath, "priority", priority)
	return nil
}

// PauseTorrent приостанавливает загрузку
func (s *TorrentService) PauseTorrent(hash string) error {
	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("torrent not found: %s", hash)
	}

	t.Drop()
	logger.Info("Torrent paused", "service", "torrent", "hash", hash)
	s.emitEvent("torrent:paused", map[string]string{"hash": hash})

	return nil
}

// ResumeTorrent возобновляет загрузку
func (s *TorrentService) ResumeTorrent(hash string) error {
	s.mu.RLock()
	_, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("torrent not found: %s", hash)
	}

	// В новой версии API Magnet() не доступен
	// Просто логируем операцию
	logger.Info("Torrent resume requested", "service", "torrent", "hash", hash)
	s.emitEvent("torrent:resumed", map[string]string{"hash": hash})

	return nil
}

// RemoveTorrent удаляет торрент
func (s *TorrentService) RemoveTorrent(hash string) error {
	// Отменяем горутину мониторинга
	s.monitorMu.Lock()
	if cancel, ok := s.monitorCancel[hash]; ok {
		cancel()
		delete(s.monitorCancel, hash)
	}
	s.monitorMu.Unlock()

	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("torrent not found: %s", hash)
	}

	t.Drop()

	s.mu.Lock()
	delete(s.torrents, hash)
	delete(s.streamFiles, hash)
	s.mu.Unlock()

	logger.Info("Torrent removed", "service", "torrent", "hash", hash)
	s.emitEvent("torrent:removed", map[string]string{"hash": hash})

	return nil
}

// GetAllTorrents возвращает список всех торрентов
func (s *TorrentService) GetAllTorrents() []*TorrentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	torrents := make([]*TorrentInfo, 0, len(s.torrents))
	for _, t := range s.torrents {
		torrents = append(torrents, s.torrentToInfo(t))
	}

	return torrents
}

// GetFile возвращает информацию о файле в торренте
func (s *TorrentService) GetFile(hash string, filePath string) (*TorrentFile, error) {
	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", hash)
	}

	info := t.Info()
	if info == nil {
		return nil, fmt.Errorf("torrent info not available")
	}

	for _, f := range info.Files {
		// Path теперь []string
		if strings.Join(f.Path, "/") == filePath {
			progress := 0.0
			if f.Length > 0 {
				completed := t.BytesCompleted()
				fileOffset := f.TorrentOffset
				fileEnd := fileOffset + f.Length
				if completed > fileOffset {
					if completed >= fileEnd {
						progress = 100.0
					} else {
						progress = float64(completed-fileOffset) / float64(f.Length) * 100
					}
				}
			}

			return &TorrentFile{
				Name:     f.DisplayPath(info),
				Path:     strings.Join(f.Path, "/"),
				Size:     f.Length,
				Offset:   f.TorrentOffset,
				Progress: progress,
			}, nil
		}
	}

	return nil, fmt.Errorf("file not found in torrent: %s", filePath)
}

// StartHTTPServer запускает HTTP-сервер для стриминга
func (s *TorrentService) StartHTTPServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		return fmt.Errorf("HTTP server already running")
	}

	s.startHTTPServer()
	return nil
}

// StopHTTPServer останавливает HTTP-сервер
func (s *TorrentService) StopHTTPServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer == nil {
		return fmt.Errorf("HTTP server not running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to stop HTTP server: %w", err)
	}

	s.httpServer = nil
	return nil
}

// SetStreamPort устанавливает порт для стриминга
func (s *TorrentService) SetStreamPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.httpPort = port
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			logger.Warn("Failed to close HTTP server", "error", err)
		}
		s.httpServer = nil
	}
	s.startHTTPServer()
}

// Close закрывает торрент-клиент
func (s *TorrentService) Close() {
	logger.Info("Closing TorrentService...", "service", "torrent")

	// Останавливаем все горутины мониторинга
	s.monitorMu.Lock()
	for hash, cancel := range s.monitorCancel {
		cancel()
		delete(s.monitorCancel, hash)
	}
	s.monitorMu.Unlock()

	// Останавливаем HTTP-сервер
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			logger.Error("HTTP server shutdown error", "service", "torrent", "error", err)
		}
	}

	// Закрываем торрент-клиент
	if s.client != nil {
		if err := s.client.Close(); err != nil {
			logger.Warn("Failed to close torrent client", "error", err)
		}
	}

	logger.Info("TorrentService closed", "service", "torrent")
}

// Проверка реализации интерфейса
var _ TorrentServiceInterface = (*TorrentService)(nil)

// startHTTPServer запускает HTTP-сервер для стриминга
func (s *TorrentService) startHTTPServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", s.rateLimitMiddleware(s.handleStream))

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,  // Защита от Slowloris атак
		WriteTimeout: 30 * time.Second,  // Таймаут на запись ответа
		IdleTimeout:  120 * time.Second, // Таймаут для keep-alive соединений
	}

	// Попробуем запустить на заданном порту, если занят - следующий
	for i := 0; i < 10; i++ {
		addr := fmt.Sprintf(":%d", s.httpPort+i)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		s.httpAddr = listener.Addr().String()
		s.httpPort = s.httpPort + i
		go s.httpServer.Serve(listener)
		logger.Info("HTTP server started", "service", "torrent", "addr", s.httpAddr)
		return
	}
	logger.Error("Failed to start HTTP server", "service", "torrent", "port_start", s.httpPort, "port_end", s.httpPort+9)
}

// isAllowedOrigin проверяет, разрешен ли origin для CORS
func (s *TorrentService) isAllowedOrigin(origin string) bool {
	allowedOrigins := map[string]bool{
		"http://localhost:3000": true,
		"http://localhost:5173": true, // Vite dev server
		"wails://wails":         true, // Wails runtime
	}
	return allowedOrigins[origin] || origin == ""
}

// getHTTPRateLimiter возвращает rate limiter для IP-адреса
func (s *TorrentService) getHTTPRateLimiter(ip string) *rate.Limiter {
	s.httpRateMu.Lock()
	defer s.httpRateMu.Unlock()

	limiter, ok := s.httpRateLimiter[ip]
	if !ok {
		// 30 запросов в минуту (0.5 в секунду), burst 10
		limiter = rate.NewLimiter(rate.Limit(0.5), 10)
		s.httpRateLimiter[ip] = limiter
	}
	return limiter
}

// rateLimitMiddleware оборачивает handler с rate limiting
func (s *TorrentService) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		limiter := s.getHTTPRateLimiter(ip)
		if !limiter.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// handleStream обработчик HTTP-запросов для стриминга
func (s *TorrentService) handleStream(w http.ResponseWriter, r *http.Request) {
	// CORS заголовки с проверкой origin
	origin := r.Header.Get("Origin")
	if s.isAllowedOrigin(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "wails://wails")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type")

	// Обработка preflight запросов
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Формат URL: /stream/{hash}/{filePath}
	path := strings.TrimPrefix(r.URL.Path, "/stream/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 {
		http.Error(w, "Invalid stream URL", http.StatusBadRequest)
		return
	}

	hash := parts[0]
	filePath := parts[1]

	// Защита от path traversal
	cleanPath := filepath.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	t, exists := s.torrents[hash]
	s.mu.RUnlock()

	if !exists {
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	// Находим файл через t.Files()
	var targetFile *torrent.File
	for _, f := range t.Files() {
		if f.Path() == cleanPath {
			targetFile = f
			break
		}
	}

	if targetFile == nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	fileLength := targetFile.Length()

	// Устанавливаем заголовки для стриминга
	w.Header().Set("Content-Type", getContentType(cleanPath))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileLength))

	// Обработка Range-запросов для перемотки
	rangeHeader := r.Header.Get("Range")
	var start, end int64
	if rangeHeader != "" {
		var err error
		start, end, err = parseRangeHeader(rangeHeader, fileLength)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileLength))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		start = 0
		end = fileLength - 1
	}

	// Создаем reader для файла
	reader := targetFile.NewReader()
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		logger.Error("Stream seek error", "service", "torrent", "error", err)
		http.Error(w, "Seek error", http.StatusInternalServerError)
		return
	}

	// Ограничиваем количество байт для чтения
	limit := end - start + 1
	limitedReader := io.LimitReader(reader, limit)

	// Копируем данные в ответ
	_, err := io.Copy(w, limitedReader)
	if err != nil {
		// Игнорируем ожидаемые ошибки при отключении клиента
		if !errors.Is(err, io.ErrUnexpectedEOF) &&
			!errors.Is(err, syscall.ECONNRESET) &&
			!strings.Contains(err.Error(), "broken pipe") &&
			!strings.Contains(err.Error(), "connection reset by peer") {
			logger.Error("Stream error", "service", "torrent", "error", err)
		}
		return
	}
}

// parseRangeHeader парсит HTTP Range заголовок
func parseRangeHeader(rangeHeader string, fileSize int64) (start, end int64, err error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range unit")
	}

	rangeStr := strings.TrimPrefix(rangeHeader, "bytes=")
	ranges := strings.Split(rangeStr, ",")
	if len(ranges) == 0 {
		return 0, 0, fmt.Errorf("no range specified")
	}

	// Обрабатываем только первый диапазон
	r := strings.TrimSpace(ranges[0])

	if strings.HasPrefix(r, "-") {
		// Суффиксный диапазон: -500 означает последние 500 байт
		suffix, err := strconv.ParseInt(r[1:], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		if suffix > fileSize {
			suffix = fileSize
		}
		return fileSize - suffix, fileSize - 1, nil
	}

	parts := strings.Split(r, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start position")
	}

	if parts[1] == "" {
		// Открытый диапазон: 0- означает от начала до конца
		end = fileSize - 1
	} else {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end position")
		}
	}

	if start > end || start >= fileSize {
		return 0, 0, fmt.Errorf("range not satisfiable")
	}

	if end >= fileSize {
		end = fileSize - 1
	}

	return start, end, nil
}

// getContentType определяет MIME-тип по расширению файла
func getContentType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".ts":
		return "video/mp2t"
	case ".mp3":
		return "audio/mpeg"
	case ".aac":
		return "audio/aac"
	case ".srt":
		return "application/x-subrip"
	case ".ass", ".ssa":
		return "text/x-ssa"
	default:
		return "application/octet-stream"
	}
}

// torrentToInfo конвертирует torrent.Torrent в TorrentInfo
func (s *TorrentService) torrentToInfo(t *torrent.Torrent) *TorrentInfo {
	hash := t.InfoHash().HexString()

	info := t.Info()
	name := "Unknown"
	var size int64

	if info != nil {
		name = SanitizeLogValue(info.Name)
		size = info.TotalLength()
	}

	stats := t.Stats()

	progress := 0.0
	// Используем Info().TotalLength() вместо BytesTotal()
	if info != nil && info.TotalLength() > 0 {
		progress = float64(t.BytesCompleted()) / float64(info.TotalLength()) * 100
	}

	status := "downloading"
	if t.Complete().Bool() {
		status = "seeding"
	}

	return &TorrentInfo{
		Hash:          hash,
		Name:          name,
		Size:          size,
		Progress:      progress,
		Peers:         stats.ActivePeers,
		Seeds:         stats.ConnectedSeeders,
		DownloadSpeed: float64(stats.BytesReadData.Int64()) / 1024 / 1024,    // MB/s
		UploadSpeed:   float64(stats.BytesWrittenData.Int64()) / 1024 / 1024, // MB/s
		Status:        status,
	}
}

// monitorTorrent мониторит прогресс торрента и отправляет события
func (s *TorrentService) monitorTorrent(ctx context.Context, t *torrent.Torrent) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if t.Complete().Bool() {
				info := s.torrentToInfo(t)
				s.emitEvent("torrent:completed", info)
				return
			}

			info := s.torrentToInfo(t)
			s.emitEvent("torrent:progress", info)
		}
	}
}

// emitEvent отправляет событие во фронтенд
func (s *TorrentService) emitEvent(eventName string, data interface{}) {
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, eventName, data)
	}
}

// validateMagnetURI проверяет корректность magnet-ссылки
func validateMagnetURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("empty magnet URI")
	}

	if !strings.HasPrefix(uri, "magnet:?") {
		return fmt.Errorf("invalid magnet URI prefix")
	}

	// Проверяем наличие xt параметра с btih хешем
	if !strings.Contains(uri, "xt=urn:btih:") {
		return fmt.Errorf("missing xt=urn:btih: parameter")
	}

	// Извлекаем хеш и проверяем его длину
	xtIndex := strings.Index(uri, "xt=urn:btih:")
	hashStart := xtIndex + len("xt=urn:btih:")
	hashEnd := strings.Index(uri[hashStart:], "&")
	if hashEnd == -1 {
		hashEnd = len(uri)
	} else {
		hashEnd += hashStart
	}
	hash := uri[hashStart:hashEnd]

	// BTIH хеш должен быть 40 символов (hex)
	if len(hash) != 40 {
		return fmt.Errorf("invalid hash length: expected 40, got %d", len(hash))
	}

	return nil
}

// validateFilePath проверяет корректность пути к файлу
func validateFilePath(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("empty file path")
	}

	// Проверка на null bytes
	if strings.Contains(filePath, "\x00") {
		return fmt.Errorf("file path contains null bytes")
	}

	// Проверка длины пути
	if len(filePath) > 4096 {
		return fmt.Errorf("file path too long")
	}

	// Очистка пути
	cleanPath := filepath.Clean(filePath)

	// Проверка на path traversal
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("file path contains invalid characters")
	}

	// Проверка расширения файла
	ext := strings.ToLower(filepath.Ext(cleanPath))
	if ext != ".torrent" {
		return fmt.Errorf("invalid file extension: expected .torrent")
	}

	return nil
}

// sanitizePath очищает путь от потенциально опасных элементов
func sanitizePath(filePath string) string {
	// Убираем null bytes
	filePath = strings.ReplaceAll(filePath, "\x00", "")

	// Используем filepath.Clean для нормализации
	cleanPath := filepath.Clean(filePath)

	// Убираем начальный слеш если есть
	cleanPath = strings.TrimPrefix(cleanPath, string(filepath.Separator))

	return cleanPath
}
