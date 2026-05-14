---
title: PeerJS Signaling
created: 2026-05-15
updated: 2026-05-15
type: concept
tags: [signaling, p2p, webrtc, architecture]
sources: [raw/articles/p2p-service-source.md, raw/articles/spec-source.md]
---

# PeerJS Signaling

Сигналинг для установления P2P-соединений через WebRTC.

## Обзор

PeerJS — библиотека для упрощения WebRTC. Использует центральный сигналинг-сервер для обмена SDP offers/answers между пирами, после чего устанавливается прямое P2P-соединение через WebRTC data channels.

## Топология

```
[Host] ←→ [PeerJS Signaling Server (0.peerjs.com)] ←→ [Guest]
   ↓                                                          ↓
   └──────────── WebRTC Data Channel (P2P) ─────────────────┘
```

## Peer ID

- **Host**: `torrsync-{random 6 chars}` — префикс позволяет идентифицировать хосты
- **Guest**: случайный ID от PeerJS сервера
- Room code = peer ID хоста (без префикса `torrsync-`)

## Конфигурация

По умолчанию: `0.peerjs.com:443/` (WSS). Настраивается через env:
- `VITE_PEERJS_HOST`
- `VITE_PEERJS_PORT`
- `VITE_PEERJS_PATH`
- `VITE_PEERJS_SECURE`

## Ограничения

- Один сигналинг-сервер — единая точка отказа
- Максимум одно активное соединение (1 host ↔ 1 guest)

## Будущее

SPEC.md предусматривает self-hosted signaling на Go в `server/`, но текущая реализация использует только PeerJS cloud.

## Связи

- [[P2PService]] — реализация
- [[Playback Synchronization Protocol]] — использует data channel
- [[App]] — инициализация и управление
