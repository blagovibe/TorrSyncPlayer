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

// AddTorrentRequest request to add a torrent
type AddTorrentRequest struct {
	MagnetURI   string `json:"magnetUri,omitempty"`
	TorrentFile string `json:"torrentFile,omitempty"` // base64 encoded torrent file content
}

// CreateRoomRequest request to create a room
type CreateRoomRequest struct {
	Name      string `json:"name"`
	Password  string `json:"password"`
	TorrentID string `json:"torrentId,omitempty"` // infohash of the torrent to bind this room to
}

// JoinRoomRequest request to join a room
type JoinRoomRequest struct {
	RoomID    string `json:"roomId"`
	Password  string `json:"password"`
	TorrentID string `json:"torrentId,omitempty"` // must match room's torrent (idea-0046)
}

// SignalRequest request to relay a room signal to peers (server-brokered over SSE)
type SignalRequest struct {
	RoomID string `json:"roomId"`
	Signal []byte `json:"signal"`
}

// SeekRequest seek request
type SeekRequest struct {
	Position float64 `json:"position"`
}

// BufferingRequest reports that a peer started or stopped buffering.
type BufferingRequest struct {
	Buffering bool `json:"buffering"`
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

// MagnetResponse carries the magnet URI of a torrent the caller holds.
type MagnetResponse struct {
	MagnetURI string `json:"magnetUri" example:"magnet:?xt=urn:btih:3b2b1a9c"`
}

// JoinRoomResponse is returned by POST /api/v1/rooms/join.
//
// The join used to answer with a bare SuccessResponse, so a client had no way
// to learn the room id from the response and had to remember what it sent —
// the only symptom being an SSE path built as /api/v1/rooms//events.
type JoinRoomResponse struct {
	Message string `json:"message"`
	RoomID  string `json:"id"`
}

// ============ Pagination Models ============

// PaginationParams pagination parameters from request
type PaginationParams struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
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
	BufferPercent  int   `json:"buffer_percent"`  // Buffer percentage (5-20)
	BufferDuration int   `json:"buffer_duration"` // Buffer duration in seconds
	MaxBufferSize  int64 `json:"max_buffer_size"` // Maximum buffer size in bytes
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
