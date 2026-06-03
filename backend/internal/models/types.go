// Package models содержит общие типы данных для сервера
package models

// TorrentInfo информация о торренте
type TorrentInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Size     int64   `json:"size"`
}

// FileInfo информация о файле в торренте
type FileInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

// RoomInfo информация о P2P комнате
type RoomInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HostID    string `json:"hostId"`
	PeerCount int    `json:"peerCount"`
}

// SyncStatus статус синхронизации воспроизведения
type SyncStatus struct {
	IsPlaying bool    `json:"isPlaying"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Timestamp int64   `json:"timestamp"`
}

// P2PEvent событие P2P соединения
type P2PEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// AddTorrentRequest запрос на добавление торрента
type AddTorrentRequest struct {
	MagnetURI string `json:"magnetUri"`
}

// CreateRoomRequest запрос на создание комнаты
type CreateRoomRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// JoinRoomRequest запрос на присоединение к комнате
type JoinRoomRequest struct {
	RoomID   string `json:"roomId"`
	Password string `json:"password"`
}

// SignalRequest запрос на отправку WebRTC сигнала
type SignalRequest struct {
	RoomID string `json:"roomId"`
	Signal []byte `json:"signal"`
}

// SeekRequest запрос на перемотку
type SeekRequest struct {
	Position float64 `json:"position"`
}

// SelectFileRequest запрос на выбор файла
type SelectFileRequest struct {
	FileIndex int `json:"fileIndex"`
}

// ErrorResponse ответ с ошибкой
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse успешный ответ
type SuccessResponse struct {
	Message string `json:"message"`
}

// ============ Auth Models ============

// User пользователь системы
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"` // не возвращаем в JSON
	CreatedAt    int64  `json:"createdAt"`
}

// RegisterRequest запрос на регистрацию
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest запрос на вход
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse ответ с токеном аутентификации
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// Claims данные JWT токена
type Claims struct {
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	ExpiresAt int64  `json:"expiresAt"`
}

// ============ Pagination Models ============

// PaginationParams параметры пагинации из запроса
type PaginationParams struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// PaginatedResponse обёртка для пагинированных ответов
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	TotalCount int         `json:"totalCount"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
	HasMore    bool        `json:"hasMore"`
}

// TorrentListResponse ответ со списком торрентов и пагинацией
type TorrentListResponse struct {
	Torrents   []*TorrentInfo `json:"torrents"`
	TotalCount int            `json:"totalCount"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
	HasMore    bool           `json:"hasMore"`
}

// FileListResponse ответ со списком файлов и пагинацией
type FileListResponse struct {
	Files      []FileInfo `json:"files"`
	TotalCount int        `json:"totalCount"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	HasMore    bool       `json:"hasMore"`
}

// NewPaginatedResponse создаёт пагинированный ответ
func NewPaginatedResponse(data interface{}, totalCount, limit, offset int) PaginatedResponse {
	return PaginatedResponse{
		Data:       data,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}

// NewTorrentListResponse создаёт ответ со списком торрентов
func NewTorrentListResponse(torrents []*TorrentInfo, totalCount, limit, offset int) TorrentListResponse {
	return TorrentListResponse{
		Torrents:   torrents,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}

// NewFileListResponse создаёт ответ со списком файлов
func NewFileListResponse(files []FileInfo, totalCount, limit, offset int) FileListResponse {
	return FileListResponse{
		Files:      files,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}
