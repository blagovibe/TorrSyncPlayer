// Wails Runtime - автоматически генерируется Wails
// Этот файл предоставляет доступ к функциям рантайма Wails

export interface Runtime {
  EventsOn(eventName: string, callback: (...data: any[]) => void): () => void
  EventsOnce(eventName: string, callback: (...data: any[]) => void): () => void
  EventsOff(eventName: string, ...additionalEventNames: string[]): void
  EventsEmit(eventName: string, ...data: any[]): void
  LogDebug(message: string): void
  LogError(message: string): void
  LogFatal(message: string): void
  LogInfo(message: string): void
  LogTrace(message: string): void
  LogWarning(message: string): void
  WindowReload(): void
  WindowReloadApp(): void
  WindowSetAlwaysOnTop(b: boolean): void
  WindowSetSystemDefaultTheme(): void
  WindowSetLightTheme(): void
  WindowSetDarkTheme(): void
  WindowCenter(): void
  WindowSetTitle(title: string): void
  WindowFullscreen(): void
  WindowUnfullscreen(): void
  WindowIsFullscreen(): Promise<boolean>
  WindowSetSize(width: number, height: number): Promise<void>
  WindowGetSize(): Promise<{ width: number; height: number }>
  WindowSetMaxSize(width: number, height: number): void
  WindowSetMinSize(width: number, height: number): void
  WindowSetPosition(x: number, y: number): void
  WindowGetPosition(): Promise<{ x: number; y: number }>
  WindowHide(): void
  WindowShow(): void
  WindowMaximise(): void
  WindowToggleMaximise(): void
  WindowUnmaximise(): void
  WindowIsMaximised(): Promise<boolean>
  WindowMinimise(): void
  WindowUnminimise(): void
  WindowIsMinimised(): Promise<boolean>
  WindowIsNormal(): Promise<boolean>
  WindowSetBackgroundColour(colour: { r: number; g: number; b: number; a: number }): void
  ScreenGetAll(): Promise<any[]>
  BrowserOpenURL(url: string): void
  Environment(): Promise<any>
  Quit(): void
  Hide(): void
  Show(): void
}

// Глобальный объект Wails
declare global {
  interface Window {
    runtime?: Runtime
    go?: {
      main: {
        App: any
        TorrentService: any
        P2PService: any
        SyncService: any
      }
    }
  }
}

// Экспорт функций рантайма
export function EventsOn(eventName: string, callback: (...data: any[]) => void): () => void {
  if (window.runtime?.EventsOn) {
    return window.runtime.EventsOn(eventName, callback)
  }
  // Fallback для разработки без Wails
  console.log(`[Dev] EventsOn: ${eventName}`)
  return () => {}
}

export function EventsOnce(eventName: string, callback: (...data: any[]) => void): () => void {
  if (window.runtime?.EventsOnce) {
    return window.runtime.EventsOnce(eventName, callback)
  }
  console.log(`[Dev] EventsOnce: ${eventName}`)
  return () => {}
}

export function EventsOff(eventName: string, ...additionalEventNames: string[]): void {
  if (window.runtime?.EventsOff) {
    window.runtime.EventsOff(eventName, ...additionalEventNames)
  }
}

export function EventsEmit(eventName: string, ...data: any[]): void {
  if (window.runtime?.EventsEmit) {
    window.runtime.EventsEmit(eventName, ...data)
  } else {
    console.log(`[Dev] EventsEmit: ${eventName}`, data)
  }
}

export function LogDebug(message: string): void {
  window.runtime?.LogDebug(message) || console.debug(message)
}

export function LogError(message: string): void {
  window.runtime?.LogError(message) || console.error(message)
}

export function LogInfo(message: string): void {
  window.runtime?.LogInfo(message) || console.info(message)
}

export function LogWarning(message: string): void {
  window.runtime?.LogWarning(message) || console.warn(message)
}

export function Quit(): void {
  window.runtime?.Quit()
}

export function Hide(): void {
  window.runtime?.Hide()
}

export function Show(): void {
  window.runtime?.Show()
}

export function BrowserOpenURL(url: string): void {
  window.runtime?.BrowserOpenURL(url) || window.open(url, '_blank')
}
