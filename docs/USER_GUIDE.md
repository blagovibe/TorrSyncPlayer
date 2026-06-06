# TorrSyncPlayer User Guide

## Contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Core Features](#core-features)
4. [Torrent Management](#torrent-management)
5. [P2P Rooms](#p2p-rooms)
6. [Playback Synchronization](#playback-synchronization)
7. [Settings](#settings)
8. [Troubleshooting](#troubleshooting)

## Introduction

TorrSyncPlayer is an application for streaming media content via torrents with the ability to synchronize playback with other users.

### Features

- **Streaming playback** — instant viewing without full download
- **P2P rooms** — synchronized viewing with friends
- **WebRTC** — direct peer-to-peer connection without server-side video relay
- **JWT authentication** — secure room access
- **Privacy** — room passwords, private sessions
- **Buffering** — smart preloading with priorities

## Getting Started

### Requirements

- **Backend:** Go 1.25+
- **Frontend:** Qt 6.5+, libmpv
- **OS:** Windows, Linux, macOS

### Running

1. **Start the backend server:**
   ```bash
   cd backend
   make run
   ```
   The server will start on port 8889.

2. **Start the frontend application:**
   ```bash
   cd frontend
   ./build.sh  # Linux/macOS
   build.bat   # Windows
   ```

### Quick Start

1. Start the backend server
2. Start the frontend application
3. Enter a magnet link in the input field
4. Click "Add"
5. Select a file from the list
6. Click "Play"

## Core Features

### Interface

- **Left panel:** torrent list, file list, magnet link input field
- **Right panel:** video player, playback controls, room buttons
- **Status bar:** connection state information

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Space` | Pause/Play |
| `←/→` | Seek 5 seconds |
| `↑/↓` | Volume |
| `F` | Fullscreen |
| `M` | Mute |
| `Ctrl+O` | Open magnet link |
| `Ctrl+R` | Create room |

## Torrent Management

### Adding a Torrent

1. Copy a magnet link
2. Paste it into the input field at the bottom of the left panel
3. Click the "Add" button or press Enter
4. Wait for metadata to load

### Selecting a File

1. Select a torrent in the list
2. In the file list, choose the desired video file
3. Click "Select for playback"

### Removing a Torrent

1. Select a torrent in the list
2. Click the "Delete" button or press the Delete key

### Supported Formats

- **Video:** mp4, mkv, avi, webm, mov, wmv, flv
- **Audio:** mp3, aac, wav, ogg, flac
- **Subtitles:** srt, ass, ssa

## P2P Rooms

### Creating a Room

1. Click the "Create Room" button
2. Enter a room name
3. (Optional) Set a password
4. Click "Create"

You become the room host. Other users can join by room ID.

### Joining a Room

1. Click the "Join Room" button
2. Enter the room ID
3. Enter the password (if set)
4. Click "Join"

### Room Management

- **Host** can:
  - Set password
  - Remove peers
  - Transfer host privileges

- **Peer** can:
  - Leave the room
  - Synchronize playback

## Playback Synchronization

### How It Works

When you are connected to a room, all actions are synchronized:

- **Play/Pause** — playback is synchronous for everyone
- **Seek** — seeking is synchronous
- **Speed** — uniform playback speed

### Latency Compensation

The application automatically compensates for network latency for synchronized viewing. The algorithm uses smooth position adjustment with a coefficient of 0.3 and a maximum jump of 2 seconds.

## Settings

### Backend Settings

| Parameter | Default | Description |
|-----------|---------|-------------|
| `--port` | 8889 | HTTP server port |
| `--jwt-secret` | (empty) | JWT token secret |
| `--tls` | false | Enable TLS |
| `--auto-tls` | false | Generate self-signed certificate |
| `--enable-profiling` | false | Enable pprof on port 6060 |

### Frontend Settings

Settings are available in the "Settings" menu:

- **Server URL** — backend server address

## Troubleshooting

### Connection Issues

**Server unavailable:**
- Check if the backend server is running
- Check the port (default 8889)
- Check the firewall

**No video:**
- Check if a file is selected for playback
- Wait for sufficient data to load
- Check mpv format support

### P2P Issues

**Cannot connect to room:**
- Check the room ID
- Check the password
- Make sure the host is online

**Desynchronization:**
- Check internet connection stability
- Try reconnecting to the room

### Logs

Backend server logs are written to stdout/stderr (configurable via `LOG_FORMAT` env var).

### Support

If the issue is not resolved, create an issue on GitHub with a problem description.
