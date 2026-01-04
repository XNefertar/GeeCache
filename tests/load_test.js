import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const cacheHits = new Counter('cache_hits');
const cacheMisses = new Counter('cache_misses');
const errorRate = new Rate('errors');
const responseTime = new Trend('response_time');

// Test configuration
export const options = {
  stages: [
    { duration: '1m', target: 50 },   // Ramp up to 50 VUs
    { duration: '3m', target: 100 },  // Ramp up to 100 VUs
    { duration: '2m', target: 200 },  // Spike to 200 VUs (stampede simulation)
    { duration: '2m', target: 100 },  // Scale back down
    { duration: '1m', target: 0 },    // Ramp down to 0
  ],
  thresholds: {
    'http_req_duration': ['p(95)<500', 'p(99)<1000'], // 95% < 500ms, 99% < 1s
    'errors': ['rate<0.05'], // Error rate should be less than 5%
  },
};

// Environment variables
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const GROUP_NAME = __ENV.GROUP_NAME || 'scores';

// Zipfian distribution parameters (80-20 rule)
// In a Zipfian distribution, a small number of keys account for most requests
const TOTAL_KEYS = 10000;
const HOT_KEYS = 100; // Top 100 keys account for 80% of traffic
const HOT_KEY_PROBABILITY = 0.8;

// Generate key following Zipfian distribution (simplified)
function generateKey() {
  const rand = Math.random();
  
  if (rand < HOT_KEY_PROBABILITY) {
    // 80% of requests go to hot keys (top 100)
    const keyId = Math.floor(Math.random() * HOT_KEYS);
    return `hot_key_${keyId}`;
  } else {
    // 20% of requests go to cold keys
    const keyId = Math.floor(Math.random() * TOTAL_KEYS);
    return `cold_key_${keyId}`;
  }
}

// Simulate cache stampede - many requests for the same key at once
function cacheStampedeScenario() {
  const sameKey = 'stampede_key_' + Math.floor(Date.now() / 10000); // Changes every 10s
  return sameKey;
}

export default function () {
  // Scenario selection based on iteration
  let key;
  const scenario = Math.random();
  
  if (scenario < 0.2) {
    // 20% cache stampede scenario
    key = cacheStampedeScenario();
  } else {
    // 80% normal Zipfian distribution
    key = generateKey();
  }

  const url = `${BASE_URL}/_geecache/${GROUP_NAME}/${key}`;
  
  const startTime = Date.now();
  const response = http.get(url);
  const duration = Date.now() - startTime;
  
  responseTime.add(duration);

  // Check response
  const success = check(response, {
    'status is 200 or 404': (r) => r.status === 200 || r.status === 404,
    'response time < 1000ms': (r) => r.timings.duration < 1000,
  });

  if (!success) {
    errorRate.add(1);
  } else {
    errorRate.add(0);
  }

  // Track cache hits vs misses
  // In a real scenario, you'd check response headers or logs
  // For now, we assume faster responses are cache hits
  if (duration < 50 && response.status === 200) {
    cacheHits.add(1);
  } else if (response.status === 200) {
    cacheMisses.add(1);
  }

  // Think time - random sleep between requests
  sleep(Math.random() * 0.5); // 0-500ms
}

// Setup function - runs once at the start
export function setup() {
  console.log(`Starting load test against ${BASE_URL}`);
  console.log(`Target group: ${GROUP_NAME}`);
  console.log(`Test will simulate Zipfian distribution with cache stampedes`);
  
  // Pre-populate some hot keys
  console.log('Pre-populating hot keys...');
  for (let i = 0; i < 10; i++) {
    const key = `hot_key_${i}`;
    const url = `${BASE_URL}/_geecache/${GROUP_NAME}/${key}`;
    http.get(url);
  }
  
  return { startTime: Date.now() };
}

// Teardown function - runs once at the end
export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log(`Load test completed in ${duration}s`);
}

// Handle summary for custom output
export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'summary.json': JSON.stringify(data),
  };
}

function textSummary(data, options) {
  // Custom summary generation
  let summary = '\n';
  summary += '='.repeat(60) + '\n';
  summary += 'GeeCache Load Test Summary\n';
  summary += '='.repeat(60) + '\n\n';
  
  summary += `Total Requests: ${data.metrics.http_reqs.values.count}\n`;
  summary += `Cache Hits: ${data.metrics.cache_hits ? data.metrics.cache_hits.values.count : 'N/A'}\n`;
  summary += `Cache Misses: ${data.metrics.cache_misses ? data.metrics.cache_misses.values.count : 'N/A'}\n`;
  summary += `Error Rate: ${(data.metrics.errors.values.rate * 100).toFixed(2)}%\n\n`;
  
  summary += 'Response Time:\n';
  summary += `  p50: ${data.metrics.http_req_duration.values['p(50)']}ms\n`;
  summary += `  p95: ${data.metrics.http_req_duration.values['p(95)']}ms\n`;
  summary += `  p99: ${data.metrics.http_req_duration.values['p(99)']}ms\n`;
  summary += `  max: ${data.metrics.http_req_duration.values.max}ms\n\n`;
  
  summary += '='.repeat(60) + '\n';
  
  return summary;
}
