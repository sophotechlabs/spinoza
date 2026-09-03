import { request } from '@playwright/test';
import { statSync } from 'node:fs';
import { expect, side, state, test } from '../harness/test';
import { BASE_URL, TOKEN_FILE } from '../harness/paths';

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

test('the token is accepted in the header', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/version`, {
    headers: { 'X-Spinoza-Token': state().token },
  });
  expect(response.status()).toBe(200);
  await bare.dispose();
});

test('the token is accepted in the query, and comes back as a cookie', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/api/version?token=${state().token}`);
  expect(response.status()).toBe(200);
  const cookies = await bare.storageState();
  const minted = cookies.cookies.find((one) => one.name === 'spinoza_token');
  expect(minted?.value).toBe(state().token);
  expect(minted?.httpOnly).toBe(true);
  expect(minted?.sameSite).toBe('Strict');
  await bare.dispose();
});

test('the cookie the query minted is enough on its own', async () => {
  const bare = await request.newContext();
  await bare.get(`${BASE_URL}/api/version?token=${state().token}`);
  const response = await bare.get(`${BASE_URL}/api/version`);
  expect(response.status()).toBe(200);
  await bare.dispose();
});

test('the favicon is served without a token, and the api is not', async () => {
  const bare = await request.newContext();
  const icon = await bare.get(`${BASE_URL}/favicon.svg`);
  expect(icon.status()).toBe(200);
  const guarded = await bare.get(`${BASE_URL}/healthz`);
  expect(guarded.status()).toBe(401);
  await bare.dispose();
});

test('the profiler is not mounted unless it was asked for', async () => {
  const bare = await request.newContext();
  const response = await bare.get(`${BASE_URL}/debug/pprof/cmdline`, {
    headers: { 'X-Spinoza-Token': state().token },
  });
  const body = await response.text();
  expect(body).not.toContain('--kubeconfig');
  await bare.dispose();
});

test('the token file is readable only by the user that started spinoza', async () => {
  const mode = statSync(TOKEN_FILE).mode & 0o777;
  expect(mode.toString(8)).toBe('600');
});

test('the profiler mounts behind the same token when it is asked for', async () => {
  const profiled = side('profiled');
  const bare = await request.newContext();
  const refused = await bare.get(`${profiled.baseURL}/debug/pprof/`);
  expect(refused.status()).toBe(401);
  const allowed = await bare.get(`${profiled.baseURL}/debug/pprof/`, {
    headers: { 'X-Spinoza-Token': profiled.token },
  });
  expect(allowed.status()).toBe(200);
  expect(await allowed.text()).toContain('goroutine');
  await bare.dispose();
});
