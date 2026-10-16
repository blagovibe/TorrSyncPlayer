// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	apperrors "github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// errNotImplemented — методы, не участвующие в проверке. Заглушка должна
// падать, если тест начнёт на них опираться, а не молча возвращать нули.
var errNotImplemented = errors.New("not implemented in per-peer test double")

// bug-0048: выбор файла — per-peer.
//
// Юнит-тесты в internal/torrent доказывают семантику карты выборов. Здесь
// проверяется другое: что HTTP-слой не теряет идентичность пира на трёх
// переходах, где она нужна.
//
//	POST /select         → userID из X-Client-ID (вычитывает TokenMiddleware)
//	POST /stream-ticket  → тот же userID кладётся в тикет
//	GET  /stream         → userID ИЗ ТИКЕТА, а не из заголовка
//
// Ошибка на стыке «тикет ↔ заголовок» иначе невидима: если бы ServeFile брал
// идентичность из запроса, подделанный X-Client-ID позволил бы смотреть файл,
// выбранный другим пиром.

const peerTestTorrentID = "0123456789abcdef0123456789abcdef01234567"

// recordingTorrentService запоминает, с какой пир-идентичностью к нему
// пришли. Реальный торрент для этого не нужен: проверяется маршрутизация
// идентичности, а не загрузка кусков.
type recordingTorrentService struct {
	mu sync.Mutex

	selections []selection // по порядку вызовов SelectFile
	served     []string    // userID в порядке вызовов ServeFile

	// Что сервер расскажет о комнатном торренте. Задаётся тестами, которые
	// проверяют передачу magnet участникам комнаты; по умолчанию торрента
	// нет, и раздача молча не происходит.
	magnet    string
	magnetErr error
	name      string
	selected  int
}

func newRecordingTorrentService() *recordingTorrentService {
	return &recordingTorrentService{}
}

func (s *recordingTorrentService) MagnetFor(context.Context, string, string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.magnet, s.magnetErr
}

type selection struct {
	userID    string
	fileIndex int
}

var _ internal.TorrentService = (*recordingTorrentService)(nil)

func (s *recordingTorrentService) AddMagnet(context.Context, string, string) (*models.TorrentInfo, error) {
	return nil, errNotImplemented
}

func (s *recordingTorrentService) AddTorrent(context.Context, string, io.Reader) (*models.TorrentInfo, error) {
	return nil, errNotImplemented
}

func (s *recordingTorrentService) RemoveTorrent(context.Context, string, string) error { return nil }

func (s *recordingTorrentService) ListTorrents(context.Context, string) []*models.TorrentInfo {
	return nil
}

func (s *recordingTorrentService) GetFiles(context.Context, string, string) ([]models.FileInfo, error) {
	return nil, errNotImplemented
}

func (s *recordingTorrentService) SelectFile(_ context.Context, _ string, userID string, fileIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selections = append(s.selections, selection{userID: userID, fileIndex: fileIndex})
	return nil
}

func (s *recordingTorrentService) ServeFile(w http.ResponseWriter, _ *http.Request, _ string, userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.served = append(s.served, userID)
	w.WriteHeader(http.StatusOK)
}

func (s *recordingTorrentService) ServeSubtitles(w http.ResponseWriter, _ *http.Request, _, _ string, _ int) {
	w.WriteHeader(http.StatusOK)
}

func (s *recordingTorrentService) CanRead(context.Context, string, string) bool {
	return true // recording service allows everything by default
}

func (s *recordingTorrentService) UpdateBufferPosition(context.Context, string, int64) error {
	return nil
}

func (s *recordingTorrentService) GetBufferInfo(context.Context, string, string) (*models.BufferInfo, error) {
	return nil, errNotImplemented
}

func (s *recordingTorrentService) Close() error { return nil }

func (s *recordingTorrentService) snapshot() ([]selection, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]selection(nil), s.selections...), append([]string(nil), s.served...)
}

// perPeerRouter собирает обработчики с НАСТОЯЩИМ TokenMiddleware: именно он
// кладёт X-Client-ID в контекст, и подменять его своей заглушкой значило бы
// не тестировать то, что проверяем.
func perPeerRouter(svc internal.TorrentService) http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(apiAuthSvc.TokenMiddleware)
		r.Post("/torrents/{id}/select", SelectFile(svc))
		r.Delete("/torrents/{id}", RemoveTorrent(svc))
	})
	r.Get("/torrents/{id}/stream", StreamFile(svc, apiAuthSvc))
	return r
}

func postSelectAs(t *testing.T, r http.Handler, torrentID, clientID string, fileIndex int, withHeader bool) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(models.SelectFileRequest{FileIndex: fileIndex})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/torrents/"+torrentID+"/select", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	if withHeader {
		req.Header.Set(auth.HeaderClientID, clientID)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// Выбор обязан нести идентичность пира: без неё он не к кому привязать, и
// сервер обязан сказать об этом, а не принять молча.
func TestSelectFile_WithoutClientIDIsRejected(t *testing.T) {
	svc := &recordingTorrentService{}
	r := perPeerRouter(svc)

	rec := postSelectAs(t, r, peerTestTorrentID, "", 0, false)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	selections, _ := svc.snapshot()
	assert.Empty(t, selections, "запрос без идентичности не должен доходить до выбора файла")
}

// Идентичность из заголовка обязана доходить до сервиса: это и есть per-peer.
func TestSelectFile_PassesClientIDThrough(t *testing.T) {
	svc := &recordingTorrentService{}
	r := perPeerRouter(svc)

	require.Equal(t, http.StatusOK, postSelectAs(t, r, peerTestTorrentID, "peer-a", 7, true).Code)
	require.Equal(t, http.StatusOK, postSelectAs(t, r, peerTestTorrentID, "peer-b", 9, true).Code)

	selections, _ := svc.snapshot()
	require.Len(t, selections, 2)
	assert.Equal(t, selection{userID: "peer-a", fileIndex: 7}, selections[0])
	assert.Equal(t, selection{userID: "peer-b", fileIndex: 9}, selections[1],
		"второй выбор не должен затирать первый: это разные пиры")
}

// Идентичность в /stream берётся из заголовка, как на всех прочих эндпоинтах.
// Никакого посредника больше нет, и это не regressed-сценарий "тикет важнее
// заголовка" — тот тест имел бы смысл только при тикетах.
func TestStreamFile_UsesHeaderClientID(t *testing.T) {
	svc := &recordingTorrentService{}
	r := perPeerRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/torrents/"+peerTestTorrentID+"/stream", nil)
	applyAuthHeadersAs(req, "peer-a")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	_, served := svc.snapshot()
	require.Len(t, served, 1)
	assert.Equal(t, "peer-a", served[0],
		"potок должен достаться пиру из заголовка, не last caller'у")
}

// denyingTorrentService implements TorrentService but denies CanRead for all peers.
// Used to test room-scoped access control (deviation-0039).
type denyingTorrentService struct {
	*recordingTorrentService
}

func (s *denyingTorrentService) CanRead(context.Context, string, string) bool {
	return false
}

// Override SelectFile to enforce CanRead, mirroring torrent.Service.SelectFile.
func (s *denyingTorrentService) SelectFile(ctx context.Context, torrentID, userID string, fileIndex int) error {
	if !s.CanRead(ctx, userID, torrentID) {
		return apperrors.New(apperrors.ErrForbidden, "access denied")
	}
	return s.recordingTorrentService.SelectFile(ctx, torrentID, userID, fileIndex)
}

// SelectFile также ограничен room-scoped доступом (deviation-0039):
// пир вне комнаты не может выбрать файл чужого торрента.
func TestSelectFile_RefusesPeerOutsideRoom(t *testing.T) {
	svc := &denyingTorrentService{recordingTorrentService: &recordingTorrentService{}}

	r := perPeerRouter(svc)

	rec := postSelectAs(t, r, peerTestTorrentID, "peer-a", 0, true)
	assert.Equal(t, http.StatusForbidden, rec.Code, "выбор не должен приниматься пиром вне комнаты")
}

// Сквозная согласованность: кто выбрал — тот и получает поток.
func TestStreamFile_ServesTheSamePeerThatSelected(t *testing.T) {
	svc := &recordingTorrentService{}
	r := perPeerRouter(svc)

	require.Equal(t, http.StatusOK, postSelectAs(t, r, peerTestTorrentID, "peer-a", 1, true).Code)

	req := httptest.NewRequest(http.MethodGet, "/torrents/"+peerTestTorrentID+"/stream", nil)
	applyAuthHeadersAs(req, "peer-a")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	selections, served := svc.snapshot()
	require.Len(t, selections, 1)
	require.Len(t, served, 1)
	assert.Equal(t, selections[0].userID, served[0],
		"поток должен достаться тому, кто сделал выбор, а не последнему обратившемуся")
}

// ── bug-0049: удаление ограничено владельцем ────────────────────────────

// Гость, знающий процессный токен, не должен получать 200 на удаление
// чужого торрента. Именно это и было наблюдаемым дефектом: авторизация одна
// на процесс, ресурс — нет.
func TestRemoveTorrent_PeerCannotRemoveOthersTorrent(t *testing.T) {
	svc := &forbiddingTorrentService{}
	r := perPeerRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/torrents/"+peerTestTorrentID, nil)
	applyAuthHeadersAs(req, "peer-b")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"чужой торрент удалять нельзя; 200 означал бы потерю данных хоста")
	assert.Contains(t, rec.Body.String(), "Access denied",
		"клиенту не следует раскрывать, чей это торрент")
}

// Свой торрент удалить можно — защита не должна ломать обычный сценарий.
func TestRemoveTorrent_OwnerCanRemoveOwnTorrent(t *testing.T) {
	svc := &forbiddingTorrentService{allow: true}
	r := perPeerRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/torrents/"+peerTestTorrentID, nil)
	applyAuthHeadersAs(req, "peer-b")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "peer-b", svc.removedBy)
}

// forbiddingTorrentService имитирует отказ владения на уровне сервиса: так
// тест не зависит от внутреннего состояния настоящего torrent.Service.
type forbiddingTorrentService struct {
	*recordingTorrentService
	allow     bool
	removedBy string
}

func (s *forbiddingTorrentService) RemoveTorrent(_ context.Context, _ string, userID string) error {
	if !s.allow {
		return apperrors.New(apperrors.ErrForbidden, "not the owner")
	}
	s.removedBy = userID
	return nil
}
