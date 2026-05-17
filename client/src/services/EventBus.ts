/**
 * Type-safe EventBus for cross-service communication.
 * Uses plain objects/arrays for ES5 compatibility.
 */

type Listener = (data: unknown) => void;

export class EventBus {
  private listeners: Record<string, Listener[]> = {};

  on(event: string, listener: Listener): () => void {
    if (!this.listeners[event]) {
      this.listeners[event] = [];
    }
    this.listeners[event].push(listener);
    return () => {
      const arr = this.listeners[event];
      if (arr) {
        const idx = arr.indexOf(listener);
        if (idx !== -1) arr.splice(idx, 1);
      }
    };
  }

  emit(event: string, data: unknown): void {
    const arr = this.listeners[event];
    if (arr) {
      for (let i = 0; i < arr.length; i++) {
        arr[i](data);
      }
    }
  }

  removeAllListeners(): void {
    this.listeners = {};
  }
}

export const appEventBus = new EventBus();
