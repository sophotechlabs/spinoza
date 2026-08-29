import { createHash } from 'node:crypto';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

export const E2E_DIR = resolve(here, '..');
export const REPO_DIR = resolve(E2E_DIR, '..');
export const TMP_DIR = join(E2E_DIR, '.tmp');

export const KUBECONFIG = join(TMP_DIR, 'kubeconfig');
export const TOKEN_FILE = join(TMP_DIR, 'token');
export const STATE_FILE = join(TMP_DIR, 'state.json');
export const STORAGE_STATE = join(TMP_DIR, 'storage.json');

const DEFAULT_CLUSTER = 'spinoza';
const DEFAULT_PORT = 34215;
const SESSION_PORT_BASE = 24000;
const SESSION_PORT_SPAN = 1000;

let cluster = DEFAULT_CLUSTER;
if (process.env.SPINOZA_KIND_CLUSTER) {
  cluster = process.env.SPINOZA_KIND_CLUSTER;
}

function portFor(session: string): number {
  if (session === DEFAULT_CLUSTER) {
    return DEFAULT_PORT;
  }
  const digest = createHash('sha256').update(session).digest();
  return SESSION_PORT_BASE + (digest.readUInt16BE(0) % SESSION_PORT_SPAN);
}

let addr = `127.0.0.1:${portFor(cluster)}`;
if (process.env.SPINOZA_E2E_ADDR) {
  addr = process.env.SPINOZA_E2E_ADDR;
}

export const CLUSTER = cluster;
export const CONTEXT = `kind-${CLUSTER}`;
export const NAMESPACE = 'e2e';
export const ADDR = addr;
export const BASE_URL = `http://${ADDR}`;

export const BINARY = join(REPO_DIR, 'spinoza');
