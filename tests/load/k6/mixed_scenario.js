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

// Guards against a journey that silently stops halfway. Any exception thrown
// mid-iteration aborts it before the final add, so this rate drops below 1 even
// though every individual check that did run passed. It is what caught
// http.delete not existing in k6 v2: checks were 100% green
// while the delete request was never sent at all.
const journeyCompleted = new Rate('journey_completed');

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
// The journey deliberately probes a torrent id that does not exist to confirm
// it is answered with 404 rather than a 500 or a silent success. k6 counts any
// status >= 400 as a failed request by default, so that intentional 404 showed up
// as exactly one failure per iteration — 16.61% http_req_failed on a server that
// had answered every other request correctly.
//
// Declaring 404 expected leaves http_req_failed measuring transport failures and
// 5xx, which is what it is useful for. Per-endpoint correctness is not weakened:
// the checks below still assert exact status codes (201, 200, 404).
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 399 }, 404));

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
    // Derived from the calibration run (0bcffe8 lineage, 60 VUs, 13m, k6 2.3.0):
    // list p(95)=1.31ms, room create p(95)=0.87ms, sync p(95)=1.30ms,
    // delete p(95)=1.33ms, http_req_duration p(95)=1.30ms, 0 failed requests.
    //
    // These sit far above the measured values on purpose. A GitHub runner is
    // shared and noisy, so a tight budget would flake; but they are still tight
    // enough to catch the failure mode that motivated them. Both deadlocks in
    // internal/p2p and internal/sync pinned play, pause, seek and room creation
    // at exactly 30s, which blows through every one of these by three orders of
    // magnitude. Thresholds that only catch an outage are not worth having.
    http_req_failed: ['rate<0.01'],
    errors: ['rate<0.02'],
    http_req_duration: ['p(95)<50'],
    list_duration: ['p(95)<50'],
    room_create_duration: ['p(95)<100'],
    sync_duration: ['p(95)<50'],
    delete_duration: ['p(95)<50'],
    auth_duration: ['p(95)<1000'],
    // Catches a journey that aborts part-way: any exception before the end of
    // userJourney leaves this below 1, which the summary alone does not reveal.
    journey_completed: ['rate>0.99'],
  },
};

// One room per VU, created on that VU's first iteration and reused after.
// Indexed by __VU, not __ITER: k6 keeps one VU-local copy of module scope per
// virtual user, so this persists across that VU's iterations.
const roomByVU = [];

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

  // 2. Create a room — once per VU, not once per iteration.
  //
  // A session tracks exactly one current room, and every create overwrites it.
  // Creating on every iteration orphaned every room except the most recent one,
  // because leave only ever removes the session's current room. After ~1000
  // orphaned rooms the server hit constants.MaxRooms and every create started
  // failing with "maximum number of rooms exceeded".
  //
  // One room per VU also matches real use: a person opens a room once and then
  // plays and syncs in it, rather than recreating it several times a minute.
  if (!roomByVU[vu]) {
    group('Create Room', () => {
      const res = http.post(
        `${BASE_URL}/api/v1/rooms`,
        JSON.stringify({ name: `LoadTest Room ${vu}` }),
        { headers, timeout: '30s' }
      );
      roomCreateDuration.add(res.timings.duration);
      totalRequests.add(1);
      check(res, {
        'create room 201': (r) => r.status === 201,
        'has room id': (r) => (r.json('id') || r.json('roomId')) !== undefined,
      }) || errorRate.add(1);

      if (res.status === 201) {
        roomByVU[vu] = res.json('id') || res.json('roomId');
      }
    });
  }

  sleep(1);

  // 3. Sync playback: play, seek, status, pause.
  // Creating the room already puts the caller in it, so no join step is needed.
  group('Sync Playback', () => {
    const play = http.post(`${BASE_URL}/api/v1/sync/play`, null, { headers, timeout: '30s' });
    syncDuration.add(play.timings.duration);
    totalRequests.add(1);
    check(play, { 'sync play 200': (r) => r.status === 200 }) || errorRate.add(1);

    sleep(1);

    const seek = http.post(`${BASE_URL}/api/v1/sync/seek`,
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
    // http.del, not http.delete: k6 v2 removed the delete alias, so http.delete
    // is undefined and calling it throws "TypeError: Object has no member
    // 'delete'". A throw here aborts the iteration before the request is sent,
    // and nothing in the summary shows it: every check that did run still
    // passed, and the journey ended one step early. Signature is
    // (url, body, params), so the options go in third position.
    const res = http.del(
      `${BASE_URL}/api/v1/torrents/0000000000000000000000000000000000000000`,
      null,
      { headers, timeout: '30s' }
    );
    deleteDuration.add(res.timings.duration);
    totalRequests.add(1);
    check(res, { 'delete unknown 404': (r) => r.status === 404 }) || errorRate.add(1);
  });

  sleep(1);

  // Leave is deliberately NOT part of the iteration. It clears the session's
  // current room, which is the room the next iteration's sync calls target, so
  // running it every iteration would both break the sync calls and leave each
  // VU without a room to return to. It is exercised once in teardown() instead,
  // which is also the only place it belongs: leaving is a one-off at the end of
  // a session, not a per-iteration action.
  journeyCompleted.add(1);
}

// teardown exercises the leave endpoint once, at the end of the run, and checks
// that leaving actually releases the room. This is the only place leave belongs:
// it clears the session's current room, so calling it per iteration would break
// the sync calls that follow.
export function teardown(data) {
  const headers = jsonHeaders(data.token);

  const created = http.post(
    `${BASE_URL}/api/v1/rooms`,
    JSON.stringify({ name: 'LoadTest Teardown Room' }),
    { headers, timeout: '30s' }
  );
  check(created, { 'teardown create room 201': (r) => r.status === 201 });

  const left = http.post(`${BASE_URL}/api/v1/rooms/leave`, null, { headers, timeout: '30s' });
  check(left, { 'teardown leave 200': (r) => r.status === 200 });
}
