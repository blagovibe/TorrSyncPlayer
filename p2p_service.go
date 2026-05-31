package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"torrsyncplayer/logger"

	"github.com/pion/webrtc/v4"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

// P2PService сервис для P2P соединений
type P2PService struct {
	ctx               context.Context
	cancel            context.CancelFunc
	api               *webrtc.API
	peers             map[string]*PeerData
	mu                sync.RWMutex
	isHost            bool
	roomID            string
	localPeerID       string
	heartbeatInterval time.Duration
	heartbeatStopCh   map[string]chan struct{}
	rateLimiter       map[string]*rate.Limiter
	rateMu            sync.Mutex
	roomPasswordHash  string
	passwordMu        sync.RWMutex
}

// NewP2PService создает новый P2P сервис
func NewP2PService() *P2PService {
	return &P2PService{
		peers:             make(map[string]*PeerData),
		heartbeatInterval: 2 * time.Second,
		localPeerID:       generatePeerID(),
		heartbeatStopCh:   make(map[string]chan struct{}),
		rateLimiter:       make(map[string]*rate.Limiter),
	}
}

// Init инициализирует P2P сервис
func (s *P2PService) Init(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	logger.Info("P2P Service initialized", "service", "p2p", "peer_id", s.localPeerID)
	return nil
}

// SetRoomPassword устанавливает пароль для комнаты с bcrypt хешированием
func (s *P2PService) SetRoomPassword(password string) error {
	s.passwordMu.Lock()
	defer s.passwordMu.Unlock()

	if password == "" {
		s.roomPasswordHash = ""
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	s.roomPasswordHash = string(hash)
	return nil
}

// VerifyRoomPassword проверяет пароль через bcrypt
func (s *P2PService) VerifyRoomPassword(password string) bool {
	s.passwordMu.RLock()
	defer s.passwordMu.RUnlock()

	if s.roomPasswordHash == "" {
		return true // Комната без пароля
	}
	err := bcrypt.CompareHashAndPassword([]byte(s.roomPasswordHash), []byte(password))
	return err == nil
}

// CreateRoom создает комнату (Host)
func (s *P2PService) CreateRoom() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isHost = true
	s.roomID = generateRoomID()

	logger.Info("Room created", "service", "p2p", "room_id", s.roomID, "host_id", s.localPeerID)

	// Отправляем событие во фронтенд
	s.emitEvent("room_created", map[string]interface{}{
		"roomID": s.roomID,
		"peerID": s.localPeerID,
	})

	return s.roomID, nil
}

// CreateRoomWithPassword создаёт комнату с опциональным паролем
func (s *P2PService) CreateRoomWithPassword(password string) (string, error) {
	roomID, err := s.CreateRoom()
	if err != nil {
		return "", err
	}
	if password != "" {
		if err := s.SetRoomPassword(password); err != nil {
			return "", fmt.Errorf("failed to set room password: %w", err)
		}
		logger.Info("Room created with password protection", "service", "p2p", "room_id", roomID)
	}
	return roomID, nil
}

// JoinRoom подключается к комнате (Guest)
func (s *P2PService) JoinRoom(roomID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isHost = false
	s.roomID = roomID

	logger.Info("Guest joined room", "service", "p2p", "room_id", roomID)

	// Отправляем событие во фронтенд
	s.emitEvent("room_joined", map[string]interface{}{
		"roomID": roomID,
		"peerID": s.localPeerID,
	})

	return nil
}

// JoinRoomWithPassword принимает пароль для входа в комнату
func (s *P2PService) JoinRoomWithPassword(roomID string, password string) error {
	// Проверяем пароль перед подключением
	if !s.VerifyRoomPassword(password) {
		return fmt.Errorf("invalid room password")
	}
	return s.JoinRoom(roomID)
}

// JoinRoomWithSDP подключается к комнате с SDP offer (Guest)
func (s *P2PService) JoinRoomWithSDP(roomID string, offerSDP string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isHost = false
	s.roomID = roomID

	// Создаем новое PeerConnection
	peerConnection, err := s.createPeerConnection()
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Устанавливаем удаленный SDP (offer от хоста)
	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(offerSDP), &offer); err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to parse offer SDP: %w", err)
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	// Создаем answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	// Устанавливаем local description
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	// Генерируем ID для пира
	peerID := generatePeerID()

	// Сохраняем пира
	s.peers[peerID] = &PeerData{
		PeerConnection: peerConnection,
		IsHost:         false,
		LastSeen:       time.Now(),
		Connected:      false,
	}

	// Обрабатываем ICE candidates
	s.handleICECandidates(peerConnection, peerID)

	// Сериализуем answer
	answerBytes, err := json.Marshal(answer)
	if err != nil {
		delete(s.peers, peerID)
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to marshal answer: %w", err)
	}

	logger.Info("Guest joined room", "service", "p2p", "peer_id", peerID, "room_id", roomID)

	// Отправляем событие во фронтенд
	s.emitEvent("room_joined", map[string]interface{}{
		"roomID": roomID,
		"peerID": peerID,
	})

	return string(answerBytes), nil
}

// LeaveRoom покидает комнату
func (s *P2PService) LeaveRoom() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.roomID = ""
	s.isHost = false

	// Закрываем все соединения
	for peerID, peerData := range s.peers {
		if peerData.DataChannel != nil {
			if err := peerData.DataChannel.Close(); err != nil {
				logger.Warn("Failed to close data channel", "peer_id", peerID, "error", err)
			}
		}
		if peerData.PeerConnection != nil {
			if err := peerData.PeerConnection.Close(); err != nil {
				logger.Warn("Failed to close peer connection", "peer_id", peerID, "error", err)
			}
		}
		logger.Info("Disconnected peer", "service", "p2p", "peer_id", peerID)
	}

	s.peers = make(map[string]*PeerData)
	s.emitEvent("room_left", nil)

	return nil
}

// SendMessage отправляет сообщение всем пирам
func (s *P2PService) SendMessage(msg P2PMessage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	for peerID, peerData := range s.peers {
		if peerData.DataChannel != nil && peerData.DataChannel.ReadyState() == webrtc.DataChannelStateOpen {
			if err := peerData.DataChannel.Send(msgBytes); err != nil {
				logger.Error("Failed to send message to peer", "service", "p2p", "peer_id", peerID, "error", err)
			}
		}
	}

	return nil
}

// BroadcastMessage отправляет сообщение всем пирам (алиас для SendMessage)
func (s *P2PService) BroadcastMessage(msg P2PMessage) error {
	return s.SendMessage(msg)
}

// HandleAnswer обрабатывает SDP answer
func (s *P2PService) HandleAnswer(peerID string, answerSDP string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	peerData, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Парсим answer
	var answer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(answerSDP), &answer); err != nil {
		return fmt.Errorf("failed to parse answer SDP: %w", err)
	}

	// Устанавливаем remote description
	if err := peerData.PeerConnection.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	logger.Info("Answer handled for peer", "service", "p2p", "peer_id", peerID)
	return nil
}

// CreateOffer создает SDP offer для нового гостя (вызывается хостом)
func (s *P2PService) CreateOffer(peerID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Создаем новое PeerConnection
	peerConnection, err := s.createPeerConnection()
	if err != nil {
		return "", fmt.Errorf("failed to create peer connection: %w", err)
	}

	// Создаем Data Channel
	dataChannel, err := peerConnection.CreateDataChannel("sync", nil)
	if err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to create data channel: %w", err)
	}

	// Настраиваем обработчики Data Channel
	s.setupDataChannel(dataChannel, peerID)

	// Создаем offer
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	// Устанавливаем local description
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	// Сохраняем пира
	s.peers[peerID] = &PeerData{
		PeerConnection: peerConnection,
		DataChannel:    dataChannel,
		IsHost:         true,
		LastSeen:       time.Now(),
		Connected:      false,
	}

	// Обрабатываем ICE candidates
	s.handleICECandidates(peerConnection, peerID)

	// Сериализуем offer
	offerBytes, err := json.Marshal(offer)
	if err != nil {
		delete(s.peers, peerID)
		if closeErr := peerConnection.Close(); closeErr != nil {
			logger.Warn("Failed to close peer connection", "error", closeErr)
		}
		return "", fmt.Errorf("failed to marshal offer: %w", err)
	}

	logger.Info("Offer created for peer", "service", "p2p", "peer_id", peerID)
	return string(offerBytes), nil
}

// SendMessageToPeer отправляет сообщение конкретному пиру
func (s *P2PService) SendMessageToPeer(peerID string, msg P2PMessage) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerData, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	if peerData.DataChannel == nil || peerData.DataChannel.ReadyState() != webrtc.DataChannelStateOpen {
		return fmt.Errorf("data channel for peer %s is not open", peerID)
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := peerData.DataChannel.Send(msgBytes); err != nil {
		return fmt.Errorf("failed to send message to peer %s: %w", peerID, err)
	}

	return nil
}

// GetPeers возвращает список подключенных пиров
func (s *P2PService) GetPeers() []PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]PeerInfo, 0, len(s.peers))
	for peerID, peerData := range s.peers {
		peers = append(peers, PeerInfo{
			ID:        peerID,
			IsHost:    peerData.IsHost,
			Connected: peerData.Connected,
			LastSeen:  peerData.LastSeen,
		})
	}

	return peers
}

// IsHost возвращает true если текущий пир - Host
func (s *P2PService) IsHost() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isHost
}

// GetRoomID возвращает ID комнаты
func (s *P2PService) GetRoomID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roomID
}

// GetLocalPeerID возвращает локальный ID пира
func (s *P2PService) GetLocalPeerID() string {
	return s.localPeerID
}

// HasRoomPassword возвращает true если комната защищена паролем
func (s *P2PService) HasRoomPassword() bool {
	s.passwordMu.RLock()
	defer s.passwordMu.RUnlock()
	return s.roomPasswordHash != ""
}

// Disconnect отключает все соединения
func (s *P2PService) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Останавливаем все heartbeat горутины
	for peerID, stopCh := range s.heartbeatStopCh {
		close(stopCh)
		delete(s.heartbeatStopCh, peerID)
	}

	// Закрываем все peer connections
	for peerID, peerData := range s.peers {
		if peerData.DataChannel != nil {
			if err := peerData.DataChannel.Close(); err != nil {
				logger.Warn("Failed to close data channel", "peer_id", peerID, "error", err)
			}
		}
		if peerData.PeerConnection != nil {
			if err := peerData.PeerConnection.Close(); err != nil {
				logger.Warn("Failed to close peer connection", "peer_id", peerID, "error", err)
			}
		}
		logger.Info("Disconnected peer", "service", "p2p", "peer_id", peerID)
	}

	// Очищаем карты
	s.peers = make(map[string]*PeerData)
	s.heartbeatStopCh = make(map[string]chan struct{})

	// Очищаем rate limiter для всех пиров
	s.rateMu.Lock()
	s.rateLimiter = make(map[string]*rate.Limiter)
	s.rateMu.Unlock()

	s.emitEvent("disconnected", nil)
	logger.Info("All peers disconnected", "service", "p2p")
}

// Close закрывает P2P сервис
func (s *P2PService) Close() error {
	s.Disconnect()
	if s.cancel != nil {
		s.cancel()
	}
	logger.Info("P2P Service closed", "service", "p2p")
	return nil
}

// Вспомогательные функции

// createPeerConnection создает новое PeerConnection
func (s *P2PService) createPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
			{
				URLs: []string{"stun:stun1.l.google.com:19302"},
			},
		},
	}

	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	return peerConnection, nil
}

// setupDataChannel настраивает обработчики для Data Channel
func (s *P2PService) setupDataChannel(dc *webrtc.DataChannel, peerID string) {
	dc.OnOpen(func() {
		logger.Info("Data channel opened", "service", "p2p", "peer_id", peerID)

		// Используем атомарную операцию через горутину для избежания deadlock
		go func() {
			s.mu.Lock()
			if peerData, exists := s.peers[peerID]; exists {
				peerData.Connected = true
				peerData.LastSeen = time.Now()
			}
			s.mu.Unlock()

			// Запускаем heartbeat
			s.startHeartbeat(peerID)

			s.emitEvent("peer_connected", map[string]interface{}{
				"peerID": peerID,
			})
		}()
	})

	dc.OnClose(func() {
		logger.Info("Data channel closed", "service", "p2p", "peer_id", peerID)

		// Используем атомарную операцию через горутину
		go func() {
			s.mu.Lock()
			if peerData, exists := s.peers[peerID]; exists {
				peerData.Connected = false
			}
			// Останавливаем heartbeat
			if stopCh, exists := s.heartbeatStopCh[peerID]; exists {
				close(stopCh)
				delete(s.heartbeatStopCh, peerID)
			}
			s.mu.Unlock()

			s.emitEvent("peer_disconnected", map[string]interface{}{
				"peerID": peerID,
			})
		}()
	})

	dc.OnError(func(err error) {
		logger.Error("Data channel error", "service", "p2p", "peer_id", peerID, "error", err)
		s.emitEvent("peer_error", map[string]interface{}{
			"peerID": peerID,
			"error":  err.Error(),
		})
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.handleDataChannelMessage(peerID, msg.Data)
	})
}

// handleDataChannel обрабатывает входящий Data Channel
func (s *P2PService) handleDataChannel(dc *webrtc.DataChannel, peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Если пира нет в карте, добавляем его
	if _, exists := s.peers[peerID]; !exists {
		s.peers[peerID] = &PeerData{
			IsHost:   false,
			LastSeen: time.Now(),
		}
	}

	s.peers[peerID].DataChannel = dc
	s.setupDataChannel(dc, peerID)
}

// getRateLimiter возвращает rate limiter для пира
func (s *P2PService) getRateLimiter(peerID string) *rate.Limiter {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	limiter, ok := s.rateLimiter[peerID]
	if !ok {
		// 10 сообщений в секунду с burst 20
		limiter = rate.NewLimiter(rate.Limit(10), 20)
		s.rateLimiter[peerID] = limiter
	}
	return limiter
}

// MaxMessageSize максимальный размер входящего сообщения (1MB)
const MaxMessageSize = 1 << 20 // 1 MB

// handleDataChannelMessage обрабатывает входящее сообщение
func (s *P2PService) handleDataChannelMessage(peerID string, data []byte) {
	// Проверяем rate limit
	limiter := s.getRateLimiter(peerID)
	if !limiter.Allow() {
		logger.Warn("Rate limit exceeded, dropping message", "service", "p2p", "peer_id", peerID)
		return
	}

	// Проверяем размер сообщения для предотвращения DoS
	if len(data) > MaxMessageSize {
		logger.Warn("Message exceeds max size, dropping", "service", "p2p", "peer_id", peerID, "size", len(data), "max_size", MaxMessageSize)
		return
	}

	var msg P2PMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		logger.Error("Failed to unmarshal message", "service", "p2p", "peer_id", peerID, "error", err)
		return
	}

	// Обновляем LastSeen
	s.mu.Lock()
	if peerData, exists := s.peers[peerID]; exists {
		peerData.LastSeen = time.Now()
	}
	s.mu.Unlock()

	logger.Info("Received message from peer", "service", "p2p", "peer_id", peerID, "msg_type", msg.Type)

	// Обрабатываем heartbeat - отправляем ответ в отдельной горутине
	if msg.Type == MsgHeartbeat {
		go func() {
			if err := s.SendMessageToPeer(peerID, P2PMessage{
				Type:      MsgHeartbeat,
				Timestamp: time.Now().UnixMilli(),
			}); err != nil {
				logger.Error("Failed to send heartbeat response", "service", "p2p", "peer_id", peerID, "error", err)
			}
		}()
		return
	}

	// Отправляем событие во фронтенд
	s.emitEvent("p2p_message", map[string]interface{}{
		"peerID":    peerID,
		"msgType":   msg.Type,
		"msgData":   msg.Data,
		"timestamp": msg.Timestamp,
	})
}

// handleICECandidates обрабатывает ICE candidates
func (s *P2PService) handleICECandidates(pc *webrtc.PeerConnection, peerID string) {
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		logger.Debug("ICE candidate", "service", "p2p", "peer_id", peerID, "candidate", candidate.ToJSON().Candidate)
		s.emitEvent("ice_candidate", map[string]interface{}{
			"peerID":    peerID,
			"candidate": candidate.ToJSON(),
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Info("Peer connection state changed", "service", "p2p", "peer_id", peerID, "state", state.String())

		s.mu.Lock()
		if peerData, exists := s.peers[peerID]; exists {
			switch state {
			case webrtc.PeerConnectionStateConnected:
				peerData.Connected = true
			case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
				peerData.Connected = false
			}
		}
		s.mu.Unlock()

		s.emitEvent("peer_connection_state", map[string]interface{}{
			"peerID": peerID,
			"state":  state.String(),
		})
	})

	// Обрабатываем входящие Data Channels (для хоста)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		logger.Info("Incoming data channel", "service", "p2p", "peer_id", peerID, "label", dc.Label())
		s.handleDataChannel(dc, peerID)
	})
}

// AddICECandidate добавляет ICE candidate
func (s *P2PService) AddICECandidate(peerID string, candidate webrtc.ICECandidateInit) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerData, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("peer %s not found", peerID)
	}

	if err := peerData.PeerConnection.AddICECandidate(candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	return nil
}

// startHeartbeat запускает heartbeat для пира
func (s *P2PService) startHeartbeat(peerID string) {
	stopCh := make(chan struct{})
	s.heartbeatStopCh[peerID] = stopCh

	go func() {
		ticker := time.NewTicker(s.heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				// Проверяем, жив ли пир
				s.mu.RLock()
				peerData, exists := s.peers[peerID]
				s.mu.RUnlock()

				if !exists {
					return
				}

				// Если пир не отвечал больше 10 секунд, считаем его отключенным
				if time.Since(peerData.LastSeen) > 10*time.Second {
					logger.Warn("Peer heartbeat timeout", "service", "p2p", "peer_id", peerID)
					s.mu.Lock()
					peerData.Connected = false
					s.mu.Unlock()

					s.emitEvent("peer_timeout", map[string]interface{}{
						"peerID": peerID,
					})
					return
				}

				// Отправляем heartbeat
				if err := s.SendMessageToPeer(peerID, P2PMessage{
					Type:      MsgHeartbeat,
					Timestamp: time.Now().UnixMilli(),
				}); err != nil {
					logger.Error("Failed to send heartbeat", "service", "p2p", "peer_id", peerID, "error", err)
				}
			}
		}
	}()
}

// emitEvent отправляет событие во фронтенд через Wails Events
func (s *P2PService) emitEvent(eventName string, data interface{}) {
	if s.ctx == nil {
		return
	}

	select {
	case <-s.ctx.Done():
		return
	default:
		runtime.EventsEmit(s.ctx, "p2p:"+eventName, data)
	}
}

// generatePeerID генерирует уникальный ID пира
func generatePeerID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// generateRoomID генерирует уникальный ID комнаты (8 байт = 16 hex символов)
func generateRoomID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Play отправляет команду воспроизведения
func (s *P2PService) Play(timestamp float64) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgPlay,
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"timestamp": timestamp,
		},
	})
}

// Pause отправляет команду паузы
func (s *P2PService) Pause(timestamp float64) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgPause,
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"timestamp": timestamp,
		},
	})
}

// Seek отправляет команду перемотки
func (s *P2PService) Seek(position float64) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgSeek,
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": position,
		},
	})
}

// SendChatMessage отправляет чат-сообщение
func (s *P2PService) SendChatMessage(text string) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgChat,
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"text":      text,
			"from":      s.localPeerID,
			"timestamp": time.Now().UnixMilli(),
		},
	})
}

// SendTorrentInfo отправляет информацию о торренте
func (s *P2PService) SendTorrentInfo(info interface{}) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgTorrentInfo,
		Timestamp: time.Now().UnixMilli(),
		Data:      info,
	})
}

// SendState отправляет текущее состояние воспроизведения
func (s *P2PService) SendState(state interface{}) error {
	return s.SendMessage(P2PMessage{
		Type:      MsgState,
		Timestamp: time.Now().UnixMilli(),
		Data:      state,
	})
}

// Проверка реализации интерфейса
var _ P2PServiceInterface = (*P2PService)(nil)
