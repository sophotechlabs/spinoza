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

export const CLUSTER = 'spinoza-e2e';
export const CONTEXT = `kind-${CLUSTER}`;
export const NAMESPACE = 'e2e';
export const ADDR = '127.0.0.1:34215';
export const BASE_URL = `http://${ADDR}`;

export const BINARY = join(REPO_DIR, 'spinoza');
