import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const syncDuration = new Trend('sync_duration');

export const options = {
  scenarios: {
    p2p_sync_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '1m', target: 25 },
        { duration: '2m', target: 50 },
        { duration: '1m', target: 10 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'],
    sync_duration: ['p(95)<200'],
    errors: ['rate<0.02'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';

function getHeaders(vu) {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Bearer p2p-test-token-${vu}`,
  };
}

export default function () {
  const vu = __VU;
  
  group('Room Lifecycle', () => {
    // Create room
    const createRes = http.post(
      `${BASE_URL}/api/v1/rooms`,
      JSON.stringify({ name: `P2P Room ${vu}`, password: '' }),
      { headers: getHeaders(vu), insecureSkipTLSVerify: true }
    );
    check(createRes, { 'create room ok': (r) => r.status === 201 }) || errorRate.add(1);
    if (createRes.status !== 201) return;
    
    const roomId = createRes.json('roomId') || createRes.json('id');
    
    // Join room
    const joinRes = http.post(
      `${BASE_URL}/api/v1/rooms/join`,
      JSON.stringify({ roomId, password: '' }),
      { headers: getHeaders(vu), insecureSkipTLSVerify: true }
    );
    check(joinRes, { 'join room ok': (r) => r.status === 200 }) || errorRate.add(1);
    if (joinRes.status !== 200) return;
    
    // Sync operations (the core P2P sync)
    const syncStart = new Date();
    
    // Multiple rapid sync commands
    for (let i = 0; i < 10; i++) {
      const playRes = http.post(`${BASE_URL}/api/v1/sync/play`, null, {
        headers: getHeaders(vu),
        insecureSkipTLSVerify: true,
      });
      check(playRes, { 'play ok': (r) => r.status === 200 }) || errorRate.add(1);
      
      const pauseRes = http.post(`${BASE_URL}/api/v1/sync/pause`, null, {
        headers: getHeaders(vu),
        insecureSkipTLSVerify: true,
      });
      check(pauseRes, { 'pause ok': (r) => r.status === 200 }) || errorRate.add(1);
      
      const seekRes = http.post(
        `${BASE_URL}/api/v1/sync/seek`,
        JSON.stringify({ position: Math.random() * 3600 }),
        { headers: getHeaders(vu), insecureSkipTLSVerify: true }
      );
      check(seekRes, { 'seek ok': (r) => r.status === 200 }) || errorRate.add(1);
    }
    
    syncDuration.add(new Date() - syncStart);
    
    // WebRTC signaling
    for (let i = 0; i < 5; i++) {
      const signalRes = http.post(
        `${BASE_URL}/api/v1/rooms/signal`,
        JSON.stringify({
          type: i % 2 === 0 ? 'offer' : 'answer',
          from: `peer-${vu}`,
          to: `peer-${(vu + i) % 10}`,
          payload: { sdp: `v=0\r\no=- ${Date.now()} 2 IN IP4 127.0.0.1\r\n...` }
        }),
        { headers: getHeaders(vu), insecureSkipTLSVerify: true }
      );
      check(signalRes, { 'signal ok': (r) => r.status === 200 }) || errorRate.add(1);
    }
    
    // Leave room
    const leaveRes = http.post(`${BASE_URL}/api/v1/rooms/leave`, null, {
      headers: getHeaders(vu),
      insecureSkipTLSVerify: true,
    });
    check(leaveRes, { 'leave ok': (r) => r.status === 204 }) || errorRate.add(1);
  });
  
  sleep(Math.random() * 2 + 1);
}