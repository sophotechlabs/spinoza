import { request } from '@playwright/test';
import { expect, test } from '../harness/test';
import { BASE_URL } from '../harness/paths';

test.use({ storageState: { cookies: [], origins: [] } });

test('a request without the token is refused', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/overview`);
  expect(response.status()).toBe(401);
  await bare.dispose();
});

test('a request with the wrong token is refused', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/overview`, {
    headers: { 'X-Spinoza-Token': 'not-the-token' },
  });
  expect(response.status()).toBe(401);
  await bare.dispose();
});

test('a non-local Host header is rejected', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/overview`, {
    headers: { Host: 'evil.example.com' },
  });
  expect(response.status()).toBeGreaterThanOrEqual(400);
  await bare.dispose();
});

test('a websocket without the token is refused', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/resources`, {
    headers: { Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13' },
  });
  expect(response.status()).toBeGreaterThanOrEqual(400);
  await bare.dispose();
});

test('the exec socket without the token is refused', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/exec`, {
    headers: { Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13' },
  });
  expect(response.status()).toBeGreaterThanOrEqual(400);
  await bare.dispose();
});

test('a non-local Origin header is rejected', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/overview`, {
    headers: { Origin: 'https://evil.example.com' },
  });
  expect(response.status()).toBeGreaterThanOrEqual(400);
  await bare.dispose();
});
