import { execFileSync } from 'node:child_process';
import { expect } from '@playwright/test';
import type { Page } from '@playwright/test';

let configuredBaseURL = process.env.SPINOZA_CM_BASE_URL;
if (configuredBaseURL === undefined || configuredBaseURL === '') {
  configuredBaseURL = 'https://spinoza.localtest.me:8443';
}
export const BASE_URL = configuredBaseURL;

interface Scope {
  everywhere: boolean;
  namespaces?: string[];
  undecided?: string[];
}

export interface Session {
  authenticated: boolean;
  groups?: string[];
  mode: string;
  role: string;
  scope: Scope;
  user?: string;
}

export interface ApiResult {
  status: number;
  body: string;
}

function context(): string {
  const held = process.env.SPINOZA_CM_CONTEXT;
  if (held === undefined || held === '') {
    throw new Error('SPINOZA_CM_CONTEXT is required');
  }
  return held;
}

export function kubectl(args: string[]): string {
  return execFileSync('kubectl', ['--context', context(), ...args], {
    encoding: 'utf8',
    timeout: 600_000,
  });
}

export async function signIn(page: Page, user: string): Promise<void> {
  await page.goto(BASE_URL);
  const link = page.getByTestId('sign-in');
  const username = page.locator('#username');
  await expect
    .poll(async () => {
      if (await username.isVisible()) {
        return true;
      }
      return link.isVisible();
    })
    .toBe(true);
  if (await link.isVisible()) {
    const href = await link.getAttribute('href');
    if (href === null) {
      throw new Error('the sign-in link has no destination');
    }
    await page.goto(new URL(href, BASE_URL).toString());
  }
  await expect(username).toBeVisible();
  await username.fill(user);
  await page.locator('#password').fill(user);
  await page.locator('#kc-login').click();
  await expect
    .poll(() => authenticated(page))
    .toBe(true);
  await expect(page.locator('summary[aria-label="Account"]')).toBeVisible();
}

export async function session(page: Page, headers: Record<string, string> = {}): Promise<Session> {
  return page.evaluate(async (requestHeaders) => {
    const response = await fetch('/api/auth/me', { headers: requestHeaders });
    return (await response.json()) as Session;
  }, headers);
}

export async function authenticated(page: Page): Promise<boolean> {
  try {
    return (await session(page)).authenticated;
  } catch {
    return false;
  }
}

export async function api(
  page: Page,
  method: string,
  path: string,
  payload = '',
  headers: Record<string, string> = {},
): Promise<ApiResult> {
  return page.evaluate(
    async ({ requestMethod, requestPath, requestPayload, requestHeaders }) => {
      const init: RequestInit = { method: requestMethod, headers: requestHeaders };
      if (requestPayload !== '') {
        init.body = requestPayload;
      }
      const response = await fetch(requestPath, init);
      return { status: response.status, body: await response.text() };
    },
    {
      requestMethod: method,
      requestPath: path,
      requestPayload: payload,
      requestHeaders: headers,
    },
  );
}

export async function openFeed(page: Page): Promise<void> {
  await page.evaluate(async () => {
    const held = window as typeof window & {
      clusterModeSocket?: WebSocket;
      clusterModeSocketClosed?: boolean;
    };
    await new Promise<void>((resolve, reject) => {
      let scheme = 'ws:';
      if (window.location.protocol === 'https:') {
        scheme = 'wss:';
      }
      const socket = new WebSocket(`${scheme}//${window.location.host}/ws`);
      held.clusterModeSocket = socket;
      held.clusterModeSocketClosed = false;
      const timer = window.setTimeout(() => reject(new Error('the feed did not answer')), 60_000);
      socket.addEventListener('close', () => {
        held.clusterModeSocketClosed = true;
      });
      socket.addEventListener('error', () => {
        window.clearTimeout(timer);
        reject(new Error('the feed failed'));
      });
      socket.addEventListener('open', () => {
        socket.send(
          JSON.stringify({
            type: 'subscribe',
            subId: 'browser-probe',
            version: 'v1',
            resource: 'pods',
          }),
        );
      });
      socket.addEventListener('message', (event) => {
        const reply = JSON.parse(String(event.data)) as { subId?: string; type?: string };
        if (reply.subId !== 'browser-probe' || reply.type !== 'snapshot') {
          return;
        }
        window.clearTimeout(timer);
        resolve();
      });
    });
  });
}

export async function feedClosed(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const held = window as typeof window & { clusterModeSocketClosed?: boolean };
    return held.clusterModeSocketClosed === true;
  });
}

export function restartSpinoza(): void {
  kubectl(['-n', 'spinoza', 'rollout', 'restart', 'deployment/spinoza']);
  kubectl(['-n', 'spinoza', 'rollout', 'status', 'deployment/spinoza', '--timeout=8m']);
}

export async function providerSessionID(page: Page): Promise<string> {
  const cookies = await page.context().cookies(BASE_URL);
  const sessionCookie = cookies.find((cookie) => cookie.name === 'spinoza_session');
  if (sessionCookie === undefined) {
    throw new Error('the browser has no spinoza session cookie');
  }
  const payload = sessionCookie.value.split('.')[0];
  if (payload === undefined || payload === '') {
    throw new Error('the spinoza session cookie has no payload');
  }
  const decoded = JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')) as {
    s?: unknown;
  };
  if (typeof decoded.s !== 'string' || decoded.s === '') {
    throw new Error('the spinoza session cookie names no provider session');
  }
  return decoded.s;
}

export function endProviderSession(sessionID: string): void {
  const script = `
import json, os, urllib.parse, urllib.request
KC = "http://keycloak:8080"
def call(path, token=None, data=None, method=None, form=False):
    body = None
    headers = {}
    if data is not None:
        if form:
            body = urllib.parse.urlencode(data).encode()
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        else:
            body = json.dumps(data).encode()
            headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(KC + path, data=body, headers=headers, method=method)
    with urllib.request.urlopen(req) as resp:
        raw = resp.read()
        return resp.status, (json.loads(raw) if raw else None)
_, tok = call("/realms/master/protocol/openid-connect/token", data={
    "client_id": "admin-cli", "username": "admin", "password": "admin", "grant_type": "password"}, form=True)
token = tok["access_token"]
status, _ = call("/admin/realms/spinoza/sessions/" + os.environ["SESSION_ID"], token, method="DELETE")
print("logout", status)
`;
  const encoded = Buffer.from(script).toString('base64');
  const pod = `browser-kick-${String(Date.now())}`;
  kubectl([
    '-n',
    'keycloak',
    'run',
    pod,
    '--restart=Never',
    '--image=python:3.13-alpine',
    `--env=SESSION_ID=${sessionID}`,
    '--command',
    '--',
    'sh',
    '-c',
    `echo ${encoded} | base64 -d | python3 -`,
  ]);
  let output = '';
  try {
    kubectl([
      '-n',
      'keycloak',
      'wait',
      `pod/${pod}`,
      '--for=jsonpath={.status.phase}=Succeeded',
      '--timeout=2m',
    ]);
    output = kubectl(['-n', 'keycloak', 'logs', pod]);
  } finally {
    kubectl(['-n', 'keycloak', 'delete', 'pod', pod, '--ignore-not-found', '--wait=false']);
  }
  if (!output.includes('logout 204')) {
    throw new Error(`provider logout failed: ${output}`);
  }
}
