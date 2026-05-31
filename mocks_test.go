package main

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==================== TorrentService Mock ====================

// MockTorrentService мок для TorrentServiceInterface
type MockTorrentService struct {
	mu sync.RWMutex

	// Вызовы методов для проверки
	InitCalled               bool
	InitCalledWith           context.Context
	AddTorrentByMagnetCalled bool
	AddTorrentByMagnetURI    string
	AddTorrentByFileCalled   bool
	AddTorrentByFilePath     string
	RemoveTorrentCalled      bool
	RemoveTorrentHash        string
	PauseTorrentCalled       bool
	PauseTorrentHash         string
	ResumeTorrentCalled      bool
	ResumeTorrentHash        string
	GetTorrentInfoCalled     bool
	GetTorrentInfoHash       string
	GetAllTorrentsCalled     bool
	GetStreamURLCalled       bool
	GetStreamURLHash         string
	GetStreamURLFilePath     string
	GetFileCalled            bool
	GetFileHash              string
	GetFileFilePath          string
	StartHTTPServerCalled    bool
	StopHTTPServerCalled     bool
	SetStreamPortCalled      bool
	SetStreamPortValue       int
	CloseCalled              bool

	// Возвращаемые значения
	InitErr                error
	AddTorrentByMagnetInfo *TorrentInfo
	AddTorrentByMagnetErr  error
	AddTorrentByFileInfo   *TorrentInfo
	AddTorrentByFileErr    error
	RemoveTorrentErr       error
	PauseTorrentErr        error
	ResumeTorrentErr       error
	GetTorrentInfoInfo     *TorrentInfo
	GetTorrentInfoErr      error
	GetAllTorrentsResult   []*TorrentInfo
	GetStreamURLResult     string
	GetFileInfo            *TorrentFile
	GetFileErr             error
	StartHTTPServerErr     error
	StopHTTPServerErr      error
}

func NewMockTorrentService() *MockTorrentService {
	return &MockTorrentService{}
}

func (m *MockTorrentService) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InitCalled = true
	m.InitCalledWith = ctx
	return m.InitErr
}

func (m *MockTorrentService) AddTorrentByMagnet(magnetURI string) (*TorrentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddTorrentByMagnetCalled = true
	m.AddTorrentByMagnetURI = magnetURI
	return m.AddTorrentByMagnetInfo, m.AddTorrentByMagnetErr
}

func (m *MockTorrentService) AddTorrentByFile(filePath string) (*TorrentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddTorrentByFileCalled = true
	m.AddTorrentByFilePath = filePath
	return m.AddTorrentByFileInfo, m.AddTorrentByFileErr
}

func (m *MockTorrentService) RemoveTorrent(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoveTorrentCalled = true
	m.RemoveTorrentHash = hash
	return m.RemoveTorrentErr
}

func (m *MockTorrentService) PauseTorrent(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PauseTorrentCalled = true
	m.PauseTorrentHash = hash
	return m.PauseTorrentErr
}

func (m *MockTorrentService) ResumeTorrent(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResumeTorrentCalled = true
	m.ResumeTorrentHash = hash
	return m.ResumeTorrentErr
}

func (m *MockTorrentService) GetTorrentInfo(hash string) (*TorrentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetTorrentInfoCalled = true
	m.GetTorrentInfoHash = hash
	return m.GetTorrentInfoInfo, m.GetTorrentInfoErr
}

func (m *MockTorrentService) GetAllTorrents() []*TorrentInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetAllTorrentsCalled = true
	return m.GetAllTorrentsResult
}

func (m *MockTorrentService) GetStreamURL(hash string, filePath string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetStreamURLCalled = true
	m.GetStreamURLHash = hash
	m.GetStreamURLFilePath = filePath
	return m.GetStreamURLResult
}

func (m *MockTorrentService) GetFile(hash string, filePath string) (*TorrentFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetFileCalled = true
	m.GetFileHash = hash
	m.GetFileFilePath = filePath
	return m.GetFileInfo, m.GetFileErr
}

func (m *MockTorrentService) StartHTTPServer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StartHTTPServerCalled = true
	return m.StartHTTPServerErr
}

func (m *MockTorrentService) StopHTTPServer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StopHTTPServerCalled = true
	return m.StopHTTPServerErr
}

func (m *MockTorrentService) SetStreamPort(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetStreamPortCalled = true
	m.SetStreamPortValue = port
}

func (m *MockTorrentService) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCalled = true
}

// Проверка реализации интерфейса
var _ TorrentServiceInterface = (*MockTorrentService)(nil)

// ==================== P2PService Mock ====================

// MockP2PService мок для P2PServiceInterface
type MockP2PService struct {
	mu sync.RWMutex

	// Вызовы методов
	InitCalled                   bool
	InitCalledWith               context.Context
	CreateRoomCalled             bool
	CreateRoomWithPasswordCalled bool
	CreateRoomWithPasswordValue  string
	JoinRoomCalled               bool
	JoinRoomID                   string
	JoinRoomWithPasswordCalled   bool
	JoinRoomWithPasswordID       string
	JoinRoomWithPasswordValue    string
	LeaveRoomCalled              bool
	SendMessageCalled            bool
	SendMessageMsg               P2PMessage
	BroadcastMessageCalled       bool
	BroadcastMessageMsg          P2PMessage
	GetPeersCalled               bool
	GetRoomIDCalled              bool
	IsHostCalled                 bool
	HasRoomPasswordCalled        bool
	SetRoomPasswordCalled        bool
	SetRoomPasswordValue         string
	VerifyRoomPasswordCalled     bool
	VerifyRoomPasswordValue      bool
	CloseCalled                  bool

	// Возвращаемые значения
	InitErr                   error
	CreateRoomID              string
	CreateRoomErr             error
	CreateRoomWithPasswordID  string
	CreateRoomWithPasswordErr error
	JoinRoomErr               error
	JoinRoomWithPasswordErr   error
	LeaveRoomErr              error
	SendMessageErr            error
	BroadcastMessageErr       error
	GetPeersResult            []PeerInfo
	GetRoomIDResult           string
	IsHostResult              bool
	HasRoomPasswordResult     bool
	SetRoomPasswordErr        error
	VerifyRoomPasswordResult  bool
	CloseErr                  error
}

func NewMockP2PService() *MockP2PService {
	return &MockP2PService{}
}

func (m *MockP2PService) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InitCalled = true
	m.InitCalledWith = ctx
	return m.InitErr
}

func (m *MockP2PService) CreateRoom() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateRoomCalled = true
	return m.CreateRoomID, m.CreateRoomErr
}

func (m *MockP2PService) CreateRoomWithPassword(password string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateRoomWithPasswordCalled = true
	m.CreateRoomWithPasswordValue = password
	return m.CreateRoomWithPasswordID, m.CreateRoomWithPasswordErr
}

func (m *MockP2PService) JoinRoom(roomID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.JoinRoomCalled = true
	m.JoinRoomID = roomID
	return m.JoinRoomErr
}

func (m *MockP2PService) JoinRoomWithPassword(roomID string, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.JoinRoomWithPasswordCalled = true
	m.JoinRoomWithPasswordID = roomID
	m.JoinRoomWithPasswordValue = password
	return m.JoinRoomWithPasswordErr
}

func (m *MockP2PService) LeaveRoom() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LeaveRoomCalled = true
	return m.LeaveRoomErr
}

func (m *MockP2PService) SendMessage(msg P2PMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendMessageCalled = true
	m.SendMessageMsg = msg
	return m.SendMessageErr
}

func (m *MockP2PService) BroadcastMessage(msg P2PMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BroadcastMessageCalled = true
	m.BroadcastMessageMsg = msg
	return m.BroadcastMessageErr
}

func (m *MockP2PService) GetPeers() []PeerInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetPeersCalled = true
	return m.GetPeersResult
}

func (m *MockP2PService) GetRoomID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetRoomIDCalled = true
	return m.GetRoomIDResult
}

func (m *MockP2PService) IsHost() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.IsHostCalled = true
	return m.IsHostResult
}

func (m *MockP2PService) HasRoomPassword() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HasRoomPasswordCalled = true
	return m.HasRoomPasswordResult
}

func (m *MockP2PService) SetRoomPassword(password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetRoomPasswordCalled = true
	m.SetRoomPasswordValue = password
	return m.SetRoomPasswordErr
}

func (m *MockP2PService) VerifyRoomPassword(password string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.VerifyRoomPasswordCalled = true
	return m.VerifyRoomPasswordResult
}

func (m *MockP2PService) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCalled = true
	return m.CloseErr
}

// Проверка реализации интерфейса
var _ P2PServiceInterface = (*MockP2PService)(nil)

// ==================== SyncService Mock ====================

// MockSyncService мок для SyncServiceInterface
type MockSyncService struct {
	mu sync.RWMutex

	// Вызовы методов
	InitCalled                   bool
	InitCalledWith               context.Context
	PlayCalled                   bool
	PlayPosition                 float64
	PauseCalled                  bool
	SeekCalled                   bool
	SeekPosition                 float64
	GetPlaybackStateCalled       bool
	GetSyncStatsCalled           bool
	SetLatencyCompensationCalled bool
	SetLatencyCompensationValue  int
	OnStateChangeCalled          bool
	OnStateChangeHandler         func(PlaybackState)
	OnSyncStatsCalled            bool
	OnSyncStatsHandler           func(SyncStats)
	SetP2PServiceCalled          bool
	SetP2PServiceValue           P2PServiceInterface

	// Возвращаемые значения
	InitErr                error
	PlayErr                error
	PauseErr               error
	SeekErr                error
	GetPlaybackStateResult PlaybackState
	GetSyncStatsResult     SyncStats
}

func NewMockSyncService() *MockSyncService {
	return &MockSyncService{}
}

func (m *MockSyncService) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InitCalled = true
	m.InitCalledWith = ctx
	return m.InitErr
}

func (m *MockSyncService) Play(position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PlayCalled = true
	m.PlayPosition = position
	return m.PlayErr
}

func (m *MockSyncService) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PauseCalled = true
	return m.PauseErr
}

func (m *MockSyncService) Seek(position float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SeekCalled = true
	m.SeekPosition = position
	return m.SeekErr
}

func (m *MockSyncService) GetPlaybackState() PlaybackState {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetPlaybackStateCalled = true
	return m.GetPlaybackStateResult
}

func (m *MockSyncService) GetSyncStats() SyncStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetSyncStatsCalled = true
	return m.GetSyncStatsResult
}

func (m *MockSyncService) SetLatencyCompensation(ms int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetLatencyCompensationCalled = true
	m.SetLatencyCompensationValue = ms
}

func (m *MockSyncService) OnStateChange(handler func(PlaybackState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.OnStateChangeCalled = true
	m.OnStateChangeHandler = handler
}

func (m *MockSyncService) OnSyncStats(handler func(SyncStats)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.OnSyncStatsCalled = true
	m.OnSyncStatsHandler = handler
}

func (m *MockSyncService) SetP2PService(p2p P2PServiceInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SetP2PServiceCalled = true
	m.SetP2PServiceValue = p2p
}

// Проверка реализации интерфейса
var _ SyncServiceInterface = (*MockSyncService)(nil)

// ==================== Тесты с моками ====================

func TestAppWithMockServices(t *testing.T) {
	torrentMock := NewMockTorrentService()
	p2pMock := NewMockP2PService()
	syncMock := NewMockSyncService()

	app := NewApp(torrentMock, p2pMock, syncMock)

	assert.NotNil(t, app)
	assert.Equal(t, torrentMock, app.TorrentService)
	assert.Equal(t, p2pMock, app.P2PService)
	assert.Equal(t, syncMock, app.SyncService)
}

func TestAppShutdownWithMocks(t *testing.T) {
	torrentMock := NewMockTorrentService()
	p2pMock := NewMockP2PService()
	syncMock := NewMockSyncService()

	app := NewApp(torrentMock, p2pMock, syncMock)

	// Shutdown не должен паниковать
	assert.NotPanics(t, func() {
		app.Shutdown()
	})

	// Проверяем, что методы Close были вызваны
	assert.True(t, p2pMock.CloseCalled)
	assert.True(t, torrentMock.StopHTTPServerCalled)
}

func TestMockTorrentServiceImplementsInterface(t *testing.T) {
	mock := NewMockTorrentService()
	var _ TorrentServiceInterface = mock
	assert.NotNil(t, mock)
}

func TestMockP2PServiceImplementsInterface(t *testing.T) {
	mock := NewMockP2PService()
	var _ P2PServiceInterface = mock
	assert.NotNil(t, mock)
}

func TestMockSyncServiceImplementsInterface(t *testing.T) {
	mock := NewMockSyncService()
	var _ SyncServiceInterface = mock
	assert.NotNil(t, mock)
}

func TestMockTorrentServiceInit(t *testing.T) {
	mock := NewMockTorrentService()
	ctx := context.Background()

	err := mock.Init(ctx)
	assert.NoError(t, err)
	assert.True(t, mock.InitCalled)
	assert.Equal(t, ctx, mock.InitCalledWith)
}

func TestMockP2PServiceCreateRoom(t *testing.T) {
	mock := NewMockP2PService()
	mock.CreateRoomID = "test-room-123"

	roomID, err := mock.CreateRoom()
	assert.NoError(t, err)
	assert.Equal(t, "test-room-123", roomID)
	assert.True(t, mock.CreateRoomCalled)
}

func TestMockSyncServicePlay(t *testing.T) {
	mock := NewMockSyncService()

	err := mock.Play(100.5)
	assert.NoError(t, err)
	assert.True(t, mock.PlayCalled)
	assert.Equal(t, 100.5, mock.PlayPosition)
}

func TestMockSyncServiceSetP2PService(t *testing.T) {
	syncMock := NewMockSyncService()
	p2pMock := NewMockP2PService()

	syncMock.SetP2PService(p2pMock)

	assert.True(t, syncMock.SetP2PServiceCalled)
	assert.Equal(t, p2pMock, syncMock.SetP2PServiceValue)
}
