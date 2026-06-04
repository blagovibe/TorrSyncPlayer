package sync

import (
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Инициализируем логгер для тестов
	logger.Init("error", "json")
}

// TestNewService проверяет инициализацию сервиса синхронизации
func TestNewService(t *testing.T) {
	svc := NewService()
	require.NotNil(t, svc)

	defer svc.Close()

	status := svc.GetStatus()
	assert.False(t, status.IsPlaying)
	assert.Equal(t, float64(0), status.Position)
	assert.Equal(t, float64(0), status.Duration)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPlay проверяет запуск воспроизведения
func TestPlay(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status := svc.Play()
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPause проверяет приостановку воспроизведения
func TestPause(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Сначала запускаем
	svc.Play()

	// Затем ставим на паузу
	status := svc.Pause()
	assert.False(t, status.IsPlaying)
}

// TestSeek проверяет перемотку на указанную позицию
func TestSeek(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status, err := svc.Seek(100.5)
	require.NoError(t, err)
	assert.Equal(t, 100.5, status.Position)
}

// TestSeek_InvalidPosition проверяет перемотку с невалидной позицией
func TestSeek_InvalidPosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Отрицательная позиция
	_, err := svc.Seek(-1)
	assert.Error(t, err)

	// Слишком большая позиция
	_, err = svc.Seek(100000)
	assert.Error(t, err)
}

// TestSetDuration проверяет установку длительности
func TestSetDuration(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.SetDuration(3600.0)
	require.NoError(t, err)

	status := svc.GetStatus()
	assert.Equal(t, 3600.0, status.Duration)
}

// TestSetDuration_Invalid проверяет установку невалидной длительности
func TestSetDuration_Invalid(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.SetDuration(-1)
	assert.Error(t, err)
}

// TestUpdatePosition проверяет обновление позиции
func TestUpdatePosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.UpdatePosition(50.0)
	require.NoError(t, err)

	status := svc.GetStatus()
	assert.Equal(t, 50.0, status.Position)
}

// TestSyncWithLatency проверяет синхронизацию с учётом задержки сети
func TestSyncWithLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  100.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(peerStatus, 50) // 50ms задержка
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Position, float64(0))
}

// TestSyncWithLatency_Pause проверяет синхронизацию с состоянием паузы
func TestSyncWithLatency_Pause(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Запускаем воспроизведение
	svc.Play()

	peerStatus := models.SyncStatus{
		IsPlaying: false,
		Position:  50.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(peerStatus, 0)
	assert.False(t, status.IsPlaying)
}

// TestClose проверяет закрытие сервиса
func TestClose(t *testing.T) {
	svc := NewService()

	// Запускаем
	svc.Play()
	status := svc.GetStatus()
	assert.True(t, status.IsPlaying)

	// Закрываем
	svc.Close()

	// После закрытия статус не должен меняться
	status = svc.Play()
	assert.False(t, status.IsPlaying)
}

// TestClose_MultipleCalls проверяет что множественные вызовы Close безопасны
func TestClose_MultipleCalls(t *testing.T) {
	svc := NewService()

	// Множественные вызовы Close не должны вызывать панику
	svc.Close()
	svc.Close()
	svc.Close()
}

// TestValidatePosition проверяет валидацию позиции
func TestValidatePosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	tests := []struct {
		name    string
		pos     float64
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 100.5, false},
		{"valid max", 86400, false},
		{"negative", -1, true},
		{"too large", 86401, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Seek(tt.pos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============ Новые интеграционные тесты ============

// TestSyncPlayback_FullCycle проверяет полный цикл синхронизации воспроизведения
func TestSyncPlayback_FullCycle(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// 1. Начальное состояние — пауза
	status := svc.GetStatus()
	assert.False(t, status.IsPlaying)
	assert.Equal(t, float64(0), status.Position)

	// 2. Устанавливаем длительность
	err := svc.SetDuration(7200.0) // 2 часа
	require.NoError(t, err)

	// 3. Запускаем воспроизведение
	status = svc.Play()
	assert.True(t, status.IsPlaying)

	// 4. Обновляем позицию
	err = svc.UpdatePosition(120.0)
	require.NoError(t, err)

	status = svc.GetStatus()
	assert.Equal(t, 120.0, status.Position)
	assert.True(t, status.IsPlaying)

	// 5. Перематываем
	status, err = svc.Seek(300.0)
	require.NoError(t, err)
	assert.Equal(t, 300.0, status.Position)

	// 6. Ставим на паузу
	status = svc.Pause()
	assert.False(t, status.IsPlaying)
	assert.Equal(t, 300.0, status.Position)

	// 7. Проверяем что длительность сохранилась
	status = svc.GetStatus()
	assert.Equal(t, 7200.0, status.Duration)
}

// TestSyncPlayback_SeekBoundaries проверяет перемотку на граничных значениях
func TestSyncPlayback_SeekBoundaries(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Устанавливаем длительность
	err := svc.SetDuration(3600.0)
	require.NoError(t, err)

	// Перемотка в начало
	status, err := svc.Seek(0)
	require.NoError(t, err)
	assert.Equal(t, float64(0), status.Position)

	// Перемотка в конец
	status, err = svc.Seek(3600.0)
	require.NoError(t, err)
	assert.Equal(t, 3600.0, status.Position)
}

// TestGetPlaybackState_Consistency проверяет консистентность состояния
func TestGetPlaybackState_Consistency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Получаем состояние несколько раз — оно должно быть консистентным
	status1 := svc.GetStatus()
	status2 := svc.GetStatus()
	assert.Equal(t, status1.IsPlaying, status2.IsPlaying)
	assert.Equal(t, status1.Position, status2.Position)
	assert.Equal(t, status1.Duration, status2.Duration)

	// После изменений состояние должно отражать изменения
	svc.Play()
	_ = svc.UpdatePosition(42.0)

	status3 := svc.GetStatus()
	assert.True(t, status3.IsPlaying)
	assert.Equal(t, 42.0, status3.Position)
}

// TestSyncWithLatency_LargeLatency проверяет синхронизацию с большой задержкой
func TestSyncWithLatency_LargeLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  500.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	// Большая задержка 500ms
	status := svc.SyncWithLatency(peerStatus, 500)
	assert.True(t, status.IsPlaying)
	// Позиция должна быть скорректирована с учётом задержки
	assert.Greater(t, status.Position, float64(0))
}

// TestSyncWithLatency_ZeroLatency проверяет синхронизацию без задержки
func TestSyncWithLatency_ZeroLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Устанавливаем позицию близко к пиру (разница < 2с — полная подстройка)
	err := svc.UpdatePosition(199.0)
	require.NoError(t, err)

	now := time.Now().UnixMilli()
	peerStatus := models.SyncStatus{
		IsPlaying: false,
		Position:  200.0,
		Duration:  3600.0,
		Timestamp: now,
	}

	status := svc.SyncWithLatency(peerStatus, 0)
	assert.False(t, status.IsPlaying)
	// При малом расхождении (<2с) должна быть полная подстройка
	assert.InDelta(t, 200.0, status.Position, 0.1)
}

// TestSyncWithLatency_SmallDifference проверяет плавную подстройку при малом расхождении
func TestSyncWithLatency_SmallDifference(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Устанавливаем текущую позицию
	err := svc.UpdatePosition(100.0)
	require.NoError(t, err)

	// Пир с небольшим расхождением (менее 2 секунд)
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  101.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(peerStatus, 0)
	// При малом расхождении должна быть полная подстройка
	assert.InDelta(t, 101.0, status.Position, 0.1)
}

// TestSyncWithLatency_LargeDifference проверяет плавную подстройку при большом расхождении
func TestSyncWithLatency_LargeDifference(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Устанавливаем текущую позицию
	err := svc.UpdatePosition(100.0)
	require.NoError(t, err)

	// Пир с большим расхождением (более 2 секунд — максимальный прыжок)
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  200.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(peerStatus, 0)
	// При большом расхождении должна быть плавная подстройка (30% от разницы)
	// Разница = 100, 30% = 30, новая позиция = 100 + 30 = 130
	assert.InDelta(t, 130.0, status.Position, 0.1)
}

// TestUpdatePosition_InvalidValues проверяет обновление позиции с невалидными значениями
func TestUpdatePosition_InvalidValues(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Отрицательная позиция
	err := svc.UpdatePosition(-10.0)
	assert.Error(t, err)

	// Слишком большая позиция
	err = svc.UpdatePosition(90000.0)
	assert.Error(t, err)
}

// TestSetDuration_EdgeCases проверяет установку длительности в граничных случаях
func TestSetDuration_EdgeCases(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Нулевая длительность — допустима
	err := svc.SetDuration(0)
	assert.NoError(t, err)

	// Отрицательная — недопустима
	err = svc.SetDuration(-1)
	assert.Error(t, err)
}

// TestPlayPauseSequence проверяет последовательность Play/Pause
func TestPlayPauseSequence(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	for i := 0; i < 5; i++ {
		status := svc.Play()
		assert.True(t, status.IsPlaying)

		status = svc.Pause()
		assert.False(t, status.IsPlaying)
	}
}

// TestClosedService_Operations проверяет поведение операций после закрытия
func TestClosedService_Operations(t *testing.T) {
	svc := NewService()
	svc.Close()

	// Play после закрытия
	status := svc.Play()
	assert.False(t, status.IsPlaying)

	// Pause после закрытия
	status = svc.Pause()
	assert.False(t, status.IsPlaying)

	// Seek после закрытия
	_, err := svc.Seek(100.0)
	assert.Error(t, err)

	// SetDuration после закрытия
	err = svc.SetDuration(3600.0)
	assert.Error(t, err)

	// UpdatePosition после закрытия
	err = svc.UpdatePosition(50.0)
	assert.Error(t, err)

	// SyncWithLatency после закрытия
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  100.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}
	status = svc.SyncWithLatency(peerStatus, 0)
	assert.False(t, status.IsPlaying)
}
