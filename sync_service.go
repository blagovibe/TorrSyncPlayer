package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"torrsyncplayer/logger"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// MaxDuration максимальная длительность контента в секундах (24 часа)
const MaxDuration = 86400

// SyncService сервис синхронизации воспроизведения
type SyncService struct {
	ctx context.Context
	mu  sync.RWMutex
	p2p P2PServiceInterface

	// Состояние
	isHost       bool
	currentState PlaybackState
	syncStats    SyncStats

	// Настройки
	syncTolerance     float64 // допуск в миллисекундах
	heartbeatInterval time.Duration

	// Каналы
	commandChan     chan SyncCommand
	stateUpdateChan chan PlaybackState
	closeChan       chan struct{}
	closeOnce       sync.Once

	// RTT измерения
	rttMeasurements []float64
	lastHeartbeat   int64

	// Коллбэки
	onSyncCommand func(SyncCommand)
	onStateUpdate func(PlaybackState)
}

// NewSyncService создает новый сервис синхронизации
func NewSyncService() *SyncService {
	return &SyncService{
		syncTolerance:     1500, // 1.5 секунды по умолчанию
		heartbeatInterval: 2 * time.Second,
		commandChan:       make(chan SyncCommand, 100),
		stateUpdateChan:   make(chan PlaybackState, 100),
		closeChan:         make(chan struct{}),
		rttMeasurements:   make([]float64, 0, 10),
		syncStats: SyncStats{
			SyncTolerance: 1500,
		},
	}
}

// Init инициализирует сервис синхронизации с P2P сервисом
func (s *SyncService) Init(ctx context.Context) error {
	s.ctx = ctx
	logger.Info("Sync Service initialized", "service", "sync")
	return nil
}

// SetP2PService устанавливает P2P сервис (для dependency injection)
func (s *SyncService) SetP2PService(p2p P2PServiceInterface) {
	s.p2p = p2p
}

// SetAsHost устанавливает режим Host
func (s *SyncService) SetAsHost() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isHost = true
	logger.Info("Sync Service: set as Host", "service", "sync")
	s.emitEvent("sync:role_changed", map[string]string{"role": "host"})
}

// SetAsGuest устанавливает режим Guest
func (s *SyncService) SetAsGuest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isHost = false
	logger.Info("Sync Service: set as Guest", "service", "sync")
	s.emitEvent("sync:role_changed", map[string]string{"role": "guest"})
}

// IsHost возвращает true если текущий пир - Host
func (s *SyncService) IsHost() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isHost
}

// validatePosition валидирует позицию воспроизведения
func (s *SyncService) validatePosition(position float64) error {
	if position < 0 {
		return fmt.Errorf("position cannot be negative: %.2f", position)
	}
	if position > MaxDuration {
		return fmt.Errorf("position exceeds maximum duration (%d seconds): %.2f", MaxDuration, position)
	}
	if math.IsNaN(position) {
		return fmt.Errorf("position cannot be NaN")
	}
	if math.IsInf(position, 0) {
		return fmt.Errorf("position cannot be infinite")
	}
	return nil
}

// Play команда воспроизведения
func (s *SyncService) Play(position float64) error {
	// Валидация позиции
	if err := s.validatePosition(position); err != nil {
		return fmt.Errorf("invalid play position: %w", err)
	}

	s.mu.Lock()
	s.currentState.IsPlaying = true
	s.currentState.Position = position
	s.currentState.Timestamp = time.Now().UnixMilli()
	if s.currentState.PlaybackRate == 0 {
		s.currentState.PlaybackRate = 1.0
	}
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.notifyStateChange(state)

	if s.isHost {
		s.broadcastCommand(SyncCommand{
			Type:      "play",
			Timestamp: time.Now().UnixMilli(),
			Data: map[string]interface{}{
				"position":     position,
				"playbackRate": state.PlaybackRate,
			},
		})
	}

	logger.Info("Play command", "service", "sync", "position", position)
	return nil
}

// Pause команда паузы
func (s *SyncService) Pause() error {
	s.mu.Lock()
	s.currentState.IsPlaying = false
	s.currentState.Timestamp = time.Now().UnixMilli()
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.notifyStateChange(state)

	if s.isHost {
		s.broadcastCommand(SyncCommand{
			Type:      "pause",
			Timestamp: time.Now().UnixMilli(),
			Data: map[string]interface{}{
				"position": state.Position,
			},
		})
	}

	logger.Info("Pause command", "service", "sync", "position", state.Position)
	return nil
}

// PauseAt команда паузы с указанием позиции
func (s *SyncService) PauseAt(position float64) error {
	// Валидация позиции
	if err := s.validatePosition(position); err != nil {
		return fmt.Errorf("invalid pause position: %w", err)
	}

	s.mu.Lock()
	s.currentState.IsPlaying = false
	s.currentState.Position = position
	s.currentState.Timestamp = time.Now().UnixMilli()
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.notifyStateChange(state)

	if s.isHost {
		s.broadcastCommand(SyncCommand{
			Type:      "pause",
			Timestamp: time.Now().UnixMilli(),
			Data: map[string]interface{}{
				"position": position,
			},
		})
	}

	logger.Info("Pause command", "service", "sync", "position", position)
	return nil
}

// Seek команда перемотки
func (s *SyncService) Seek(position float64) error {
	// Валидация позиции
	if err := s.validatePosition(position); err != nil {
		return fmt.Errorf("invalid seek position: %w", err)
	}

	s.mu.Lock()
	s.currentState.Position = position
	s.currentState.Timestamp = time.Now().UnixMilli()
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.notifyStateChange(state)

	if s.isHost {
		s.broadcastCommand(SyncCommand{
			Type:      "seek",
			Timestamp: time.Now().UnixMilli(),
			Data: map[string]interface{}{
				"position": position,
			},
		})
	}

	logger.Info("Seek command", "service", "sync", "position", position)
	return nil
}

// GetPlaybackState возвращает текущее состояние воспроизведения
func (s *SyncService) GetPlaybackState() PlaybackState {
	return s.GetState()
}

// GetSyncStats возвращает статистику синхронизации
func (s *SyncService) GetSyncStats() SyncStats {
	return s.GetStats()
}

// SetLatencyCompensation устанавливает компенсацию задержки (в мс)
func (s *SyncService) SetLatencyCompensation(ms int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Компенсация задержки реализована через syncTolerance
	s.syncTolerance = float64(ms)
	s.syncStats.SyncTolerance = float64(ms)
	logger.Info("Latency compensation set", "service", "sync", "latency_ms", ms)
	s.emitEvent("sync:latency_changed", map[string]int{"latency": ms})
}

// OnStateChange регистрирует обработчик изменения состояния
func (s *SyncService) OnStateChange(handler func(PlaybackState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStateUpdate = handler
}

// OnSyncStats регистрирует обработчик изменения статистики
func (s *SyncService) OnSyncStats(handler func(SyncStats)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Сохраняем обработчик (можно расширить для множественных обработчиков)
	_ = handler
}

// notifyStateChange уведомляет об изменении состояния
func (s *SyncService) notifyStateChange(state PlaybackState) {
	s.mu.RLock()
	handler := s.onStateUpdate
	s.mu.RUnlock()
	if handler != nil {
		handler(state)
	}
}

// notifySyncStats уведомляет об изменении статистики
func (s *SyncService) notifySyncStats(stats SyncStats) {
	s.mu.RLock()
	handler := s.onSyncCommand
	s.mu.RUnlock()
	if handler != nil {
		handler(SyncCommand{Type: "stats", Data: stats})
	}
}

// UpdateState обновляет текущее состояние (вызывается из фронтенда)
func (s *SyncService) UpdateState(state PlaybackState) {
	s.mu.Lock()
	s.currentState = state
	s.mu.Unlock()

	if s.isHost {
		// Host отправляет состояние всем пирам
		s.broadcastCommand(SyncCommand{
			Type:      "state",
			Timestamp: time.Now().UnixMilli(),
			Data:      state,
		})
	}
}

// HandleSyncCommand обрабатывает команду синхронизации от Host
func (s *SyncService) HandleSyncCommand(cmd SyncCommand) {
	s.mu.Lock()
	s.syncStats.LastSyncTime = time.Now().UnixMilli()
	s.mu.Unlock()

	logger.Info("Received sync command", "service", "sync", "cmd_type", cmd.Type)

	switch cmd.Type {
	case "play":
		s.handlePlayCommand(cmd)
	case "pause":
		s.handlePauseCommand(cmd)
	case "seek":
		s.handleSeekCommand(cmd)
	case "state":
		s.handleStateCommand(cmd)
	case "heartbeat":
		s.handleHeartbeat(cmd)
	default:
		logger.Warn("Unknown sync command type", "service", "sync", "cmd_type", cmd.Type)
	}
}

// GetState возвращает текущее состояние воспроизведения
func (s *SyncService) GetState() PlaybackState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentState
}

// GetStats возвращает статистику синхронизации
func (s *SyncService) GetStats() SyncStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncStats
}

// SetSyncTolerance устанавливает допуск синхронизации (в мс)
func (s *SyncService) SetSyncTolerance(toleranceMs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncTolerance = toleranceMs
	s.syncStats.SyncTolerance = toleranceMs
	logger.Info("Sync tolerance set", "service", "sync", "tolerance_ms", toleranceMs)
	s.emitEvent("sync:tolerance_changed", map[string]float64{"tolerance": toleranceMs})
}

// StartHeartbeat запускает heartbeat для проверки синхронизации
func (s *SyncService) StartHeartbeat() {
	go s.heartbeatLoop()
	logger.Info("Heartbeat started", "service", "sync")
}

// Close безопасно закрывает сервис (можно вызывать многократно)
func (s *SyncService) Close() {
	s.closeOnce.Do(func() {
		close(s.closeChan)
		logger.Info("SyncService closed", "service", "sync")
	})
}

// Внутренние методы

// heartbeatLoop цикл отправки heartbeat
func (s *SyncService) heartbeatLoop() {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.closeChan:
			return
		case <-ticker.C:
			s.sendHeartbeat()
		}
	}
}

// sendHeartbeat отправляет heartbeat
func (s *SyncService) sendHeartbeat() {
	now := time.Now().UnixMilli()

	s.mu.Lock()
	s.lastHeartbeat = now
	s.mu.Unlock()

	if s.isHost {
		// Host отправляет heartbeat с текущим состоянием
		state := s.GetState()
		s.broadcastCommand(SyncCommand{
			Type:      "heartbeat",
			Timestamp: now,
			Data: map[string]interface{}{
				"position":     state.Position,
				"isPlaying":    state.IsPlaying,
				"playbackRate": state.PlaybackRate,
			},
		})
	} else {
		// Guest отправляет heartbeat для измерения RTT
		s.broadcastCommand(SyncCommand{
			Type:      "heartbeat",
			Timestamp: now,
		})
	}
}

// handlePlayCommand обработка команды воспроизведения
func (s *SyncService) handlePlayCommand(cmd SyncCommand) {
	if s.isHost {
		return // Host не обрабатывает свои команды
	}

	data, ok := cmd.Data.(map[string]interface{})
	if !ok {
		logger.Warn("Invalid play command data", "service", "sync")
		return
	}

	position, _ := data["position"].(float64)
	playbackRate, _ := data["playbackRate"].(float64)

	// Компенсация задержки: вычисляем ожидаемую позицию на момент получения
	now := time.Now().UnixMilli()
	elapsed := float64(now-cmd.Timestamp) / 1000.0 // в секундах

	s.mu.Lock()
	s.currentState.IsPlaying = true
	s.currentState.Position = position + elapsed*playbackRate
	s.currentState.PlaybackRate = playbackRate
	s.currentState.Timestamp = now
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.emitEvent("sync:play", map[string]interface{}{
		"position":     state.Position,
		"playbackRate": playbackRate,
	})

	logger.Info("Handled play command", "service", "sync", "position", position, "compensated", state.Position)
}

// handlePauseCommand обработка команды паузы
func (s *SyncService) handlePauseCommand(cmd SyncCommand) {
	if s.isHost {
		return
	}

	data, ok := cmd.Data.(map[string]interface{})
	if !ok {
		logger.Warn("Invalid pause command data", "service", "sync")
		return
	}

	position, _ := data["position"].(float64)

	// Компенсация задержки
	now := time.Now().UnixMilli()
	elapsed := float64(now-cmd.Timestamp) / 1000.0

	s.mu.Lock()
	s.currentState.IsPlaying = false
	s.currentState.Position = position + elapsed
	s.currentState.Timestamp = now
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.emitEvent("sync:pause", map[string]interface{}{
		"position": state.Position,
	})

	logger.Info("Handled pause command", "service", "sync", "position", position, "compensated", state.Position)
}

// handleSeekCommand обработка команды перемотки
func (s *SyncService) handleSeekCommand(cmd SyncCommand) {
	if s.isHost {
		return
	}

	data, ok := cmd.Data.(map[string]interface{})
	if !ok {
		logger.Warn("Invalid seek command data", "service", "sync")
		return
	}

	position, _ := data["position"].(float64)

	s.mu.Lock()
	s.currentState.Position = position
	s.currentState.Timestamp = time.Now().UnixMilli()
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)
	s.emitEvent("sync:seek", map[string]interface{}{
		"position": position,
	})

	logger.Info("Handled seek command", "service", "sync", "position", position)
}

// handleStateCommand обработка команды обновления состояния
func (s *SyncService) handleStateCommand(cmd SyncCommand) {
	if s.isHost {
		return
	}

	// Десериализуем состояние
	stateData, err := json.Marshal(cmd.Data)
	if err != nil {
		logger.Error("Failed to marshal state data", "service", "sync", "error", err)
		return
	}

	var state PlaybackState
	if err := json.Unmarshal(stateData, &state); err != nil {
		logger.Error("Failed to unmarshal state", "service", "sync", "error", err)
		return
	}

	// Компенсация задержки
	now := time.Now().UnixMilli()
	elapsed := float64(now-cmd.Timestamp) / 1000.0

	if state.IsPlaying {
		state.Position += elapsed * state.PlaybackRate
	}

	state.Timestamp = now

	s.mu.Lock()
	s.currentState = state
	s.mu.Unlock()

	// Проверяем рассинхронизацию
	drift := s.checkDrift()
	if math.Abs(drift) > s.syncTolerance {
		s.correctDrift(drift)
	}

	s.emitEvent("sync:state_changed", state)
	logger.Info("Handled state command", "service", "sync", "position", state.Position, "is_playing", state.IsPlaying)
}

// handleHeartbeat обработка heartbeat
func (s *SyncService) handleHeartbeat(cmd SyncCommand) {
	now := time.Now().UnixMilli()

	if s.isHost {
		// Host получает heartbeat от Guest - вычисляем RTT
		rtt := s.calculateRTT(cmd.Timestamp)
		s.mu.Lock()
		s.syncStats.RTT = rtt
		s.mu.Unlock()
	} else {
		// Guest получает heartbeat от Host
		data, ok := cmd.Data.(map[string]interface{})
		if !ok {
			// Это ответ на наш heartbeat - вычисляем RTT
			rtt := s.calculateRTT(cmd.Timestamp)
			s.mu.Lock()
			s.syncStats.RTT = rtt
			s.mu.Unlock()
			return
		}

		// Обновляем состояние на основе heartbeat от Host
		position, _ := data["position"].(float64)
		isPlaying, _ := data["isPlaying"].(bool)
		playbackRate, _ := data["playbackRate"].(float64)

		elapsed := float64(now-cmd.Timestamp) / 1000.0

		s.mu.Lock()
		s.currentState.IsPlaying = isPlaying
		if isPlaying {
			s.currentState.Position = position + elapsed*playbackRate
		} else {
			s.currentState.Position = position
		}
		s.currentState.PlaybackRate = playbackRate
		s.currentState.Timestamp = now
		state := s.currentState
		s.mu.Unlock()

		// Проверяем рассинхронизацию
		drift := s.checkDrift()
		if math.Abs(drift) > s.syncTolerance {
			s.correctDrift(drift)
		}

		s.emitEvent("sync:state_changed", state)
	}
}

// broadcastCommand отправляет команду всем пирам через P2P сервис
func (s *SyncService) broadcastCommand(cmd SyncCommand) {
	if s.p2p == nil {
		logger.Warn("P2P service not available", "service", "sync")
		return
	}

	// Конвертируем SyncCommand в P2PMessage
	var msgType P2PMessageType
	switch cmd.Type {
	case "play":
		msgType = MsgPlay
	case "pause":
		msgType = MsgPause
	case "seek":
		msgType = MsgSeek
	case "state":
		msgType = MsgState
	case "heartbeat":
		msgType = MsgHeartbeat
	default:
		logger.Warn("Unknown command type for broadcast", "service", "sync", "cmd_type", cmd.Type)
		return
	}

	msg := P2PMessage{
		Type:      msgType,
		Timestamp: cmd.Timestamp,
		Data:      cmd,
	}

	if err := s.p2p.SendMessage(msg); err != nil {
		logger.Error("Failed to broadcast command", "service", "sync", "error", err)
	}
}

// calculateRTT вычисляет Round Trip Time
func (s *SyncService) calculateRTT(timestamp int64) float64 {
	now := time.Now().UnixMilli()
	rtt := float64(now - timestamp)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Сохраняем последние 10 измерений
	s.rttMeasurements = append(s.rttMeasurements, rtt)
	if len(s.rttMeasurements) > 10 {
		s.rttMeasurements = s.rttMeasurements[1:]
	}

	// Вычисляем среднее RTT
	var sum float64
	for _, v := range s.rttMeasurements {
		sum += v
	}
	avgRTT := sum / float64(len(s.rttMeasurements))

	s.syncStats.RTT = avgRTT
	return avgRTT
}

// checkDrift проверяет рассинхронизацию
func (s *SyncService) checkDrift() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Вычисляем ожидаемую позицию на основе времени
	now := time.Now().UnixMilli()
	elapsed := float64(now-s.currentState.Timestamp) / 1000.0

	expectedPosition := s.currentState.Position
	if s.currentState.IsPlaying {
		expectedPosition += elapsed * s.currentState.PlaybackRate
	}

	// Здесь мы можем сравнить с реальной позицией из фронтенда
	// Пока возвращаем 0, так как реальная позиция будет обновляться из фронтенда
	return 0
}

// correctDrift корректирует рассинхронизацию
func (s *SyncService) correctDrift(drift float64) {
	s.mu.Lock()
	s.syncStats.CorrectionCount++
	s.syncStats.Drift = drift
	s.mu.Unlock()

	logger.Warn("Drift detected, correcting...", "service", "sync", "drift", drift)
	s.emitEvent("sync:correction", map[string]interface{}{
		"drift": drift,
	})

	// Коррекция будет выполнена через обновление состояния
	// Фронтенд должен обработать sync:correction и скорректировать позицию
}

// emitEvent отправляет событие во фронтенд через Wails Events
func (s *SyncService) emitEvent(eventName string, data interface{}) {
	if s.ctx == nil {
		return
	}

	select {
	case <-s.ctx.Done():
		return
	default:
		runtime.EventsEmit(s.ctx, "sync:"+eventName, data)
	}
}

// SetPlaybackRate устанавливает скорость воспроизведения
func (s *SyncService) SetPlaybackRate(rate float64) {
	s.mu.Lock()
	s.currentState.PlaybackRate = rate
	s.currentState.Timestamp = time.Now().UnixMilli()
	state := s.currentState
	s.mu.Unlock()

	s.emitEvent("sync:state_changed", state)

	if s.isHost {
		s.broadcastCommand(SyncCommand{
			Type:      "play",
			Timestamp: time.Now().UnixMilli(),
			Data: map[string]interface{}{
				"position":     state.Position,
				"playbackRate": rate,
			},
		})
	}

	logger.Info("Playback rate set", "service", "sync", "rate", rate)
}

// SyncNow принудительная синхронизация (для Guest)
func (s *SyncService) SyncNow() {
	if s.isHost {
		return
	}

	s.mu.RLock()
	state := s.currentState
	s.mu.RUnlock()

	// Запрашиваем синхронизацию у Host
	s.broadcastCommand(SyncCommand{
		Type:      "sync",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position":  state.Position,
			"isPlaying": state.IsPlaying,
		},
	})

	logger.Info("Sync requested", "service", "sync")
}

// GetDrift возвращает текущий дрифт
func (s *SyncService) GetDrift() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncStats.Drift
}

// GetRTT возвращает текущий RTT
func (s *SyncService) GetRTT() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncStats.RTT
}

// Проверка реализации интерфейса
var _ SyncServiceInterface = (*SyncService)(nil)
