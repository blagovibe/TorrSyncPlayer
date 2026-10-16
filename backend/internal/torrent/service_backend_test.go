// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
)

// stubBackend is a TorrentBackend with no torrent machinery behind it. It lets
// these tests pin the Service's contract with the seam: what the Service must
// resolve, validate and delegate, independently of whether a real client or a
// gateway is underneath.
type stubBackend struct {
	mu sync.Mutex

	handle TorrentHandle
	// addErr, when set, is returned by AddMagnet.
	addErr error

	servedIndex  []int
	servedHandle []TorrentHandle
	dropped      []string
	closed       bool
}

func (b *stubBackend) AddMagnet(context.Context, string) (TorrentHandle, error) {
	if b.addErr != nil {
		return nil, b.addErr
	}
	return b.handle, nil
}

func (b *stubBackend) AddTorrentFile(context.Context, io.Reader) (TorrentHandle, error) {
	return b.handle, nil
}

func (b *stubBackend) Drop(torrentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dropped = append(b.dropped, torrentID)
}

func (b *stubBackend) ServeStream(w http.ResponseWriter, _ *http.Request, handle TorrentHandle, fileIndex int) {
	b.mu.Lock()
	b.servedIndex = append(b.servedIndex, fileIndex)
	b.servedHandle = append(b.servedHandle, handle)
	b.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("payload"))
}

func (b *stubBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
}

// stubHandle is a TorrentHandle with fixed metadata.
type stubHandle struct {
	id        string
	name      string
	total     int64
	completed int64
	complete  bool
	peers     int
	names     []string
	lengths   []int64

	mu          sync.Mutex
	selected    int
	selectedSet bool
}

func (h *stubHandle) ID() string            { return h.id }
func (h *stubHandle) Name() string          { return h.name }
func (h *stubHandle) TotalLength() int64    { return h.total }
func (h *stubHandle) BytesCompleted() int64 { return h.completed }
func (h *stubHandle) Complete() bool        { return h.complete }
func (h *stubHandle) ActivePeers() int      { return h.peers }

func (h *stubHandle) FileCount() int { return len(h.names) }

func (h *stubHandle) FileName(index int) string {
	if index < 0 || index >= len(h.names) {
		return ""
	}
	return h.names[index]
}

func (h *stubHandle) FileLength(index int) int64 {
	if index < 0 || index >= len(h.lengths) {
		return 0
	}
	return h.lengths[index]
}

func (h *stubHandle) SelectForStreaming(fileIndex int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.selected = fileIndex
	h.selectedSet = true
	return nil
}

// BufferTarget returns nil by default, which is the gateway case: the buffer
// service must then be left alone rather than handed a nil file.
func (h *stubHandle) BufferTarget(int) any { return nil }

func newStubService(t *testing.T, backend TorrentBackend, buf *buffer.Service) *Service {
	t.Helper()
	svc, err := NewServiceWithBackend(buf, backend)
	require.NoError(t, err)
	return svc
}

func newStubHandle() *stubHandle {
	return &stubHandle{
		id:      "abc123",
		name:    "Example Movie",
		total:   3000,
		names:   []string{"movie/ex.mkv", "movie/subs.srt"},
		lengths: []int64{2900, 100},
	}
}

// A Service built on a backend must behave like one built on anacrolix:
// same list rendering, same progress maths, same statuses.
func TestServiceOverBackend_ListTorrents(t *testing.T) {
	handle := newStubHandle()
	svc := newStubService(t, &stubBackend{handle: handle}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	list := svc.ListTorrents(context.Background(), "peer-a")
	require.Len(t, list, 1)
	assert.Equal(t, "abc123", list[0].ID)
	assert.Equal(t, "Example Movie", list[0].Name)
	assert.Equal(t, int64(3000), list[0].Size)
	assert.Equal(t, float64(0), list[0].Progress, "nothing downloaded yet")
	assert.Equal(t, "loading", list[0].Status)

	// Progress and the seeding status must come from the handle, not a
	// hardcoded placeholder.
	handle.completed = 1500
	handle.complete = true
	list = svc.ListTorrents(context.Background(), "peer-a")
	assert.InDelta(t, 0.5, list[0].Progress, 0.0001)
	assert.Equal(t, "seeding", list[0].Status)
}

// The list must carry the swarm beside the bytes: the window's preparing
// line shows peers next to megabytes, and a count the service drops is a
// count the window cannot show.
func TestServiceOverBackend_ListCarriesPeers(t *testing.T) {
	handle := newStubHandle()
	handle.peers = 3
	svc := newStubService(t, &stubBackend{handle: handle}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	list := svc.ListTorrents(context.Background(), "peer-a")
	require.Len(t, list, 1)
	assert.Equal(t, 3, list[0].Peers, "the handle's peers must reach the list")
}

func TestServiceOverBackend_GetFilesSanitizesNames(t *testing.T) {
	handle := newStubHandle()
	handle.names = []string{"../../etc/passwd", "clean.mkv"}
	svc := newStubService(t, &stubBackend{handle: handle}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	files, err := svc.GetFiles(context.Background(), "peer-a", "abc123")
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "passwd", files[0].Name, "path traversal must be stripped")
	assert.Equal(t, "clean.mkv", files[1].Name)
	assert.Equal(t, int64(2900), files[0].Size)
}

// Selection must be recorded on the Service and delegated to the backend, and
// the buffer service must be skipped when the backend exposes no piece target.
func TestServiceOverBackend_SelectFileWithoutBufferTarget(t *testing.T) {
	handle := newStubHandle()
	backend := &stubBackend{handle: handle}
	bufSvc := buffer.NewService(1024)
	svc := newStubService(t, backend, bufSvc)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 1))

	handle.mu.Lock()
	selected, wasSet := handle.selected, handle.selectedSet
	handle.mu.Unlock()
	assert.True(t, wasSet, "the backend must be told which file to stream")
	assert.Equal(t, 1, selected)

	// The important half: a nil buffer target must not reach buffer.Service,
	// which would panic or corrupt state.
	_, err = bufSvc.GetBufferInfo(context.Background(), "abc123")
	assert.Error(t, err, "buffer was never registered, so it must not know the torrent")

	assert.Error(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 5))
}

// ServeFile must pass the resolved handle and the *selected* index to the
// backend, not guess.
func TestServiceOverBackend_ServeFileDelegatesSelection(t *testing.T) {
	handle := newStubHandle()
	backend := &stubBackend{handle: handle}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// No selection yet: the backend must never be reached.
	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-a")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 1))

	rec = httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-a")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "payload", rec.Body.String())

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Len(t, backend.servedIndex, 1)
	assert.Equal(t, 1, backend.servedIndex[0], "must stream the selected file, not file 0")
	assert.Same(t, TorrentHandle(handle), backend.servedHandle[0])
}

func TestServiceOverBackend_ServeFileUnknownTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "nope", "peer-a")
	assert.Equal(t, http.StatusNotFound, rec.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex)
}

// Removal must reach the backend only after in-flight streams have drained —
// otherwise a dropped torrent cuts a playing stream mid-file.
func TestServiceOverBackend_RemoveTorrentDropsAfterStreams(t *testing.T) {
	handle := newStubHandle()
	backend := &stubBackend{handle: handle}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))

	// Occupy the stream slot the way a real in-flight stream would.
	svc.streamWG.Add(1)
	released := make(chan struct{})
	go func() {
		<-released
		svc.streamWG.Done()
	}()

	removed := make(chan struct{})
	go func() {
		defer close(removed)
		_ = svc.RemoveTorrent(context.Background(), "abc123", "peer-a")
	}()

	backend.mu.Lock()
	droppedEarly := len(backend.dropped) > 0
	backend.mu.Unlock()
	assert.False(t, droppedEarly, "the torrent must not be dropped while a stream is open")

	close(released)
	<-removed

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Equal(t, []string{"abc123"}, backend.dropped)
}

// After removal the stream slot must be free, so a second remove of the same id
// does not block.
func TestServiceOverBackend_StreamSlotReleased(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))

	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-a")

	require.NoError(t, svc.RemoveTorrent(context.Background(), "abc123", "peer-a"))

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, []string{"abc123"}, backend.dropped)
}

func TestServiceOverBackend_CloseClosesBackend(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)

	require.NoError(t, svc.Close())

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.True(t, backend.closed, "the backend owns resources and must be closed")
}

func TestNewServiceWithBackend_RejectsNil(t *testing.T) {
	_, err := NewServiceWithBackend(nil, nil)
	assert.Error(t, err, "a nil backend would panic on first use")
}

// The API layer's model must still render through the gateway path unchanged.
func TestServiceOverBackend_TorrentInfoShape(t *testing.T) {
	handle := newStubHandle()
	svc := newStubService(t, &stubBackend{handle: handle}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	info, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	var typed = info
	require.NotNil(t, typed)
	assert.Equal(t, "abc123", typed.ID)
	assert.Equal(t, int64(3000), typed.Size)
}

// ── bug-0048: selectedFiles per-peer ────────────────────────────────────
//
// Идея-0046 («каждый качает сам из роя») требует, чтобы выбор файла был
// per-peer, а не глобальным на процесс. Два участника одной комнаты вправе
// смотреть РАЗНЫЕ файлы одного торрента: это разные треки (дубль/замена),
// разные озвучки или просто разные серии сезона.
//
// Глобальный слот делал это невозможным: последний вызов SelectFile
// перетирал выбор предыдущего, и второй участник получал чужой файл.

// Два участника выбирают разные файлы одного торрента — каждый получает свой.
func TestServiceOverBackend_PeerSelectionsAreIndependent(t *testing.T) {
	handle := newStubHandle()
	handle.names = []string{"s01e01.mkv", "s01e02.mkv", "s01e03.mkv"}
	handle.lengths = []int64{1000, 1000, 1000}

	backend := &stubBackend{handle: handle}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	// Оба пира состоят в одной комнате — именно это даёт peer-b право читать
	// торрент, добавленный peer-a (bug-0049, слой 2).
	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-a": {"abc123": true},
		"peer-b": {"abc123": true},
	}})

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// Участник A смотрит первую серию.
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))
	// Участник B — вторую.
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-b", 1))

	// Каждый должен получить СВОЙ файл, а не «последний выбранный».
	recA := httptest.NewRecorder()
	svc.ServeFile(recA, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-a")
	assert.Equal(t, http.StatusOK, recA.Code)

	recB := httptest.NewRecorder()
	svc.ServeFile(recB, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-b")
	assert.Equal(t, http.StatusOK, recB.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Len(t, backend.servedIndex, 2)
	assert.Equal(t, 0, backend.servedIndex[0],
		"peer-a выбрал файл 0 и должен получить файл 0, даже если после него кто-то выбрал другой")
	assert.Equal(t, 1, backend.servedIndex[1],
		"peer-b выбрал файл 1 и должен получить файл 1")
}

// Участник, который ещё ничего не выбирал, не должен получать чужой файл:
// иначе он смотрит не то, что выбрал, и не знает об этом.
func TestServiceOverBackend_StreamRequiresOwnSelection(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	// Все трое в комнате, поэтому отказ новичка — именно про отсутствие
	// СОБСТВЕННОГО выбора. Отказ по доступу проверяется отдельно: он даёт 404
	// (TestServiceOverBackend_ServeFileRefusesForeignTorrent).
	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-a":        {"abc123": true},
		"peer-newcomer": {"abc123": true},
	}})

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// Кто-то другой выбрал файл — это не делает выбор доступным новичку.
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))

	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-newcomer")
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"участник без собственного выбора не должен получать поток")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex, "бэкенд не должен дотягиваться до выбора другого пира")
}

// Повторный выбор тем же участником перезаписывает ТОЛЬКО его выбор.
func TestServiceOverBackend_ReselectionKeepsOtherPeers(t *testing.T) {
	handle := newStubHandle()
	handle.names = []string{"a.mkv", "b.mkv"}
	handle.lengths = []int64{500, 500}

	backend := &stubBackend{handle: handle}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-a": {"abc123": true},
		"peer-b": {"abc123": true},
	}})

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-b", 1))
	// A передумал и вернулся к файлу 0 — отличному от того, что выбрал B.
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))

	recB := httptest.NewRecorder()
	svc.ServeFile(recB, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-b")
	assert.Equal(t, http.StatusOK, recB.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Len(t, backend.servedIndex, 1)
	assert.Equal(t, 1, backend.servedIndex[0],
		"выбор B не должен пострадать от того, что A позже выбрал файл 0")
}

// Удаление торрента убирает выбор всех участников разом.
//
// Этот тест НЕ доказывает per-peer семантику — он одинаково проходит и на
// глобальном слоте (проверено). Он нужен как защита от утечки: при переходе на
// вложенную карту легко забыть удалить внешний ключ, и тогда удалённый торрент
// оставит после себя пустую карту выборов, пережившую сам торрент.
func TestServiceOverBackend_RemoveTorrentClearsAllPeerSelections(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// Owner selects two files (simulating reselection)
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 1))

	require.NoError(t, svc.RemoveTorrent(context.Background(), "abc123", "peer-a"))

	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-a")
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"после удаления торрента выбор не должен уцелеть ни для кого")
}

// ── bug-0049: торрент принадлежит добавившему его пиру ──────────────────
//
// Токен доступа один на процесс, поэтому гость комнаты, подключившийся к
// чужому серверу, получает ту же авторизацию, что и хост. Наблюдаемое
// последствие: DELETE /torrents/{id} удаляет чужие данные без спроса.
//
// Минимальная граница, которую имеет смысл провести сейчас: удалять свой
// торрент можно всегда, чужой — нельзя.

// Гость не может удалить торрент хоста.
func TestServiceOverBackend_PeerCannotRemoveOthersTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	err = svc.RemoveTorrent(context.Background(), "abc123", "peer-b")
	require.Error(t, err, "чужой торрент удалять нельзя: это данные другого участника")

	// Торрент обязан остаться на месте.
	assert.Equal(t, 1, len(svc.ListTorrents(context.Background(), "peer-a")),
		"отказ в удалении не должен выбрасывать торрент")

	backend.mu.Lock()
	assert.Empty(t, backend.dropped, "бэкенд не должен терять куски чужого торрента")
	backend.mu.Unlock()
}

// Владелец удаляет свой торрент — обычный сценарий должен работать.
func TestServiceOverBackend_OwnerCanRemoveOwnTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	require.NoError(t, svc.RemoveTorrent(context.Background(), "abc123", "peer-a"),
		"владелец должен иметь право убрать свой торрент")

	assert.Empty(t, svc.ListTorrents(context.Background(), "peer-a"))

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Equal(t, []string{"abc123"}, backend.dropped)
}

// ── bug-0049 (слой 2): что пиру видно при чтении ─────────────────────────

// stubVisibility отвечает за комнаты, не поднимая настоящий p2p-сервис.
type stubVisibility struct {
	mu     sync.Mutex
	byUser map[string]map[string]bool
}

func (v *stubVisibility) TorrentsVisibleTo(_ context.Context, userID string) map[string]bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if set, ok := v.byUser[userID]; ok {
		return set
	}
	return map[string]bool{}
}

// Гость комнаты видит торрент своей комнаты и не видит чужих.
func TestServiceOverBackend_ListHidesForeignTorrents(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-b": {"abc123": true}, // гость состоит в комнате
	}})

	guestList := svc.ListTorrents(context.Background(), "peer-b")
	assert.Len(t, guestList, 1, "гость комнаты видит её торрент")

	// Посторонний не состоит ни в какой комнате.
	outsiderList := svc.ListTorrents(context.Background(), "peer-c")
	assert.Empty(t, outsiderList,
		"пир вне комнаты не должен видеть чужую библиотеку")
}

// Владелец всегда видит своё, даже если его нет ни в какой комнате.
func TestServiceOverBackend_OwnerSeesOwnTorrentWithoutRoom(t *testing.T) {
	svc := newStubService(t, &stubBackend{handle: newStubHandle()}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{}})
	assert.Len(t, svc.ListTorrents(context.Background(), "peer-a"), 1,
		"владелец не должен терять доступ к своему торренту без комнаты")
}

// Список файлов чужого торрента недоступен.
func TestServiceOverBackend_GetFilesRefusesForeignTorrent(t *testing.T) {
	svc := newStubService(t, &stubBackend{handle: newStubHandle()}, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-b": {"abc123": true},
	}})

	_, err = svc.GetFiles(context.Background(), "peer-a", "abc123")
	assert.NoError(t, err, "участник комнаты получает список файлов")

	_, err = svc.GetFiles(context.Background(), "peer-c", "abc123")
	require.Error(t, err, "посторонний не должен узнавать состав чужого торрента")
}

// Поток чужого торрента не отдаётся: это данные, а не только метаданные.
func TestServiceOverBackend_ServeFileRefusesForeignTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)
	require.NoError(t, svc.SelectFile(context.Background(), "abc123", "peer-a", 0))

	svc.SetVisibility(&stubVisibility{byUser: map[string]map[string]bool{
		"peer-b": {"abc123": true},
	}})

	rec := httptest.NewRecorder()
	svc.ServeFile(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), "abc123", "peer-c")
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"посторонний не должен получать поток; 404 не раскрывает существование торрента")

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex, "бэкенд не должен отдавать куки чужого торрента")
}
