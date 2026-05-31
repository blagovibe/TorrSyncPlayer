// Автоматически генерируется Wails
// Этот файл предоставляет доступ к методам Go

export function GetAppInfo(): Promise<any> {
  return window.go?.main?.App?.GetAppInfo() || Promise.reject('Wails not available')
}
