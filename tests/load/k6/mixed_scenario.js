import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const listDuration = new Trend('list_duration');
const roomCreateDuration = new Trend('room_create_duration');
const syncDuration = new Trend('sync_duration');
const deleteDuration = new Trend('delete_duration');
const authDuration = new Trend('auth_duration');
const totalRequests = new Counter('total_requests');

// Test configuration — mixed realistic user journey.
//
// SCOPE CHANGE (was: Add torrent -> Get files -> Select file -> Stream).
// Adding a torrent requires metadata from real peers: POST /api/v1/torrents
// blocks until GotInfo() and returns 500 "timeout waiting for metadata" on a
// runner with no peers. That is not a load-test defect and cannot be fixed by
// throttling — the info hash in testMagnets belongs to nothing. Every step
// downstream of it (files, select, stream) was therefore unreachable, and the
// old journey short-circuited at `if (!torrentId) return;` before it measured
// anything at all.
//
// This journey covers what a CI runner can actually exercise: the authenticated
// list endpoint, room lifecycle, and the sync endpoints, plus one deliberate
// error path. Torrent *add* is the one thing load testing cannot cover here;
// verifying it needs a seeded swarm.
//
// The journey also used to send a fabricated token
// (`load-test-token-<vu>-<iter>`) that the JWT middleware rejected, so every
// authenticated call answered 401. setup() now registers a real user once and
// hands the JWT to every VU.
//
// Known simplification: all VUs share one account. The auth limiter
// (constants.AuthRateLimit, ~10/min) makes per-VU registration take minutes, and
// a load generator measures the server rather than login throughput. Concurrent
// room creation by one account means a VU's sync calls may target the room its
// session currently points at, which is fine for throughput measurement.
export const options = {
  // k6 v2 removed Params.insecureSkipTLSVerify (per-request); it now exists
  // only as a global option. The backend serves TLS with a self-signed cert
  // from --auto-tls, so without this every request fails with
  // "x509: certificate signed by unknown authority".
  insecureSkipTLSVerify: true,
  scenarios: {
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
    continuous_operations: {
      executor: 'constant-vus',
      vus: 10,
      duration: '10m',
      startTime: '30s',
      gracefulStop: '30s',
    },
  },
  thresholds: {
    // CALIBRATION RUN. These numbers are deliberately loose so the first
    // measurement can complete; they are replaced with real values derived
    // from that run's output before this job is allowed to gate anything.
    http_req_failed: ['rate<0.60'],
    errors: ['rate<0.60'],
    http_req_duration: ['p(95)<10000'],
    list_duration: ['p(95)<10000'],
    room_create_duration: ['p(95)<10000'],
    sync_duration: ['p(95)<10000'],
    delete_duration: ['p(95)<10000'],
    auth_duration: ['p(95)<10000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';
const TEST_USER = __ENV.LOADTEST_USER || 'loadtestuser';
const TEST_PASSWORD = 'LoadTestPass1!';

function jsonHeaders(token) {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
  };
}

// setup: wait for the backend, then obtain a real JWT.
//
// Registration is attempted once; if the account already exists the token is
// fetched by logging in instead. Either way every VU gets a usable token or
// the run aborts immediately instead of measuring 401s.
export function setup() {
  const health = http.get(`${BASE_URL}/health`, { timeout: '30s' });
  if (health.status !== 200) {
    throw new Error(`Backend not ready: ${health.status} (${health.error || 'no response'})`);
  }

  const credentials = JSON.stringify({ username: TEST_USER, password: TEST_PASSWORD });
  const anon = { 'Content-Type': 'application/json' };

  let res = http.post(`${BASE_URL}/api/v1/auth/register`, credentials, { headers: anon, timeout: '30s' });
  authDuration.add(res.timings.duration);

  if (res.status !== 201) {
    res = http.post(`${BASE_URL}/api/v1/auth/login`, credentials, { headers: anon, timeout: '30s' });
    authDuration.add(res.timings.duration);
  }

  let token = null;
  try {
    token = JSON.parse(res.body).token;
  } catch (e) {
    token = null;
  }
  if (!token) {
    throw new Error(`Could not obtain a token: register/login returned ${res.status} — ${res.body}`);
  }
  return { token };
}

export default function (data) {
  userJourney(data.token, __VU);
}

function userJourney(token, vu) {
  const headers = jsonHeaders(token);
  const iter = __ITER;

  // 1. List torrents — authenticated read of a paged envelope.
  group('List Torrents', () => {
    const res = http.get(`${BASE_URL}/api/v1/torrents`, { headers, timeout: '30s' });
    listDuration.add(res.timings.duration);
    totalRequests.add(1);
    check(res, {
      'list 200': (r) => r.status === 200,
      'list has totalCount': (r) => r.json('totalCount') !== undefined,
      'list has hasMore': (r) => r.json('hasMore') !== undefined,
    }) || errorRate.add(1);
  });

  sleep(1);

  // 2. Create a room.
  group('Create Room', () => {
    const res = http.post(
      `${BASE_URL}/api/v1/rooms`,
      JSON.stringify({ name: `LoadTest Room ${vu}-${iter}` }),
      { headers, timeout: '30s' }
    );
    roomCreateDuration.add(res.timings.duration);
    totalRequests.add(1);
    check(res, {
      'create room 201': (r) => r.status === 201,
      'has room id': (r) => (r.json('id') || r.json('roomId')) !== undefined,
    }) || errorRate.add(1);
  });

  sleep(1);

  // 3. Sync playback: play, seek, status, pause.
  // Creating the room already puts the caller in it, so no join step is needed.
  group('Sync Playback', () => {
    const play = http.post(`${BASE_URL}/api/v1/sync/play`, null, { headers, timeout: '30s' });
    syncDuration.add(play.timings.duration);
    totalRequests.add(1);
    check(play, { 'sync play 200': (r) => r.status === 200 }) || errorRate.add(1);

    sleep(1);

    const seek = http.post(
      `${BASE_URL}/api/v1/sync/seek`,
      JSON.stringify({ position: 30 }),
      { headers, timeout: '30s' }
    );
    syncDuration.add(seek.timings.duration);
    totalRequests.add(1);
    check(seek, {
      'sync seek 200': (r) => r.status === 200,
      'seek reports position': (r) => r.json('position') !== undefined,
    }) || errorRate.add(1);

    sleep(1);

    const status = http.get(`${BASE_URL}/api/v1/sync/status`, { headers, timeout: '30s' });
    syncDuration.add(status.timings.duration);
    totalRequests.add(1);
    check(status, {
      'sync status 200': (r) => r.status === 200,
      'status has timestamp': (r) => r.json('timestamp') !== undefined,
    }) || errorRate.add(1);

    sleep(1);

    const pause = http.post(`${BASE_URL}/api/v1/sync/pause`, null, { headers, timeout: '30s' });
    syncDuration.add(pause.timings.duration);
    totalRequests.add(1);
    check(pause, { 'sync pause 200': (r) => r.status === 200 }) || errorRate.add(1);
  });

  sleep(1);

  // 4. Deliberate error path: deleting an unknown torrent must be a 404, not a
  //    500 or a silent success. Torrent ids are 40 hex chars.
  group('Delete Unknown Torrent', () => {
    const res = http.delete(
      `${BASE_URL}/api/v1/torrents/0000000000000000000000000000000000000000`,
      { headers, timeout: '30s' }
    );
    deleteDuration.add(res.timings.duration);
    totalRequests.add(1);
    check(res, { 'delete unknown 404': (r) => r.status === 404 }) || errorRate.add(1);
  });

  sleep(1);

  // 5. Leave the room.
  group('Leave Room', () => {
    const res = http.post(`${BASE_URL}/api/v1/rooms/leave`, null, { headers, timeout: '30s' });
    totalRequests.add(1);
    check(res, { 'leave 200': (r) => r.status === 200 }) || errorRate.add(1);
  });
}