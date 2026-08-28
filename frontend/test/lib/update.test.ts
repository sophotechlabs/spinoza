import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  announceUpdate,
  fetchUpdateStatus,
  installUpdate,
  updateFailure,
  updateMessage,
  updateOutcome,
} from '../../src/lib/update';
import { useToastsStore } from '../../src/store/toasts';

function status(overrides: Record<string, unknown> = {}) {
  return {
    checked: true,
    current: 'v1.14.1',
    latest: 'v1.15.0',
    available: true,
    url: 'https://github.com/sophotechlabs/spinoza/releases/tag/v1.15.0',
    command: 'curl -fsSL https://spinoza.tech/install.sh | sh',
    ...overrides,
  };
}

function stub(body: unknown, ok = true) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok,
    status: ok ? 200 : 500,
    json: () => Promise.resolve(body),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchUpdateStatus', () => {
  it('asks the server what it found', async () => {
    const fetchMock = stub(status());

    await expect(fetchUpdateStatus()).resolves.toMatchObject({ latest: 'v1.15.0' });
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/update');
  });

  it('surfaces a failure rather than an answer', async () => {
    stub({ message: 'no' }, false);

    await expect(fetchUpdateStatus()).rejects.toThrow('no');
  });
});

describe('announceUpdate', () => {
  it('names both versions', () => {
    expect(updateMessage(status())).toBe('Spinoza v1.15.0 is out. You are running v1.14.1.');
  });

  it('offers the command that installs it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    stub(status());

    await announceUpdate();

    const [toast] = useToastsStore.getState().toasts;
    expect(toast.message).toContain('v1.15.0');
    expect(toast.action?.label).toBe('Copy install command');
    toast.action?.run();
    await vi.waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('curl -fsSL https://spinoza.tech/install.sh | sh');
    });
  });

  it('says nothing when there is nothing newer', async () => {
    stub(status({ available: false, latest: 'v1.14.1' }));

    await announceUpdate();

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('says nothing when the check never happened', async () => {
    stub({ checked: false, current: 'dev', available: false, reason: 'not a release build' });

    await announceUpdate();

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('says nothing when the server offered no command to run', async () => {
    stub(status({ command: undefined }));

    await announceUpdate();

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('stays quiet when the check itself fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    await expect(announceUpdate()).resolves.toBeUndefined();
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });
});

function result(overrides: Record<string, unknown> = {}) {
  return { updated: false, current: 'v1.14.1', ...overrides };
}

describe('installUpdate', () => {
  it('asks the server to install', async () => {
    const fetchMock = stub(result({ updated: true, latest: 'v1.15.0' }));

    await expect(installUpdate()).resolves.toMatchObject({ updated: true });

    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/update');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
  });

  it('surfaces a failure rather than an outcome', async () => {
    stub({ message: 'the settings file is read-only' }, false);

    await expect(installUpdate()).rejects.toThrow('the settings file is read-only');
  });
});

describe('updateOutcome', () => {
  it('says what to do once it has replaced the binary', () => {
    expect(updateOutcome(result({ updated: true, latest: 'v1.15.0' }))).toBe(
      'Updated to v1.15.0. Restart spinoza to finish.',
    );
  });

  it('hands over the command when this build cannot replace itself', () => {
    const said = updateOutcome(
      result({ command: 'curl -fsSL https://spinoza.tech/install.sh | sh' }),
    );

    expect(said).toContain('cannot replace itself');
    expect(said).toContain('install.sh');
  });

  it('passes on what went wrong', () => {
    expect(updateOutcome(result({ reason: 'checksum did not match' }))).toBe(
      'checksum did not match',
    );
  });

  it('says there is nothing newer', () => {
    expect(updateOutcome(result())).toBe('v1.14.1 is the newest release.');
  });
});

describe('updateFailure', () => {
  it('passes on the message', () => {
    expect(updateFailure(new Error('offline'))).toBe('offline');
  });

  it('falls back for a rejection that is not an error', () => {
    expect(updateFailure('nope')).toBe('the update failed');
  });
});
