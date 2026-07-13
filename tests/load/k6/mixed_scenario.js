import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const torrentAddDuration = new Trend('torrent_add_duration');
const roomCreateDuration = new Trend('room_create_duration');
const syncDuration = new Trend('sync_duration');
const streamDuration = new Trend('stream_duration');
const authDuration = new Trend('auth_duration');
const totalRequests = new Counter('total_requests');

// Test configuration - mixed realistic user journey
export const options = {
  scenarios: {
    // Main load test: realistic user journeys
    mixed_scenario: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 5 },    // Warm up
        { duration: '3m', target: 20 },   // Normal load
        { duration: '5m', target: 50 },   // Peak load
        { duration: '3m', target: 20 },   // Cool down
        { duration: '1m', target: 0 },    // Ramp down
      ],
      gracefulRampDown: '30s',
    },
    // Background continuous operations
    continuous_operations: {
      executor: 'constant-vus',
      vus: 10,
      duration: '10m',
      startTime: '30s',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<800', 'p(99)<1500'],
    http_req_failed: ['rate<0.02'],
    errors: ['rate<0.05'],
    torrent_add_duration: ['p(95)<3000'],
    room_create_duration: ['p(95)<1500'],
    sync_duration: ['p(95)<300'],
    stream_duration: ['p(95)<8000'],
    auth_duration: ['p(95)<1000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';
const JWT_SECRET = __ENV.JWT_SECRET || 'test-jwt-secret-key-for-load-testing-min-32-chars';

// Test data
const testMagnets = [
  'magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=Movie1.mp4',
  'magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=Movie2.mkv',
  'magnet:?xt=urn:btih:3333333333333333333333333333333333333333&dn=Show.S01E01.mp4',
  'magnet:?xt=urn:btih:4444444444444444444444444444444444444444&dn=Movie3.avi',
  'magnet:?xt=urn:btih:5555555555555555555555555555555555555555&dn=Movie4.mp4',
];

function getHeaders(token) {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };
}

// Simulate user authentication
function authenticate(vu) {
  const start = new Date();
  
  // In real test, would register/login
  // For load test, use pre-generated token
  const token = `load-test-token-${vu}-${__ITER}`;
  
  authDuration.add(new Date() - start);
  return token;
}

// User journey: Add torrent -> Create room -> Sync playback -> Stream -> Leave
function userJourney(token, vu) {
  const magnet = testMagnets[__ITER % testMagnets.length];
  let torrentId = null;
  let roomId = null;
  
  // 1. Add torrent
  group('Add Torrent', () => {
    const start = new Date();
    const res = http.post(
      `${BASE_URL}/api/v1/torrents`,
      JSON.stringify({ magnetUri: magnet }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    torrentAddDuration.add(new Date() - start);
    totalRequests.add(1);
    
    check(res, {
      'add torrent 201': (r) => r.status === 201,
      'has torrent id': (r) => r.json('id') !== undefined,
    }) || errorRate.add(1);
    
    if (res.status === 201) {
      torrentId = res.json('id');
    }
  });
  
  if (!torrentId) return;
  
  sleep(1);
  
  // 2. List torrents
  group('List Torrents', () => {
    const res = http.get(`${BASE_URL}/api/v1/torrents`, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    totalRequests.add(1);
    check(res, { 'list 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(1);
  
  // 3. Get torrent files
  group('Get Files', () => {
    const res = http.get(`${BASE_URL}/api/v1/torrents/${torrentId}/files`, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    totalRequests.add(1);
    check(res, { 'files 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(1);
  
  // 4. Select file for streaming
  group('Select File', () => {
    const res = http.post(
      `${BASE_URL}/api/v1/torrents/${torrentId}/select`,
      JSON.stringify({ fileIndex: 0 }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    totalRequests.add(1);
    check(res, { 'select 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(2);
  
  // 5. Create room
  group('Create Room', () => {
    const start = new Date();
    const res = http.post(
      `${BASE_URL}/api/v1/rooms`,
      JSON.stringify({ name: `LoadTest Room ${vu}-${__ITER}`, password: '' }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    roomCreateDuration.add(new Date() - start);
    totalRequests.add(1);
    
    check(res, {
      'create room 201': (r) => r.status === 201,
      'has room id': (r) => r.json('roomId') || r.json('id'),
    }) || errorRate.add(1);
    
    if (res.status === 201) {
      roomId = res.json('roomId') || res.json('id');
    }
  });
  
  if (!roomId) return;
  
  sleep(1);
  
  // 6. Join room (simulate same user as host)
  group('Join Room', () => {
    const res = http.post(
      `${BASE_URL}/api/v1/rooms/join`,
      JSON.stringify({ roomId, password: '' }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    totalRequests.add(1);
    check(res, { 'join room 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(2);
  
  // 7. Sync operations (play, pause, seek)
  group('Sync Playback', () => {
    const start = new Date();
    
    const playRes = http.post(`${BASE_URL}/api/v1/sync/play`, null, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    check(playRes, { 'sync play 200': (r) => r.status === 200 }) || errorRate.add(1);
    totalRequests.add(1);
    
    sleep(1);
    
    const pauseRes = http.post(`${BASE_URL}/api/v1/sync/pause`, null, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    check(pauseRes, { 'sync pause 200': (r) => r.status === 200 }) || errorRate.add(1);
    totalRequests.add(1);
    
    sleep(1);
    
    const seekRes = http.post(
      `${BASE_URL}/api/v1/sync/seek`,
      JSON.stringify({ position: Math.random() * 3600 }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    check(seekRes, { 'sync seek 200': (r) => r.status === 200 }) || errorRate.add(1);
    totalRequests.add(1);
    
    syncDuration.add(new Date() - start);
  });
  
  sleep(2);
  
  // 8. Stream video (range request)
  group('Stream Video', () => {
    // Get stream URL first
    const selectRes = http.post(
      `${BASE_URL}/api/v1/torrents/${torrentId}/select`,
      JSON.stringify({ fileIndex: 0 }),
      { headers: getHeaders(token), insecureSkipTLSVerify: true }
    );
    
    if (selectRes.status === 200 && selectRes.json('streamUrl')) {
      const streamUrl = selectRes.json('streamUrl');
      const start = new Date();
      
      // Initial range request
      const res = http.get(streamUrl, {
        headers: { ...getHeaders(token), 'Range': 'bytes=0-1048575' },
        insecureSkipTLSVerify: true,
      });
      
      streamDuration.add(new Date() - start);
      totalRequests.add(1);
      check(res, { 'stream 206/200': (r) => r.status === 206 || r.status === 200 }) || errorRate.add(1);
    }
  });
  
  sleep(3);
  
  // 9. Send WebRTC signals
  group('WebRTC Signaling', () => {
    for (let i = 0; i < 3; i++) {
      const res = http.post(
        `${BASE_URL}/api/v1/rooms/signal`,
        JSON.stringify({
          type: ['offer', 'answer', 'candidate'][i],
          from: `peer-${vu}`,
          to: `peer-${(vu + i) % 10 + 1}`,
          payload: { sdp: 'v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n' }
        }),
        { headers: getHeaders(token), insecureSkipTLSVerify: true }
      );
      totalRequests.add(1);
      check(res, { 'signal 200': (r) => r.status === 200 }) || errorRate.add(1);
      sleep(0.5);
    }
  });
  
  sleep(1);
  
  // 10. Get sync status
  group('Sync Status', () => {
    const res = http.get(`${BASE_URL}/api/v1/sync/status`, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    totalRequests.add(1);
    check(res, { 'sync status 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(1);
  
  // 11. Leave room
  group('Leave Room', () => {
    const res = http.post(`${BASE_URL}/api/v1/rooms/leave`, null, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    totalRequests.add(1);
    check(res, { 'leave room 204': (r) => r.status === 204 }) || errorRate.add(1);
  });
  
  sleep(1);
  
  // 12. Remove torrent
  group('Remove Torrent', () => {
    const res = http.del(`${BASE_URL}/api/v1/torrents/${torrentId}`, null, {
      headers: getHeaders(token),
      insecureSkipTLSVerify: true,
    });
    totalRequests.add(1);
    check(res, { 'delete torrent 204': (r) => r.status === 204 }) || errorRate.add(1);
  });
}

// Setup - verify backend is ready
export function setup() {
  const res = http.get(`${BASE_URL}/health`, {
    insecureSkipTLSVerify: true,
    timeout: '30s',
  });
  
  check(res, { 'health check ok': (r) => r.status === 200 });
  
  if (res.status !== 200) {
    throw new Error(`Backend not ready: ${res.status}`);
  }
  
  return { baseUrl: BASE_URL };
}

// Main test function
export default function (data) {
  const token = authenticate(__VU);
  userJourney(token, __VU);
}

// Teardown
export function teardown(data) {
  // Cleanup if needed
}