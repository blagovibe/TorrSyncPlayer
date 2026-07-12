import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const addDuration = new Trend('torrent_add_duration');
const listDuration = new Trend('torrent_list_duration');

export const options = {
  scenarios: {
    torrent_load: {
      executor: 'constant-vus',
      vus: 20,
      duration: '3m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    errors: ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';

const magnets = Array.from({ length: 100 }, (_, i) => 
  `magnet:?xt=urn:btih:${String(i).padStart(40, '0')}&dn=Test${i}.mp4`
);

function getHeaders(vu) {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Bearer load-test-token-${vu}`,
  };
}

export default function () {
  const vu = __VU;
  const iter = __ITER;
  
  group('Add Torrent', () => {
    const magnet = magnets[iter % magnets.length];
    const start = new Date();
    
    const res = http.post(
      `${BASE_URL}/api/v1/torrents`,
      JSON.stringify({ magnetUri: magnet }),
      { headers: getHeaders(vu), insecureSkipTLSVerify: true }
    );
    
    addDuration.add(new Date() - start);
    check(res, { 'add ok': (r) => r.status === 201 }) || errorRate.add(1);
  });
  
  group('List Torrents', () => {
    const start = new Date();
    const res = http.get(`${BASE_URL}/api/v1/torrents`, {
      headers: getHeaders(vu),
      insecureSkipTLSVerify: true,
    });
    listDuration.add(new Date() - start);
    check(res, { 'list ok': (r) => r.status === 200 }) || errorRate.add(1);
  });
  
  sleep(1);
}