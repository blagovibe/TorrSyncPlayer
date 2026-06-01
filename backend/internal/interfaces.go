// Package internal содержит интерфейсы для сервисов приложения.
// Интерфейсы определяют контракты между слоями приложения.
package internal

import (
	"context"
	"net/http"

	"github.com/yourname/torrplayer/backend/internal/models"
)

// TorrentService интерфейс для работы с торрентами.
// Определяет методы для добавления, удаления и стриминга торрентов.
type TorrentService interface {
	// AddMagnet добавляет торрент по magnet-ссылке.
	// Параметр ctx - контекст для отмены операции.
	// Параметр magnetURI - magnet-ссылка на торрент.
	// Возвращает информацию о торренте или ошибку.
	AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error)
	// RemoveTorrent удаляет торрент по ID.
	// Возвращает ошибку если торрент не найден.
	RemoveTorrent(id string) error
	// ListTorrents возвращает список всех торрентов.
	// Возвращает массив с информацией о каждом торренте.
	ListTorrents() []*models.TorrentInfo
	// GetFiles возвращает список файлов торрента.
	// Возвращает ошибку если торрент не найден.
	GetFiles(torrentID string) ([]models.FileInfo, error)
	// SelectFile выбирает файл для стриминга.
	// Устанавливает приоритет загрузки для выбранного файла.
	SelectFile(torrentID string, fileIndex int) error
	// ServeFile обрабатывает HTTP стриминг файла.
	// Поддерживает Range запросы для перемотки.
	ServeFile(w http.ResponseWriter, r *http.Request, torrentID string)
	// Close закрывает сервис торрентов.
	// Останавливает торрент-клиент и освобождает ресурсы.
	Close() error
}

// P2PService интерфейс для P2P соединений через WebRTC.
// Определяет методы для управления комнатами и пирами.
// Поддерживает JWT аутентификацию пиров.
type P2PService interface {
	// CreateRoom создаёт новую комнату.
	// Параметр name - название комнаты.
	// Параметр password - опциональный пароль.
	// Возвращает информацию о созданной комнате.
	CreateRoom(name, password string) (*models.RoomInfo, error)
	// JoinRoom присоединяет к комнате.
	// Возвращает ошибку если комната не найдена или неверный пароль.
	JoinRoom(roomID, password string) error
	// JoinRoomWithToken присоединяет к комнате с JWT аутентификацией.
	// Параметр roomID - идентификатор комнаты.
	// Параметр password - пароль для входа.
	// Параметр token - JWT токен для аутентификации.
	JoinRoomWithToken(roomID, password, token string) error
	// AuthenticatePeer аутентифицирует пира по JWT токену.
	// Используется для отложенной аутентификации после установки соединения.
	AuthenticatePeer(peerID, token string) error
	// SetLocalUserID устанавливает ID текущего пользователя для аутентификации.
	SetLocalUserID(userID string)
	// LeaveRoom выходит из комнаты.
	// Закрывает WebRTC подключение.
	LeaveRoom() error
	// SendSignal отправляет WebRTC сигнал через data channel.
	// Параметр signal - бинарные данные сигнала.
	SendSignal(signal []byte) error
	// GetEvents возвращает канал для получения P2P событий.
	// Канал буферизован и закрывается при закрытии сервиса.
	GetEvents() chan models.P2PEvent
	// RoomEventsHandler возвращает HTTP обработчик для SSE событий.
	// Использует Server-Sent Events для доставки событий в реальном времени.
	RoomEventsHandler() http.HandlerFunc
	// GetRoomInfo возвращает информацию о текущей комнате.
	// Возвращает ошибку если не подключены к комнате.
	GetRoomInfo() (*models.RoomInfo, error)
	// Close закрывает P2P сервис.
	// Закрывает все подключения и освобождает ресурсы.
	Close() error
}

// SyncService интерфейс для синхронизации воспроизведения.
// Определяет методы для управления состоянием воспроизведения.
type SyncService interface {
	// Play запускает воспроизведение.
	// Возвращает текущий статус синхронизации.
	Play() models.SyncStatus
	// Pause приостанавливает воспроизведение.
	// Возвращает текущий статус синхронизации.
	Pause() models.SyncStatus
	// Seek выполняет перемотку на указанную позицию.
	// Параметр position - позиция в секундах.
	// Возвращает ошибку если позиция некорректна.
	Seek(position float64) (models.SyncStatus, error)
	// GetStatus возвращает текущий статус воспроизведения.
	// Включает позицию, длительность и состояние.
	GetStatus() models.SyncStatus
	// SetDuration устанавливает длительность медиафайла.
	// Параметр duration - длительность в секундах.
	SetDuration(duration float64) error
	// SyncWithLatency синхронизирует воспроизведение с учётом задержки сети.
	// Параметр peerStatus - статус удалённого пира.
	// Параметр latencyMs - задержка сети в миллисекундах.
	SyncWithLatency(peerStatus models.SyncStatus, latencyMs int) models.SyncStatus
	// UpdatePosition обновляет текущую позицию.
	// Вызывается локальным плеером при изменении позиции.
	UpdatePosition(position float64) error
	// Close закрывает сервис синхронизации.
	// Останавливает воспроизведение.
	Close()
}
