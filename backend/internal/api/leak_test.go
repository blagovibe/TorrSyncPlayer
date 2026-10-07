// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSSEConnectionManager_TryAcquire(t *testing.T) {
	manager := newSSEConnectionManager(2) // Лимит 2 соединения

	// Первые два соединения должны быть разрешены для одной комнаты
	assert.True(t, manager.tryAcquire("room1"))
	assert.True(t, manager.tryAcquire("room1"))

	// Третье соединение должно быть заблокировано
	assert.False(t, manager.tryAcquire("room1"))

	// Проверяем счётчик (общее количество соединений)
	assert.Equal(t, 2, manager.Count())
}

func TestSSEConnectionManager_Release(t *testing.T) {
	manager := newSSEConnectionManager(2)

	// Занимаем оба слота для одной комнаты
	manager.tryAcquire("room1")
	manager.tryAcquire("room1")
	assert.Equal(t, 2, manager.Count())

	// Освобождаем один
	manager.release("room1")
	assert.Equal(t, 1, manager.Count())

	// Теперь можно получить новый слот
	assert.True(t, manager.tryAcquire("room1"))
	assert.Equal(t, 2, manager.Count())
}

func TestSSEConnectionManager_ConcurrentAccess(t *testing.T) {
	manager := newSSEConnectionManager(10)

	// Конкурентный доступ
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			if manager.tryAcquire("room1") {
				time.Sleep(10 * time.Millisecond)
				manager.release("room1")
			}
			done <- true
		}()
	}

	// Ждём завершения всех горутин
	for i := 0; i < 20; i++ {
		<-done
	}

	// Все соединения должны быть освобождены
	assert.Equal(t, 0, manager.Count())
}

func TestSSEConnectionManager_MultipleRooms(t *testing.T) {
	manager := newSSEConnectionManager(2) // Лимит 2 соединения на комнату

	// Комната 1: 2 соединения
	assert.True(t, manager.tryAcquire("room1"))
	assert.True(t, manager.tryAcquire("room1"))
	assert.False(t, manager.tryAcquire("room1")) // Лимит исчерпан

	// Комната 2: 2 соединения (независимо от room1)
	assert.True(t, manager.tryAcquire("room2"))
	assert.True(t, manager.tryAcquire("room2"))
	assert.False(t, manager.tryAcquire("room2")) // Лимит исчерпан

	// Общее количество: 4
	assert.Equal(t, 4, manager.Count())
}

// ── Goroutine Leak Tests ────────────────────────────────────────────────

func TestSSEManager_ConnectionLimit(t *testing.T) {
	manager := newSSEConnectionManager(5)

	// Заполняем все слоты для одной комнаты
	for i := 0; i < 5; i++ {
		assert.True(t, manager.tryAcquire("room1"))
	}

	// Следующее соединение должно быть заблокировано
	assert.False(t, manager.tryAcquire("room1"))

	// Освобождаем все
	for i := 0; i < 5; i++ {
		manager.release("room1")
	}

	assert.Equal(t, 0, manager.Count())
}
