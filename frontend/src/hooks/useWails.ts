import { useEffect, useCallback } from 'react'
import { EventsOn, EventsEmit } from '../wailsjs/runtime/runtime'

export function useWailsEvent<T>(eventName: string, callback: (data: T) => void) {
  useEffect(() => {
    const unsubscribe = EventsOn(eventName, callback as (...data: any[]) => void)
    return () => {
      if (typeof unsubscribe === 'function') {
        unsubscribe()
      }
    }
  }, [eventName, callback])
}

export function useWailsEmit() {
  return useCallback((eventName: string, data?: any) => {
    EventsEmit(eventName, data)
  }, [])
}
