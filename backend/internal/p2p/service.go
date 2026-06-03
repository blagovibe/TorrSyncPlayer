// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p предоставляет сервис для P2P соединений через WebRTC.
// Управляет комнатами, пирами и событиями в потокобезопасном режиме.
// Поддерживает JWT аутентификацию пиров при подключении.
package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/pion/webrtc/v4"
	"golang.org/x/crypto/bcrypt"
)

// Константы P2P сервиса
const (
	// eventChannelSize размер буфера канала событий
	eventChannelSize = constants.P2PEventChannelSize

	// peerIDLength длина идентификатора пира в байтах
	peerIDLength = constants.PeerIDLength
)

// Peer представляет подключённый пир в P2P сети.
// Содержит информацию о соединении, аутентификации и последнем heartbeat.
type Peer struct {
	ID            string
	UserID        string // ID аутентифицированного пользователя
	Username      string // Имя пользователя
	Connection    *webrtc.PeerConnection
	DataChannel   *webrtc.DataChannel
	LastHeartbeat time.Time
	Authenticated bool // Флаг успешной аутентификации
}

// Room представляет P2P комнату для синхронизации воспроизведения.
// Содержит список подключённых пиров и опциональный пароль.
type Room struct {
	ID          string
	Name        string
	HostID      string
	HostUserID  string // ID пользователя-владельца комнаты
	Password    string // bcrypt-хеш пароля
	Peers       map[string]*Peer
	CreatedAt   time.Time
	RequireAuth bool // Требовать JWT аутентификацию для входа
}

// DataChannelEvent представляет событие создания нового DataChannel.
// Используется для безопасной передачи DataChannel из потока WebRTC в основную горутину.
type DataChannelEvent struct {
	PeerID      string
	DataChannel *webrtc.DataChannel
}

// Service сервис P2P соединений через WebRTC.
// Управляет комнатами, пирами и событиями в потокобезопасном режиме.
type Service struct {
	mu              sync.RWMutex
	rooms           map[string]*Room
	peers           map[string]*Peer
	eventChan       chan models.P2PEvent
	dataChannelChan chan DataChannelEvent // Канал для безопасной передачи DataChannel
	api             *webrtc.API
	config          webrtc.Configuration // Конфигурация ICE с STUN серверами
	currentRoom     string
	localPeerID     string
	localUserID     string            // ID текущего пользователя
	closeChan       chan struct{}     // Канал для сигнализации о закрытии
	authService     *auth.AuthService // Сервис аутентификации для валидации JWT
}

// NewService создаёт новый P2P сервис.
// Инициализирует WebRTC API с STUN серверами для NAT traversal.
// Возвращает инициализированный сервис или ошибку если не удалось создать API.
func NewService(authService *auth.AuthService) (*Service, error) {
	// Конфигурация ICE с STUN серверами для преодоления NAT
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		},
	}

	// Создаём WebRTC API с настройками
	settingEngine := webrtc.SettingEngine{}
	webrtcAPI := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
	)

	service := &Service{
		rooms:           make(map[string]*Room),
		peers:           make(map[string]*Peer),
		eventChan:       make(chan models.P2PEvent, eventChannelSize),
		dataChannelChan: make(chan DataChannelEvent, eventChannelSize),
		api:             webrtcAPI,
		config:          config,
		localPeerID:     generateID(),
		closeChan:       make(chan struct{}),
	}

	// Запускаем обработчик событий DataChannel в отдельной горутине
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("P2P: горутина handleDataChannelEvents завершилась с паникой", "error", r)
			}
		}()
		service.handleDataChannelEvents()
	}()

	logger.Info("P2P сервис инициализирован",
		"localPeerID", service.localPeerID,
		"stun_servers", len(config.ICEServers[0].URLs),
	)
	return service, nil
}

// SetLocalUserID устанавливает ID текущего пользователя для аутентификации
func (s *Service) SetLocalUserID(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localUserID = userID
	logger.Info("P2P: установлен ID пользователя", "userID", userID)
}

// CreateRoom создаёт новую комнату для синхронизации.
// Параметр name - название комнаты.
// Параметр password - опциональный пароль (будет хеширован bcrypt).
// Возвращает информацию о созданной комнате или ошибку.
func (s *Service) CreateRoom(name, password string) (*models.RoomInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID := generateID()

	// Хешируем пароль если указан
	var passwordHash string
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), constants.BcryptCost)
		if err != nil {
			logger.Error("P2P: ошибка хеширования пароля", "error", err, "roomID", roomID)
			return nil, fmt.Errorf("ошибка хеширования пароля: %w", err)
		}
		passwordHash = string(hash)
	}

	room := &Room{
		ID:          roomID,
		Name:        name,
		HostID:      s.localPeerID,
		HostUserID:  s.localUserID,
		Password:    passwordHash,
		Peers:       make(map[string]*Peer),
		CreatedAt:   time.Now(),
		RequireAuth: constants.P2PDefaultRoomAuth, // По умолчанию требуем аутентификацию
	}

	s.rooms[roomID] = room
	s.currentRoom = roomID

	logger.Info("P2P: комната создана",
		"roomID", roomID,
		"name", name,
		"hostID", s.localPeerID,
		"hostUserID", s.localUserID,
		"hasPassword", password != "",
	)

	s.emitEvent("room_created", room)

	return &models.RoomInfo{
		ID:        room.ID,
		Name:      room.Name,
		HostID:    room.HostID,
		PeerCount: len(room.Peers),
	}, nil
}

// JoinRoom присоединяет к существующей комнате.
// Параметр roomID - идентификатор комнаты.
// Параметр password - пароль для входа (если установлен).
// Параметр token - JWT токен для аутентификации пира.
// Возвращает ошибку если комната не найдена, неверный пароль или токен.
func (s *Service) JoinRoom(roomID, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: комната не найдена", "roomID", roomID)
		return errors.NotFound("комната", roomID)
	}

	// Проверяем пароль
	if room.Password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(password)); err != nil {
			logger.Warn("P2P: неверный пароль комнаты", "roomID", roomID)
			return errors.Unauthorized("неверный пароль")
		}
	}

	s.currentRoom = roomID

	// Создаём WebRTC подключение с STUN конфигурацией
	peerConnection, err := s.api.NewPeerConnection(s.config)
	if err != nil {
		logger.Error("P2P: ошибка создания peer connection", "error", err, "roomID", roomID)
		return fmt.Errorf("ошибка создания peer connection: %w", err)
	}

	peer := &Peer{
		ID:            s.localPeerID,
		UserID:        s.localUserID,
		Connection:    peerConnection,
		LastHeartbeat: time.Now(),
		Authenticated: s.localUserID != "", // Аутентифицирован если есть userID
	}

	room.Peers[s.localPeerID] = peer
	s.peers[s.localPeerID] = peer

	// Настраиваем обработчики
	s.setupPeerConnection(peerConnection, roomID)

	logger.Info("P2P: присоединились к комнате",
		"roomID", roomID,
		"peerID", s.localPeerID,
		"userID", s.localUserID,
		"authenticated", peer.Authenticated,
	)

	s.emitEvent("peer_joined", map[string]string{
		"peerID": s.localPeerID,
		"roomID": roomID,
	})

	return nil
}

// JoinRoomWithToken присоединяет к комнате с JWT аутентификацией.
// Параметр roomID - идентификатор комнаты.
// Параметр password - пароль для входа.
// Параметр token - JWT токен для аутентификации.
// Возвращает ошибку если аутентификация не удалась.
func (s *Service) JoinRoomWithToken(roomID, password, token string) error {
	// Валидируем JWT токен
	claims, err := s.authService.ValidateToken(token)
	if err != nil {
		logger.Warn("P2P: невалидный JWT токен", "roomID", roomID, "error", err)
		return fmt.Errorf("аутентификация не удалась: невалидный токен")
	}

	// Устанавливаем ID пользователя
	s.SetLocalUserID(claims.UserID)

	logger.Info("P2P: пир аутентифицирован",
		"roomID", roomID,
		"userID", claims.UserID,
		"username", claims.Username,
	)

	return s.JoinRoom(roomID, password)
}

// AuthenticatePeer аутентифицирует пира по JWT токену.
// Используется для отложенной аутентификации после установки соединения.
func (s *Service) AuthenticatePeer(peerID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("пир не найден: %s", peerID)
	}

	// Валидируем JWT токен
	claims, err := s.authService.ValidateToken(token)
	if err != nil {
		logger.Warn("P2P: невалидный JWT токен пира", "peerID", peerID, "error", err)
		return fmt.Errorf("аутентификация не удалась")
	}

	peer.UserID = claims.UserID
	peer.Username = claims.Username
	peer.Authenticated = true

	logger.Info("P2P: пир аутентифицирован",
		"peerID", peerID,
		"userID", claims.UserID,
		"username", claims.Username,
	)

	s.emitEvent("peer_authenticated", map[string]string{
		"peerID":   peerID,
		"userID":   claims.UserID,
		"username": claims.Username,
	})

	return nil
}

// LeaveRoom выходит из текущей комнаты.
// Закрывает WebRTC подключение и удаляет пира из комнаты.
// Если комната становится пустой - удаляет её.
// Возвращает ошибку если не подключены к комнате.
func (s *Service) LeaveRoom() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentRoom == "" {
		return errors.New(errors.ErrInvalidInput, "не подключены к комнате")
	}

	roomID := s.currentRoom
	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: комната не найдены при выходе", "roomID", roomID)
		return errors.NotFound("комната", roomID)
	}

	// Закрываем подключение
	if peer, exists := s.peers[s.localPeerID]; exists {
		if peer.DataChannel != nil {
			if err := peer.DataChannel.Close(); err != nil {
				logger.Warn("P2P: ошибка закрытия DataChannel", "error", err, "peerID", s.localPeerID)
			}
		}
		if peer.Connection != nil {
			if err := peer.Connection.Close(); err != nil {
				logger.Warn("P2P: ошибка закрытия PeerConnection", "error", err, "peerID", s.localPeerID)
			}
		}
		delete(s.peers, s.localPeerID)
	}

	// Удаляем пира из комнаты
	delete(room.Peers, s.localPeerID)

	// Если комната пустая - удаляем её
	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
		logger.Info("P2P: комната удалена (пустая)", "roomID", roomID)
	}

	s.currentRoom = ""

	logger.Info("P2P: вышли из комнаты", "roomID", roomID, "peerID", s.localPeerID)

	s.emitEvent("peer_left", map[string]string{
		"peerID": s.localPeerID,
		"roomID": roomID,
	})

	return nil
}

// SendSignal отправляет WebRTC сигнал через data channel.
// Параметр signal - бинарные данные сигнала (SDP offer/answer, ICE candidate).
// Возвращает ошибку если не подключены к комнате или data channel не создан.
func (s *Service) SendSignal(signal []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return fmt.Errorf("не подключены к комнате")
	}

	// Отправляем сигнал через data channel
	peer, exists := s.peers[s.localPeerID]
	if !exists {
		return fmt.Errorf("пир не найден")
	}

	if peer.DataChannel == nil {
		return fmt.Errorf("data channel не создан")
	}

	err := peer.DataChannel.Send(signal)
	if err != nil {
		logger.Error("P2P: ошибка отправки сигнала", "error", err, "roomID", s.currentRoom)
		return fmt.Errorf("ошибка отправки сигнала: %w", err)
	}

	logger.Debug("P2P: сигнал отправлен", "roomID", s.currentRoom, "signalSize", len(signal))
	return nil
}

// GetEvents возвращает канал для получения P2P событий.
// Канал буферизован (100 событий). События включают:
// room_created, peer_joined, peer_left, ice_candidate, connected, disconnected, failed.
func (s *Service) GetEvents() chan models.P2PEvent {
	return s.eventChan
}

// RoomEventsHandler возвращает HTTP обработчик для SSE событий комнаты.
// Поддерживает таймаут соединения, ping/pong и ограничение количества соединений.
func (s *Service) RoomEventsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events := s.GetEvents()

		// Устанавливаем заголовки для SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming не поддерживается", http.StatusInternalServerError)
			return
		}

		// Отправляем начальное событие
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		// Таймер для ping (каждые 30 секунд)
		pingTicker := time.NewTicker(30 * time.Second)
		defer pingTicker.Stop()

		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				data, err := json.Marshal(event)
				if err != nil {
					logger.Error("P2P: ошибка сериализации SSE события", "error", err)
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
				flusher.Flush()

			case <-pingTicker.C:
				fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
				flusher.Flush()

			case <-r.Context().Done():
				logger.Info("P2P: клиент SSE закрыл соединение")
				return
			}
		}
	}
}

// GetRoomInfo возвращает информацию о текущей комнате.
// Возвращает ошибку если не подключены к комнате.
func (s *Service) GetRoomInfo() (*models.RoomInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return nil, fmt.Errorf("не подключены к комнате")
	}

	room, exists := s.rooms[s.currentRoom]
	if !exists {
		return nil, fmt.Errorf("комната не найдена")
	}

	return &models.RoomInfo{
		ID:        room.ID,
		Name:      room.Name,
		HostID:    room.HostID,
		PeerCount: len(room.Peers),
	}, nil
}

// Close закрывает P2P сервис и освобождает все ресурсы.
// Закрывает все WebRTC подключения, очищает комнаты и пиров.
// После вызова сервис не может быть использован.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Сигнализируем горутине обработчика о закрытии
	select {
	case <-s.closeChan:
		// Уже закрыт
	default:
		close(s.closeChan)
	}

	// Закрываем все подключения
	for peerID, peer := range s.peers {
		if peer.DataChannel != nil {
			peer.DataChannel.Close()
		}
		if peer.Connection != nil {
			peer.Connection.Close()
		}
		logger.Debug("P2P: закрыто подключение пира", "peerID", peerID)
	}

	// Очищаем комнаты
	roomCount := len(s.rooms)
	s.rooms = make(map[string]*Room)
	s.peers = make(map[string]*Peer)
	s.currentRoom = ""

	close(s.eventChan)

	logger.Info("P2P сервис остановлен", "closedRooms", roomCount)
	return nil
}

// setupPeerConnection настраивает обработчики событий WebRTC подключения.
// Регистрирует обработчики для ICE кандидатов, состояния подключения и data channel.
func (s *Service) setupPeerConnection(pc *webrtc.PeerConnection, roomID string) {
	// Обработчик ICE кандидатов
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		s.emitEvent("ice_candidate", candidate.ToJSON())
	})

	// Обработчик состояния подключения
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Info("P2P: состояние подключения изменилось",
			"state", state.String(),
			"roomID", roomID,
		)

		switch state {
		case webrtc.PeerConnectionStateConnected:
			s.emitEvent("connected", nil)
		case webrtc.PeerConnectionStateDisconnected:
			s.emitEvent("disconnected", nil)
		case webrtc.PeerConnectionStateFailed:
			s.emitEvent("failed", nil)
		}
	})

	// Обработчик создания data channel
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		logger.Info("P2P: получен data channel", "label", dc.Label(), "roomID", roomID)

		dc.OnOpen(func() {
			s.emitEvent("data_channel_open", dc.Label())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.emitEvent("data_channel_message", msg.Data)
		})

		dc.OnClose(func() {
			s.emitEvent("data_channel_close", dc.Label())
		})

		// Отправляем DataChannel через канал в основную горутину
		// вместо прямого доступа к s.peers из потока WebRTC
		select {
		case s.dataChannelChan <- DataChannelEvent{
			PeerID:      s.localPeerID,
			DataChannel: dc,
		}:
			logger.Debug("P2P: DataChannel отправлен в канал обработки", "peerID", s.localPeerID)
		default:
			logger.Warn("P2P: канал DataChannel полный, событие пропущено", "peerID", s.localPeerID)
		}
	})
}

// handleDataChannelEvents обрабатывает события DataChannel из канала.
// Выполняется в отдельной горутине для безопасного доступа к s.peers.
func (s *Service) handleDataChannelEvents() {
	logger.Info("P2P: обработчик DataChannel событий запущен")
	for {
		select {
		case event := <-s.dataChannelChan:
			s.mu.Lock()
			if peer, exists := s.peers[event.PeerID]; exists {
				peer.DataChannel = event.DataChannel
				logger.Debug("P2P: DataChannel сохранён для пира", "peerID", event.PeerID)
			} else {
				logger.Warn("P2P: пир не найден для DataChannel", "peerID", event.PeerID)
			}
			s.mu.Unlock()
		case <-s.closeChan:
			logger.Info("P2P: обработчик DataChannel событий остановлен")
			return
		}
	}
}

// emitEvent отправляет событие в канал событий.
// Если канал полный - событие пропускается с предупреждением.
func (s *Service) emitEvent(eventType string, data interface{}) {
	select {
	case s.eventChan <- models.P2PEvent{
		Type: eventType,
		Data: data,
	}:
		// Событие отправлено
	default:
		// Канал полный, пропускаем
		logger.Warn("P2P: канал событий полный, событие пропущено", "type", eventType)
	}
}

// generateID генерирует уникальный идентификатор для комнат и пиров.
// Возвращает hex-строку из peerIDLength случайных байт.
func generateID() string {
	bytes := make([]byte, peerIDLength)
	if _, err := rand.Read(bytes); err != nil {
		// crypto/rand может вернуть ошибку только при проблемах
		// с системным источником энтропии (очень редкий случай)
		panic(fmt.Sprintf("P2P: не удалось сгенерировать случайный ID: %v", err))
	}
	return hex.EncodeToString(bytes)
}
