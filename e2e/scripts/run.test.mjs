import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mustRun, printable } from '../harness/run.ts';

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
