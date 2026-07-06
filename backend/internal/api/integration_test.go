// Package api предоставляет интеграционные тесты для HTTP API.
// Тестирует полный цикл работы с API: регистрация, логин, торрент операции, комнаты.
// Этот файл содержит только дополнительные интеграционные тесты, не дублирующие handlers_test.go.
package api
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)
// testServices содержит моки сервисов для тестирования
type integrationTestServices struct {
	torrentSvc *integrationMockTorrentService
	p2pSvc     *integrationMockP2PService
	syncSvc    *integrationMockSyncService
	authStore  *auth.UserStore
	return m.JoinRoom(ctx, roomID, password)
	if _, exists := m.rooms[roomID]; !exists {
		return fmt.Errorf("room not found: %s", roomID)
	}
	m.currentRoom = roomID
	return nil
