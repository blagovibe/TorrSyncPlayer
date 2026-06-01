package torrent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourname/torrplayer/backend/internal/validation"
)

// Глобальный сервис для тестов — создаётся один раз через TestMain
var testService *Service

// TestMain создаёт общий торрент-сервис для всех тестов
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "torrent-test-*")
	if err != nil {
		panic(err)
	}

	svc, err := NewService(tmpDir)
	if err != nil {
		panic(err)
	}
	testService = svc

	code := m.Run()

	svc.Close()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// TestNewService проверяет инициализацию торрент-сервиса
func TestNewService(t *testing.T) {
	require.NotNil(t, testService)
	assert.NotNil(t, testService.client)
	assert.NotNil(t, testService.torrents)
	assert.NotEmpty(t, testService.dataDir)
}

// TestAddMagnet_EmptyURI проверяет валидацию пустой magnet-ссылки
func TestAddMagnet_EmptyURI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := testService.AddMagnet(ctx, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не может быть пустой")
}

// TestAddMagnet_Timeout проверяет обработку невалидной magnet-ссылки
// с таймаутом (реальный торрент-клиент не сможет получить метаданные)
func TestAddMagnet_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	magnetURI := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
	_, err := testService.AddMagnet(ctx, magnetURI)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "таймаут")
}

// TestListTorrents_Empty проверяет получение пустого списка торрентов
func TestListTorrents_Empty(t *testing.T) {
	torrents := testService.ListTorrents()
	assert.NotNil(t, torrents)
}

// TestRemoveTorrent_NotFound проверяет удаление несуществующего торрента
func TestRemoveTorrent_NotFound(t *testing.T) {
	err := testService.RemoveTorrent("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestGetFiles_NotFound проверяет получение файлов несуществующего торрента
func TestGetFiles_NotFound(t *testing.T) {
	_, err := testService.GetFiles("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не найден")
}

// TestSelectFile_InvalidIndex проверяет выбор файла в несуществующем торренте
func TestSelectFile_InvalidIndex(t *testing.T) {
	err := testService.SelectFile("nonexistent", 0)
	assert.Error(t, err)
}

// TestClose проверяет корректное закрытие сервиса
func TestClose(t *testing.T) {
	// Используем глобальный сервис — просто проверяем что Close не паникует
	err := testService.Close()
	assert.NoError(t, err)
}

// TestClose_MultipleCalls проверяет что повторный Close не вызывает панику
func TestClose_MultipleCalls(t *testing.T) {
	// Первый Close уже был в TestClose, проверяем что повторный не паникует
	err := testService.Close()
	assert.NoError(t, err)
}

// TestValidatePosition проверяет валидацию позиции воспроизведения
func TestValidatePosition(t *testing.T) {
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
			err := validation.ValidatePosition(tt.pos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDetectContentType проверяет определение MIME-типа по расширению файла
func TestDetectContentType(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"video.mp4", "video/mp4"},
		{"video.mkv", "video/x-matroska"},
		{"video.avi", "video/x-msvideo"},
		{"video.webm", "video/webm"},
		{"video.mov", "video/quicktime"},
		{"video.wmv", "video/x-ms-wmv"},
		{"video.flv", "video/x-flv"},
		{"audio.mp3", "audio/mpeg"},
		{"audio.aac", "audio/aac"},
		{"audio.wav", "audio/wav"},
		{"audio.ogg", "audio/ogg"},
		{"audio.flac", "audio/flac"},
		{"subtitle.srt", "application/x-subrip"},
		{"subtitle.ass", "text/x-ass"},
		{"subtitle.ssa", "text/x-ass"},
		{"unknown.xyz", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := detectContentType(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}
