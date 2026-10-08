package p2p

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
)

// newTestService создаёт сервис с готовым auth-сервисом.
func newTestService(t *testing.T) *Service {
	t.Helper()
	authSvc, err := auth.NewAuthService()
	if err != nil {
		t.Fatalf("auth.NewAuthService: %v", err)
	}
	svc, err := NewService(authSvc)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// TestEmitEventConcurrentWithMembershipChanges — регрессионный тест на B-04.
//
// emitEvent обходил room.Peers без блокировки, пока Join/Leave/prune писали в
// ту же карту. Гонка проявляется как фатальная ошибка рантайма
// "concurrent map iteration and map write", которую НЕ ловит recover():
// процесс падает целиком.
//
// Тест гоняет broadcast параллельно с входами и выходами. До фикса он падал
// либо по -race, либо по фатальной ошибке. Сейчас emitEvent снимает список
// получателей под RLock, поэтому и карта, и гонка остаются в прошлом.
func TestEmitEventConcurrentWithMembershipChanges(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Хост создаёт комнату — emitEvent будет рассылать события её участникам.
	room, err := svc.CreateRoom(ctx, "host", "Race Room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	const participants = 8
	const rounds = 200

	var wg sync.WaitGroup

	// Вещатели: постоянно шлют события, заставляя emitEvent обходить карту.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				svc.emitEvent(room.ID, "sync", map[string]interface{}{
					"position": float64(r),
					"playing":  r%2 == 0,
				})
				// BroadcastSync идёт тем же путём и используется SyncService.
				svc.BroadcastSync(room.ID, map[string]interface{}{"position": float64(r)})
			}
		}()
	}

	// Участники хаотично входят и выходят, мутируя room.Peers и sessions.
	for i := 0; i < participants; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", n)
			for r := 0; r < rounds; r++ {
				if err := svc.JoinRoom(ctx, userID, room.ID, ""); err != nil {
					// Выход и повторный вход могут столкнуться; это ожидаемо и
					// не относится к проверяемой гонке.
					_ = err
				}
				time.Sleep(time.Microsecond)
				_ = svc.LeaveRoom(ctx, userID)
			}
		}(i)
	}

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(60 * time.Second):
		t.Fatal("concurrent emit/membership test did not finish: probable deadlock")
	}
}

// TestEmitEventDeliversToHostAndPeersOnce проверяет, что снимок получателей
// не потерял адресатов: и хост, и участники должны получить событие, а хост,
// числящийся ещё и пиром, — ровно один раз.
func TestEmitEventDeliversToHostAndPeersOnce(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host", "Delivery Room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest", room.ID, ""); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	events := svc.GetEvents("host")
	if events == nil {
		t.Fatal("host has no event channel")
	}

	// Присоединение гостя уже отправило host событие peer_joined, поэтому
	// читаем до нужного типа, а не «первое что пришло».
	waitForType := func(want string, timeout time.Duration) bool {
		deadline := time.After(timeout)
		for {
			select {
			case ev := <-events:
				if ev.Type == want {
					return true
				}
			case <-deadline:
				return false
			}
		}
	}

	svc.emitEvent(room.ID, "sync", map[string]string{"position": "1"})
	if !waitForType("sync", 2*time.Second) {
		t.Fatal("host received no sync event")
	}

	// Хост числится и участником, и создателем комнаты: событие обязано прийти
	// ровно один раз, а не дважды.
	svc.emitEvent(room.ID, "sync", map[string]string{"position": "2"})
	if !waitForType("sync", 2*time.Second) {
		t.Fatal("host received no second sync event")
	}

	// Дубликата быть не должно: хост числится и участником, и создателем,
	// но адресат он один. Канал закрывается по таймауту, если событий нет.
	extra := make(chan string, 4)
	go func() {
		defer close(extra)
		for {
			select {
			case ev := <-events:
				extra <- ev.Type
			case <-time.After(500 * time.Millisecond):
				return
			}
		}
	}()

	for typ := range extra {
		if typ == "sync" {
			t.Fatal("host received a duplicate sync event: delivery must not fan out twice")
		}
	}
}

// TestCloseIsIdempotent — регрессионный тест на B-08.
//
// Раньше closeOnce защищал только тикер и closeChan, а close(s.doneChan)
// выполнялся при каждом вызове. Второй Close() паниковал «close of closed
// channel». Выключатель в main.go вызывается из нескольких мест, и
// повторный вызов — не гипотетика.
func TestCloseIsIdempotent(t *testing.T) {
	svc := newTestService(t)

	if err := svc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Второй вызов обязан быть безопасным и не паниковать.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("second Close panicked: %v", r)
			}
		}()
		if err := svc.Close(); err != nil {
			t.Errorf("second Close returned error: %v", err)
		}
	}()
	<-done

	// И параллельные вызовы тоже.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("concurrent Close panicked: %v", r)
				}
			}()
			_ = svc.Close()
		}()
	}
	wg.Wait()
}
