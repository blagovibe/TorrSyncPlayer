# Frontend Architecture

## Module Structure

```
frontend/src/
├── main.cpp              # Entry point
├── mainwindow.h/.cpp     # Main window
├── mpvwidget.h/.cpp      # Video player (libmpv)
├── networkmanager.h/.cpp # HTTP client
├── torrentmodel.h/.cpp   # Torrent data model
├── torrentmanager.h/.cpp # Torrent manager
├── roommanager.h/.cpp    # Room manager
├── roomdialog.h/.cpp     # Create/join dialog
├── systemtray.h/.cpp     # System tray
├── inetworkmanager.h     # Network manager interface
├── utils.h/.cpp          # Utilities
├── test_torrentmodel.cpp # TorrentModel tests
└── test_networkmanager.cpp # NetworkManager tests
```

## Main Window Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MainWindow                                      │
│                                                                             │
│  ┌────────────────────────────┐  ┌────────────────────────────────────────┐ │
│  │       Left Panel           │  │           Right Panel                  │ │
│  │                            │  │                                        │ │
│  │  ┌──────────────────────┐  │  │  ┌──────────────────────────────────┐ │ │
│  │  │   TorrentModel       │  │  │  │         MpvWidget                │ │ │
│  │  │   (QListView)        │  │  │  │                                  │ │ │
│  │  └──────────────────────┘  │  │  │  - mpv_handle                    │ │ │
│  │  ┌──────────────────────┐  │  │  │  - mpv_render_context            │ │ │
│  │  │   File List          │  │  │  │  - OpenGL rendering              │ │ │
│  │  │   (QListView)        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  │  ┌──────────────────────┐  │  │  │    Control Panel                 │ │ │
│  │  │  [Magnet Input]      │  │  │  │  [Play/Pause] [Seek] [Time]      │ │ │
│  │  │  [Add Button]        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  └────────────────────────────┘  │  │    Room Panel                    │ │ │
│                                  │  │  [Create] [Join] [Leave]         │ │ │
│                                  │  └──────────────────────────────────┘ │
│                                  └────────────────────────────────────────┘ │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                         Status Bar                                     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

## Backend Communication

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         NetworkManager                                      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        HTTP REST API                                 │    │
│  │                                                                     │    │
│  │  Torrent API:    POST/GET/DELETE /api/v1/torrents/*                 │    │
│  │  Room API:       POST /api/v1/rooms/*                               │    │
│  │  Sync API:       POST/GET /api/v1/sync/*                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     SSE (Server-Sent Events)                        │    │
│  │                                                                     │    │
│  │  GET /api/v1/rooms/{roomID}/events                                  │    │
│  │                                                                     │    │
│  │  Events: connected, peer_joined, peer_left, signal, ping, timeout   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Retry Logic                                   │    │
│  │                                                                     │    │
│  │  - Exponential backoff: delay = baseDelay * 2^attempt               │    │
│  │  - Max retries: 3 (configurable)                                    │    │
│  │  - Base delay: 1000ms (configurable)                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Request pattern:**

```
1. Frontend calls a NetworkManager method (e.g., addTorrent)
2. NetworkManager builds an HTTP request and sends it
3. On response, parses JSON
4. Emits a signal (e.g., torrentAdded)
5. MainWindow is connected to the signal and updates the UI
```

## Video Player (libmpv)

**Components** ([`frontend/src/mpvwidget.h`](../frontend/src/mpvwidget.h)):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MpvWidget                                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        libmpv Core                                   │    │
│  │                                                                     │    │
│  │  mpv_handle ─── mpv_create()                                        │    │
│  │  mpv_render_context ─── mpv_render_context_create()                 │    │
│  │                                                                     │    │
│  │  Commands: play, pause, seek, getProperty                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Event Processing                                  │    │
│  │                                                                     │    │
│  │  mpv_event → processMpvEvent() → eventBuffer → emit signals        │    │
│  │                                                                     │    │
│  │  Events: positionChanged, durationChanged, playbackFinished, error  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Thread Safety                                     │    │
│  │                                                                     │    │
│  │  QMutex for mpv_handle protection                                   │    │
│  │  QTimer for event processing in the main thread                     │    │
│  │  Seek debounce to prevent leaks during fast seeking                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Thread safety:**
- All mpv calls are protected by `QMutex`
- mpv events are buffered and emitted in the main thread
- Seek debounce prevents leaks during fast seeking
