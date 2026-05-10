const { WebSocketServer } = require('ws');
const { v4: uuidv4 } = require('uuid');

const PORT = 8080;
const HEARTBEAT_INTERVAL_MS = 30000;
const HEARTBEAT_MISSES_LIMIT = 2;
const RECONNECT_GRACE_MS = 30000;
const ROOM_CODE_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
const ROOM_CODE_LENGTH = 6;

const wss = new WebSocketServer({ port: PORT });

// rooms: roomId -> { id, shortCode, peers: Map<peerId, PeerState>, joinOrder: string[] }
// PeerState = { id, ws, role, connected, disconnectedAt, reconnectTimer, missedPongs }
const rooms = new Map();
// shortCodeToRoomId: shortCode -> roomId
const shortCodeToRoomId = new Map();
// peerRooms: ws -> roomId
const peerRooms = new Map();
// disconnectedPeers: peerId -> roomId (только в grace-период)
const disconnectedPeers = new Map();

console.log('WS сервер запущен: ws://localhost:' + PORT);

wss.on('connection', function(ws) {
  var peerId = uuidv4();
  ws.peerId = peerId;
  ws.missedPongs = 0;
  
  console.log('Новое подключение: ' + peerId);

  ws.on('message', function(rawData) {
    try {
      var msg = JSON.parse(rawData);
      handleMessage(ws, msg);
    } catch (err) {
      console.error('Ошибка парсинга JSON:', err.message);
      ws.send(JSON.stringify({ type: 'error', message: 'Invalid JSON' }));
    }
  });

  ws.on('pong', function() {
    ws.missedPongs = 0;
  });

  ws.on('close', function() {
    console.log('Отключение: ' + ws.peerId);
    handleDisconnect(ws, false);
  });

  ws.on('error', function(err) {
    console.error('Ошибка WebSocket ' + ws.peerId + ':', err.message);
  });
});

function makeShortCode() {
  var code = '';
  for (var i = 0; i < ROOM_CODE_LENGTH; i += 1) {
    code += ROOM_CODE_ALPHABET.charAt(Math.floor(Math.random() * ROOM_CODE_ALPHABET.length));
  }
  return code;
}

function generateUniqueShortCode() {
  var attempts = 0;
  while (attempts < 10000) {
    var code = makeShortCode();
    if (!shortCodeToRoomId.has(code)) {
      return code;
    }
    attempts += 1;
  }
  throw new Error('Unable to generate unique room code');
}

function sendJson(ws, payload) {
  if (ws && ws.readyState === 1) {
    ws.send(JSON.stringify(payload));
  }
}

function resolveRoomId(rawRoomId) {
  if (!rawRoomId) return null;
  if (rooms.has(rawRoomId)) return rawRoomId;
  if (shortCodeToRoomId.has(rawRoomId)) return shortCodeToRoomId.get(rawRoomId);
  return null;
}

function assignRoles(room) {
  var activePeers = room.joinOrder
    .map(function(peerId) { return room.peers.get(peerId); })
    .filter(function(peer) { return !!peer && peer.connected; });

  var masterId = activePeers.length > 0 ? activePeers[0].id : null;
  var changed = false;
  room.peers.forEach(function(peer) {
    var nextRole = peer.id === masterId ? 'master' : 'slave';
    if (peer.role !== nextRole) {
      peer.role = nextRole;
      changed = true;
    }
  });
  return changed;
}

function broadcastRoleChanged(room) {
  var roles = [];
  room.peers.forEach(function(peer) {
    if (peer.connected) {
      roles.push({ peer_id: peer.id, role: peer.role });
    }
  });
  room.peers.forEach(function(peer) {
    if (peer.connected) {
      sendJson(peer.ws, { type: 'role_changed', peers: roles });
    }
  });
}

function broadcastPeerLeft(room, peerId) {
  room.peers.forEach(function(peer) {
    if (peer.connected && peer.id !== peerId) {
      sendJson(peer.ws, { type: 'peer_left', peer_id: peerId });
    }
  });
}

function finalizePeerLeave(roomId, peerId) {
  var room = rooms.get(roomId);
  if (!room) return;

  var peer = room.peers.get(peerId);
  if (!peer || peer.connected) return;

  if (peer.reconnectTimer) {
    clearTimeout(peer.reconnectTimer);
    peer.reconnectTimer = null;
  }

  room.peers.delete(peerId);
  room.joinOrder = room.joinOrder.filter(function(id) { return id !== peerId; });
  disconnectedPeers.delete(peerId);

  broadcastPeerLeft(room, peerId);

  if (assignRoles(room)) {
    broadcastRoleChanged(room);
  }

  if (room.peers.size === 0) {
    rooms.delete(roomId);
    shortCodeToRoomId.delete(room.shortCode);
    console.log('Комната удалена: ' + roomId);
  }
}

function handleDisconnect(ws, immediate) {
  var roomId = peerRooms.get(ws);
  if (!roomId) return;

  var room = rooms.get(roomId);
  if (!room) {
    peerRooms.delete(ws);
    return;
  }

  var peerId = ws.peerId;
  var peer = room.peers.get(peerId);
  peerRooms.delete(ws);
  if (!peer) return;

  peer.connected = false;
  peer.ws = null;
  peer.disconnectedAt = Date.now();

  if (assignRoles(room)) {
    broadcastRoleChanged(room);
  }

  if (immediate) {
    finalizePeerLeave(roomId, peerId);
    return;
  }

  disconnectedPeers.set(peerId, roomId);
  if (peer.reconnectTimer) {
    clearTimeout(peer.reconnectTimer);
  }
  peer.reconnectTimer = setTimeout(function() {
    finalizePeerLeave(roomId, peerId);
  }, RECONNECT_GRACE_MS);
}

function validateMessage(msg) {
  if (!msg || typeof msg !== 'object' || Array.isArray(msg)) {
    return 'Message must be a JSON object';
  }
  if (!msg.type || typeof msg.type !== 'string') {
    return 'Missing required field: type';
  }

  var requiredByType = {
    create_room: [],
    join_room: ['room_id'],
    offer: ['to', 'sdp'],
    answer: ['to', 'sdp'],
    ice: ['to', 'candidate'],
    sync: ['action', 'position'],
    leave_room: []
  };

  if (!Object.prototype.hasOwnProperty.call(requiredByType, msg.type)) {
    return null;
  }

  var required = requiredByType[msg.type];
  for (var i = 0; i < required.length; i += 1) {
    var key = required[i];
    if (msg[key] === undefined || msg[key] === null) {
      return 'Missing required field: ' + key;
    }
  }
  return null;
}

function tryRestorePeer(ws, msg) {
  var requestedPeerId = msg.peer_id;
  if (!requestedPeerId || typeof requestedPeerId !== 'string') return false;

  var roomId = disconnectedPeers.get(requestedPeerId);
  if (!roomId) return false;

  var room = rooms.get(roomId);
  if (!room) {
    disconnectedPeers.delete(requestedPeerId);
    return false;
  }

  if (msg.type === 'join_room' && resolveRoomId(msg.room_id) !== roomId) {
    return false;
  }

  var peer = room.peers.get(requestedPeerId);
  if (!peer || peer.connected) return false;

  if (peer.reconnectTimer) {
    clearTimeout(peer.reconnectTimer);
    peer.reconnectTimer = null;
  }

  ws.peerId = requestedPeerId;
  ws.missedPongs = 0;
  peer.ws = ws;
  peer.connected = true;
  peer.disconnectedAt = null;
  peer.missedPongs = 0;
  disconnectedPeers.delete(requestedPeerId);
  peerRooms.set(ws, roomId);

  room.peers.forEach(function(other) {
    if (other.connected && other.id !== requestedPeerId) {
      sendJson(other.ws, { type: 'peer_reconnected', peer_id: requestedPeerId });
    }
  });

  sendJson(ws, {
    type: 'reconnected',
    room_id: roomId,
    room_code: room.shortCode,
    peer_id: requestedPeerId,
    role: peer.role
  });

  console.log('Пир переподключился: ' + requestedPeerId + ' в комнату ' + roomId);
  return true;
}

function handleMessage(ws, msg) {
  var validationError = validateMessage(msg);
  if (validationError) {
    sendJson(ws, { type: 'error', message: validationError });
    return;
  }

  if (tryRestorePeer(ws, msg)) {
    return;
  }

  var type = msg.type;
  var room_id = msg.room_id;
  var to = msg.to;
  var sdp = msg.sdp;
  var candidate = msg.candidate;
  var action = msg.action;
  var position = msg.position;

  switch (type) {
    case 'pong': {
      ws.missedPongs = 0;
      break;
    }

    case 'create_room': {
      var roomId = uuidv4();
      var roomCode = generateUniqueShortCode();
      var room = {
        id: roomId,
        shortCode: roomCode,
        peers: new Map(),
        joinOrder: []
      };

      room.peers.set(ws.peerId, {
        id: ws.peerId,
        ws: ws,
        role: 'master',
        connected: true,
        disconnectedAt: null,
        reconnectTimer: null,
        missedPongs: 0
      });
      room.joinOrder.push(ws.peerId);
      rooms.set(roomId, room);
      shortCodeToRoomId.set(roomCode, roomId);
      peerRooms.set(ws, roomId);
      
      console.log('Комната создана: ' + roomId + ' (' + roomCode + ')');
      
      sendJson(ws, {
        type: 'room_created',
        room_id: roomId,
        room_code: roomCode,
        peer_id: ws.peerId,
        role: 'master'
      });
      break;
    }

    case 'join_room': {
      var roomId = resolveRoomId(room_id);
      var room = rooms.get(roomId);
      
      if (!room) {
        sendJson(ws, { type: 'error', message: 'Room not found' });
        return;
      }

      if (room.peers.has(ws.peerId)) {
        sendJson(ws, { type: 'error', message: 'Peer already in room' });
        return;
      }

      room.peers.set(ws.peerId, {
        id: ws.peerId,
        ws: ws,
        role: 'slave',
        connected: true,
        disconnectedAt: null,
        reconnectTimer: null,
        missedPongs: 0
      });
      room.joinOrder.push(ws.peerId);
      peerRooms.set(ws, roomId);
      
      var peers = [];
      room.peers.forEach(function(peer) {
        if (peer.id !== ws.peerId && peer.connected) {
          peers.push(peer.id);
          sendJson(peer.ws, {
            type: 'peer_joined',
            peer_id: ws.peerId
          });
        }
      });

      if (assignRoles(room)) {
        broadcastRoleChanged(room);
      }
      
      console.log('Пир ' + ws.peerId + ' присоединился к комнате ' + roomId);
      
      sendJson(ws, {
        type: 'joined',
        room_id: roomId,
        room_code: room.shortCode,
        peers: peers,
        peer_id: ws.peerId,
        role: room.peers.get(ws.peerId).role
      });
      break;
    }

    case 'leave_room': {
      var roomId = peerRooms.get(ws);
      if (roomId) {
        handleDisconnect(ws, true);
        sendJson(ws, { type: 'left_room' });
      }
      break;
    }

    case 'offer': {
      var roomId = peerRooms.get(ws);
      if (!roomId) {
        sendJson(ws, { type: 'error', message: 'Not in a room' });
        return;
      }
      
      var room = rooms.get(roomId);
      var targetPeer = room && room.peers.get(to);
      
      if (targetPeer && targetPeer.connected) {
        sendJson(targetPeer.ws, {
          type: 'offer',
          from: ws.peerId,
          sdp: sdp
        });
        console.log('offer: ' + ws.peerId + ' -> ' + to);
      } else {
        sendJson(ws, { type: 'error', message: 'Peer not found' });
      }
      break;
    }

    case 'answer': {
      var roomId = peerRooms.get(ws);
      if (!roomId) {
        sendJson(ws, { type: 'error', message: 'Not in a room' });
        return;
      }
      
      var room = rooms.get(roomId);
      var targetPeer = room && room.peers.get(to);
      
      if (targetPeer && targetPeer.connected) {
        sendJson(targetPeer.ws, {
          type: 'answer',
          from: ws.peerId,
          sdp: sdp
        });
        console.log('answer: ' + ws.peerId + ' -> ' + to);
      } else {
        sendJson(ws, { type: 'error', message: 'Peer not found' });
      }
      break;
    }

    case 'ice': {
      var roomId = peerRooms.get(ws);
      if (!roomId) {
        sendJson(ws, { type: 'error', message: 'Not in a room' });
        return;
      }
      
      var room = rooms.get(roomId);
      var targetPeer = room && room.peers.get(to);
      
      if (targetPeer && targetPeer.connected) {
        sendJson(targetPeer.ws, {
          type: 'ice',
          from: ws.peerId,
          candidate: candidate
        });
      } else {
        sendJson(ws, { type: 'error', message: 'Peer not found' });
      }
      break;
    }

    case 'sync': {
      var roomId = peerRooms.get(ws);
      if (!roomId) {
        sendJson(ws, { type: 'error', message: 'Not in a room' });
        return;
      }
      
      var room = rooms.get(roomId);
      var syncMsg = JSON.stringify({
        type: 'sync',
        from: ws.peerId,
        action: action,
        position: position,
        server_ts: Date.now()
      });
      
      room.peers.forEach(function(peer) {
        if (peer.id !== ws.peerId && peer.connected) {
          peer.ws.send(syncMsg);
        }
      });
      
      console.log('sync: ' + action + ' ' + position + ' от ' + ws.peerId);
      break;
    }

    default:
      sendJson(ws, { type: 'error', message: 'Unknown message type: ' + type });
  }
}

var heartbeatInterval = setInterval(function() {
  wss.clients.forEach(function(ws) {
    if (ws.readyState !== 1) return;

    ws.missedPongs = (ws.missedPongs || 0) + 1;
    if (ws.missedPongs >= HEARTBEAT_MISSES_LIMIT) {
      console.log('Heartbeat timeout: ' + ws.peerId);
      ws.terminate();
      return;
    }

    try {
      ws.ping();
      sendJson(ws, { type: 'ping', ts: Date.now() });
    } catch (err) {
      console.error('Ошибка ping для ' + ws.peerId + ':', err.message);
    }
  });
}, HEARTBEAT_INTERVAL_MS);

// Обработка сигнала для завершения
process.on('SIGINT', function() {
  console.log('\nЗавершение сервера...');
  clearInterval(heartbeatInterval);
  wss.close(function() {
    console.log('Сервер остановлен');
    process.exit(0);
  });
});
