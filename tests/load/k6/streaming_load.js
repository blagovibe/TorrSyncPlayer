import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const errorRate = new Rate('errors');
const streamStartDuration = new Trend('stream_start_duration');
const rangeRequestDuration = new Trend('range_request_duration');

export const options = {
  scenarios: {
    streaming_load: {
      executor: 'constant-arrival-rate',
      rate: 50, // 50 requests per second
      timeUnit: '1s',
      duration: '3m',
      preAllocatedVUs: 50,
      maxVUs: 100,
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<2000'],
    stream_start_duration: ['p(95)<1000'],
    range_request_duration: ['p(99)<500'],
    errors: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8889';

const torrentIds = Array.from({ length: 50 }, (_, i) => `torrent-${String(i).padStart(3, '0')}`);

function getHeaders(vu) {
  return {
    'Authorization': `Bearer stream-test-token-${vu}`,
  };
}

export default function () {
  const vu = __VU;
  const iter = __ITER;
  
  group('Stream Video', () => {
    const torrentId = torrentIds[iter % torrentIds.length];
    
    // 1. Get stream URL (select file)
    const selectStart = new Date();
    const selectRes = http.post(
      `${BASE_URL}/api/v1/torrents/${torrentId}/select`,
      JSON.stringify({ fileIndex: 0 }),
      { headers: getHeaders(vu), insecureSkipTLSVerify: true }
    );
    check(selectRes, { 'select ok': (r) => r.status === 200 }) || errorRate.add(1);
    if (selectRes.status !== 200) return;
    
    const streamUrl = selectRes.json('streamUrl');
    if (!streamUrl) {
      errorRate.add(1);
      return;
    }
    streamStartDuration.add(new Date() - selectStart);
    
    // 2. Initial range request (start of video)
    const rangeStart = new Date();
    const rangeRes = http.get(streamUrl, {
      headers: {
        ...getHeaders(vu),
        'Range': 'bytes=0-1048575', // First 1MB
      },
      insecureSkipTLSVerify: true,
    });
    rangeRequestDuration.add(new Date() - rangeStart);
    check(rangeRes, { 
      'range ok': (r) => r.status === 206 || r.status === 200,
      'content-range': (r) => r.headers['Content-Range'] !== undefined || r.status === 200,
    }) || errorRate.add(1);
    
    // 3. Subsequent range requests (simulate seeking)
    for (let i = 1; i < 5; i++) {
      const seekStart = new Date();
      const seekRange = i * 1048576; // 1MB chunks
      const seekRes = http.get(streamUrl, {
        headers: {
          ...getHeaders(vu),
          'Range': `bytes=${seekRange}-${seekRange + 1048575}`,
        },
        insecureSkipTLSVerify: true,
      });
      rangeRequestDuration.add(new Date() - seekStart);
      check(seekRes, { 'seek range ok': (r) => r.status === 206 }) || errorRate.add(1);
      sleep(0.1); // Small delay between chunks
    }
    
    // 4. Random seek (simulate user scrubbing)
    const randomPos = Math.floor(Math.random() * 100) * 1048576;
    const randomRes = http.get(streamUrl, {
      headers: {
        ...getHeaders(vu),
        'Range': `bytes=${randomPos}-${randomPos + 1048575}`,
      },
      insecureSkipTLSVerify: true,
    });
    check(randomRes, { 'random seek ok': (r) => r.status === 206 }) || errorRate.add(1);
  });
  
  sleep(Math.random() * 3 + 1);
}