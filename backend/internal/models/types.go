// Package models contains common data types for the server
package models

// TorrentInfo information about a torrent
type TorrentInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Size     int64   `json:"size"`
}

// FileInfo information about a file in a torrent
type FileInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

// RoomInfo information about a P2P room
type RoomInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HostID    string `json:"hostId"`
	PeerCount int    `json:"peerCount"`
}

// SyncStatus playback synchronization status
type SyncStatus struct {
	IsPlaying bool    `json:"isPlaying"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Timestamp int64   `json:"timestamp"`
}

// P2PEvent P2P connection event
type P2PEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// AddTorrentRequest request to add a torrent
type AddTorrentRequest struct {
	MagnetURI  string `json:"magnetUri,omitempty"`
	TorrentFile string `json:"torrentFile,omitempty"` // base64 encoded torrent file content
}

// CreateRoomRequest request to create a room
type CreateRoomRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// JoinRoomRequest request to join a room
type JoinRoomRequest struct {
	RoomID   string `json:"roomId"`
	Password string `json:"password"`
}

// SignalRequest request to send a WebRTC signal
type SignalRequest struct {
	RoomID string `json:"roomId"`
	Signal []byte `json:"signal"`
}

// SeekRequest seek request
type SeekRequest struct {
	Position float64 `json:"position"`
}

// SelectFileRequest file selection request
type SelectFileRequest struct {
	FileIndex int `json:"fileIndex"`
}

// ErrorResponse error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// SuccessResponse success response
type SuccessResponse struct {
	Message string `json:"message"`
}

// ============ Auth Models ============

// User system user
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	CreatedAt    int64  `json:"createdAt"`
}

// RegisterRequest registration request
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ChangePasswordRequest password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// UserResponse is the API-safe user representation (no password hash)
type UserResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"createdAt"`
}

// ToUserResponse converts a User to UserResponse
func (u *User) ToUserResponse() UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
	}
}

// AuthResponse auth token response
type AuthResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
}

// Claims JWT token data
type Claims struct {
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	ExpiresAt int64  `json:"expiresAt"`
	JTI       string `json:"jti"`
}

// ============ Pagination Models ============

// PaginationParams pagination parameters from request
type PaginationParams struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// PaginatedResponse wrapper for paginated responses
type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	TotalCount int         `json:"totalCount"`
	Limit      int         `json:"limit"`
	Offset     int         `json:"offset"`
	HasMore    bool        `json:"hasMore"`
}

// TorrentListResponse response with torrent list and pagination
type TorrentListResponse struct {
	Torrents   []*TorrentInfo `json:"torrents"`
	TotalCount int            `json:"totalCount"`
	Limit      int            `json:"limit"`
	Offset     int            `json:"offset"`
	HasMore    bool           `json:"hasMore"`
}

// FileListResponse response with file list and pagination
type FileListResponse struct {
	Files      []FileInfo `json:"files"`
	TotalCount int        `json:"totalCount"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	HasMore    bool       `json:"hasMore"`
}

// NewPaginatedResponse creates a paginated response
func NewPaginatedResponse(data interface{}, totalCount, limit, offset int) PaginatedResponse {
	return PaginatedResponse{
		Data:       data,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}

// NewTorrentListResponse creates a torrent list response
func NewTorrentListResponse(torrents []*TorrentInfo, totalCount, limit, offset int) TorrentListResponse {
	return TorrentListResponse{
		Torrents:   torrents,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}

// NewFileListResponse creates a file list response
func NewFileListResponse(files []FileInfo, totalCount, limit, offset int) FileListResponse {
	return FileListResponse{
		Files:      files,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
		HasMore:    offset+limit < totalCount,
	}
}

// ============ Buffer Models ============

// StreamConfig streaming configuration
type StreamConfig struct {
	BufferPercent    int   `json:"buffer_percent"`     // Buffer percentage (5-20)
	BufferDuration   int   `json:"buffer_duration"`    // Buffer duration in seconds
	MaxBufferSize    int64 `json:"max_buffer_size"`    // Maximum buffer size in bytes
	PreBufferPercent int   `json:"pre_buffer_percent"` // Pre-buffer percentage
}

// BufferInfo buffer state information
type BufferInfo struct {
	TorrentID       string  `json:"torrent_id"`
	FileIndex       int     `json:"file_index"`
	CurrentPosition int64   `json:"current_position"` // Current position in bytes
	BufferStart     int64   `json:"buffer_start"`     // Buffer start
	BufferEnd       int64   `json:"buffer_end"`       // Buffer end
	BufferSize      int64   `json:"buffer_size"`      // Buffer size
	BufferedBytes   int64   `json:"buffered_bytes"`   // Bytes buffered
	BufferedPercent float64 `json:"buffered_percent"` // Buffer load percentage
	DownloadSpeed   int64   `json:"download_speed"`   // Download speed (bytes/sec)
	IsBuffering     bool    `json:"is_buffering"`     // Is buffering in progress
}

// SetBufferPositionRequest request to set buffer position
type SetBufferPositionRequest struct {
	Position int64 `json:"position"` // Position in bytes
}
