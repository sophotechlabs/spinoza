import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { background, mustRun, printable, waitFor } from '../harness/run.ts';

function scratch(t) {
  const dir = mkdtempSync(join(tmpdir(), 'spinoza-e2e-run-'));
  t.after(() => {
    rmSync(dir, { recursive: true, force: true });
  });
  return dir;
}

async function logOnceItHolds(log, last) {
  await waitFor(`${log} to hold ${last}`, 250, 20, () => readFileSync(log, 'utf8').includes(last));
  return readFileSync(log, 'utf8');
}

test('a background process prints into its own log, in a directory made for it', async (t) => {
  const log = join(scratch(t), 'logs', 'guarded.log');

  background('sh', ['-c', 'echo listening on 127.0.0.1:26001; echo connection refused >&2'], {
    log,
  });

  const got = await logOnceItHolds(log, 'connection refused');
  assert.equal(got, 'listening on 127.0.0.1:26001\nconnection refused\n');
});

test('a restarted process adds to its log rather than replacing the first run', async (t) => {
  const log = join(scratch(t), 'main.log');
  writeFileSync(log, 'first run\n');

  background('sh', ['-c', 'echo second run'], { log });

  const got = await logOnceItHolds(log, 'second run');
  assert.equal(got, 'first run\nsecond run\n');
});

test('drops the emoji kind prints between its progress words', () => {
  const got = printable(' ✓ Ensuring node image (kindest/node:v1.36.1) 🖼\n • Starting control-plane 🕹️  ...');

  assert.equal(got, ' ✓ Ensuring node image (kindest/node:v1.36.1) \n • Starting control-plane   ...');
});

test('leaves no half of a character a log line could be cut through', () => {
  const got = printable('📦 📦 📦'.repeat(100));

  assert.equal(/[\uD800-\uDFFF]/.test(got), false);
});

test('drops a stray half of a character too', () => {
  assert.equal(printable('cut here \uD83D'), 'cut here ');
});

test('keeps ordinary text, accents and box-drawing as they are', () => {
  const text = 'deployment/argocd-redis not ready — Available: 0/1 · café ─ ✓ •';

  assert.equal(printable(text), text);
});

test('a failed command reports its output without the emoji', () => {
  assert.throws(
    () => mustRun('sh', ['-c', 'printf "Preparing nodes 📦 📦\\n" >&2; exit 3']),
    (err) => {
      assert.match(err.message, /^sh -c .* exited 3\n/);
      assert.equal(err.message.includes('📦'), false);
      assert.match(err.message, /Preparing nodes/);
      return true;
    },
  );
});
