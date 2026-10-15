import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const listDuration = new Trend('list_duration');
const bufferDuration = new Trend('buffer_duration');
const magnetDuration = new Trend('magnet_duration');
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
// The token is now the startup token; setup() verifies the backend is alive and
// passes it to every VU. No registration or login is needed.
//
// Known simplification: all VUs share one client id. A load generator measures
// the server rather than login throughput, and the startup token is identical
// for every caller by design.
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
    buffer_duration: ['p(95)<50'],
    magnet_duration: ['p(95)<50'],
    delete_duration: ['p(95)<50'],
    auth_duration: ['p(95)<1000'],
    // Catches a journey that aborts part-way: any exception before the end of
    // userJourney leaves this below 1, which the summary alone does not reveal.
    journey_completed: ['rate>0.99'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';
const ACCESS_TOKEN = __ENV.ACCESS_TOKEN;

// Load testing cannot seed a swarm, so the journey probes a torrent id that
// describes nothing and expects 404s: endpoints must refuse unknown torrents,
// not fail on them.
const LOAD_TEST_TORRENT_ID = '0000000000000000000000000000000000000000';

function jsonHeaders() {
  return {
    'Content-Type': 'application/json',
    'X-Access-Token': ACCESS_TOKEN,
    'X-Client-ID': `k6-vu-${__VU}`,
  };
}

// setup: wait for the backend to be ready.
//
// The access token is generated by the server at startup and printed to its
// log. Pass it in as ACCESS_TOKEN; there is nothing to register or log in to.
export function setup() {
  const health = http.get(`${BASE_URL}/health`, { timeout: '30s' });
  if (health.status !== 200) {
    throw new Error(`Backend not ready: ${health.status} (${health.error || 'no response'})`);
  }
  if (!ACCESS_TOKEN) {
    throw new Error('ACCESS_TOKEN is required — copy the token from the backend startup log');
  }
  return { token: ACCESS_TOKEN };
}

export default function (data) {
  userJourney(data.token, __VU);
}

function userJourney(token, vu) {
  const headers = jsonHeaders();

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

  // 2. Buffer heartbeat — the 2 Hz position report every playing client
  // sends, and the buffer state read beside it. Playback commands travel peer
  // to peer now, so these are the only per-second endpoints the server keeps
  // (idea-0064). Unknown torrent ids answer 404: the heartbeat of a removed
  // torrent is an ordinary state, not a failure.
  group('Buffer Heartbeat', () => {
    const report = http.post(`${BASE_URL}/api/v1/torrents/${LOAD_TEST_TORRENT_ID}/buffer/position`,
      JSON.stringify({ position: 1048576 }),
      { headers, timeout: '30s' }
    );
    bufferDuration.add(report.timings.duration);
    totalRequests.add(1);
    check(report, { 'buffer position 404 for unknown torrent': (r) => r.status === 404 }) || errorRate.add(1);

    sleep(1);

    const info = http.get(`${BASE_URL}/api/v1/torrents/${LOAD_TEST_TORRENT_ID}/buffer/info`, { headers, timeout: '30s' });
    bufferDuration.add(info.timings.duration);
    totalRequests.add(1);
    check(info, { 'buffer info 404 for unknown torrent': (r) => r.status === 404 }) || errorRate.add(1);
  });

  sleep(1);

  // 3. Magnet for an unknown torrent: 404, not 500. The host announces its own
  // film from this endpoint, so a missing torrent here would strand a room
  // before it starts.
  group('Magnet Lookup', () => {
    const res = http.get(`${BASE_URL}/api/v1/torrents/${LOAD_TEST_TORRENT_ID}/magnet`, { headers, timeout: '30s' });
    magnetDuration.add(res.timings.duration);
    totalRequests.add(1);
    check(res, { 'magnet 404 for unknown torrent': (r) => r.status === 404 }) || errorRate.add(1);
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

  journeyCompleted.add(1);
}

// teardown lists once more at the end of the run: the rooms it used to create
// and leave are gone with the broker, so there is nothing to release.
export function teardown(data) {
  const headers = jsonHeaders();

  const res = http.get(`${BASE_URL}/api/v1/torrents`, { headers, timeout: '30s' });
  check(res, { 'teardown list 200': (r) => r.status === 200 });
}
