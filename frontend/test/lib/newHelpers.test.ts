import { afterEach, describe, expect, it, vi } from 'vitest';
import { cpuText, memText, rowKey, shareOf } from '../../src/lib/waste';
import { sizeText, fetchTranscript, fetchTranscripts } from '../../src/lib/transcripts';
import { describeView, forgetView, saveView, fetchSavedViews } from '../../src/lib/savedViews';
import { fetchRevisionDiff, fetchRevisions } from '../../src/lib/rollout';
import { chipsFromText, chipsText } from '../../src/lib/filterChips';
import { exportChecks, fetchPosture } from '../../src/lib/checks';
import { paletteItems } from '../../src/lib/palette';
import type { ObjectRef, SavedView } from '../../src/lib/types';
import type { ChecksFilter } from '../../src/store/settings';

const ref: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'prod',
  name: 'web',
};

function stub(ok: boolean, body: unknown, text = '') {
  const asked: { url: string; method?: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      asked.push({ url, method: init?.method });
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve(text),
        blob: () => Promise.resolve(new Blob([text])),
      });
    }),
  );
  return asked;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('waste figures', () => {
  it('reads cpu in cores once there is more than one', () => {
    expect(cpuText(250)).toBe('250m');
    expect(cpuText(1000)).toBe('1 cores');
    expect(cpuText(1500)).toBe('1.5 cores');
  });

  it('reads memory in gibibytes once there is more than one', () => {
    expect(memText(512)).toBe('512 Mi');
    expect(memText(2048)).toBe('2.0 Gi');
  });

  it('never reports a share of nothing', () => {
    expect(shareOf(10, 0)).toBe(0);
    expect(shareOf(5, 10)).toBe(0.5);
    expect(shareOf(20, 10)).toBe(1);
  });

  it('keys a row by everything that identifies it', () => {
    expect(
      rowKey({
        namespace: 'prod',
        cpuRequested: 0,
        cpuUsed: 0,
        cpuReclaimable: 0,
        memRequested: 0,
        memUsed: 0,
        memReclaimable: 0,
      }),
    ).toBe('prod//');
    expect(
      rowKey({
        namespace: 'prod',
        kind: 'Deployment',
        name: 'web',
        cpuRequested: 0,
        cpuUsed: 0,
        cpuReclaimable: 0,
        memRequested: 0,
        memUsed: 0,
        memReclaimable: 0,
      }),
    ).toBe('prod/Deployment/web');
  });
});

describe('recorded session size', () => {
  it('grows its unit with the transcript', () => {
    expect(sizeText(12)).toBe('12 bytes');
    expect(sizeText(2048)).toBe('2 kB');
    expect(sizeText(3 * 1024 * 1024)).toBe('3.0 MB');
  });

  it('names what went wrong rather than returning nothing', async () => {
    stub(false, { message: 'your role here is viewer' });
    await expect(fetchTranscripts()).rejects.toThrow('your role here is viewer');
    await expect(fetchTranscript('abc')).rejects.toThrow('your role here is viewer');
  });

  it('asks for the session by name', async () => {
    const asked = stub(true, {}, 'uid=0(root)');
    await expect(fetchTranscript('a b')).resolves.toBe('uid=0(root)');
    expect(asked[0].url).toContain('id=a%20b');
  });
});

describe('saved views', () => {
  const view: SavedView = { id: 'a', name: 'one', view: 'resources' };

  it('describes what a view narrows to', () => {
    expect(describeView(view)).toBe('resources');
    expect(describeView({ ...view, resource: 'pods' })).toBe('pods');
    expect(describeView({ ...view, resource: 'pods', namespace: 'prod' })).toBe('pods in prod');
    expect(describeView({ ...view, resource: 'pods', filter: 'name:web' })).toBe(
      'pods matching name:web',
    );
  });

  it('says which view it is forgetting, and whether it was published', async () => {
    const asked = stub(true, {});
    await forgetView('a', false);
    expect(asked[0].url).toBe('/api/views?id=a');
    expect(asked[0].method).toBe('DELETE');
    await forgetView('b', true);
    expect(asked[1].url).toContain('shared=true');
  });

  it('names what went wrong on every call', async () => {
    stub(false, { message: 'that did not work' });
    await expect(fetchSavedViews()).rejects.toThrow('that did not work');
    await expect(saveView(view)).rejects.toThrow('that did not work');
    await expect(forgetView('a', false)).rejects.toThrow('that did not work');
  });

  it('reads an answer that carries no views as none', async () => {
    stub(true, {});
    await expect(fetchSavedViews()).resolves.toEqual({ views: [], mayShare: false });
  });
});

describe('rollout requests', () => {
  it('names what went wrong rather than an empty list', async () => {
    stub(false, { message: 'the cluster said no' });
    await expect(fetchRevisions(ref)).rejects.toThrow('the cluster said no');
    await expect(fetchRevisionDiff(ref, 1, 2)).rejects.toThrow('the cluster said no');
  });

  it('asks for the two revisions it is diffing', async () => {
    const asked = stub(true, { from: 1, to: 2, lines: 0, left: '', right: '' });
    await fetchRevisionDiff(ref, 1, 2);
    expect(asked[0].url).toContain('from=1');
    expect(asked[0].url).toContain('to=2');
  });
});

describe('filter chips as text', () => {
  it('round-trips a chip list', () => {
    const chips = [
      { field: 'name', value: 'web' },
      { field: 'status', value: 'CrashLoopBackOff' },
    ];
    expect(chipsFromText(chipsText(chips))).toEqual(chips);
  });

  it('reads a bare word as a name', () => {
    expect(chipsFromText('web')).toEqual([{ field: 'name', value: 'web' }]);
  });

  it('leaves out what says nothing', () => {
    expect(chipsFromText('')).toEqual([]);
    expect(chipsFromText('   ')).toEqual([]);
    expect(chipsFromText('status:')).toEqual([]);
  });
});

describe('framework posture', () => {
  it('asks for one framework when told to', async () => {
    const asked = stub(true, { frameworks: [], controls: [] });
    await fetchPosture('CIS Kubernetes Benchmark');
    expect(asked[0].url).toContain('framework=CIS+Kubernetes+Benchmark');
    await fetchPosture();
    expect(asked[1].url).toBe('/api/checks/frameworks');
  });

  it('names what went wrong', async () => {
    stub(false, { message: 'this view reads the whole cluster' });
    await expect(fetchPosture()).rejects.toThrow('this view reads the whole cluster');
  });
});

const everything: ChecksFilter = {
  disabled: [],
  skipNamespaces: [],
  namespace: '',
  minSeverity: '',
  wholeCluster: true,
  everyKind: false,
  onlyNew: false,
  showMuted: false,
};

describe('the checks export', () => {
  it('names the format when it is not the default', async () => {
    const asked = stub(true, {});
    await exportChecks(everything, 'sarif');
    expect(asked[0].url).toContain('format=sarif');
    await exportChecks(everything);
    expect(asked[1].url).not.toContain('format=');
  });

  it('names what went wrong', async () => {
    stub(false, { message: 'this view reads the whole cluster' });
    await expect(exportChecks(everything)).rejects.toThrow('this view reads the whole cluster');
  });
});

describe('the palette', () => {
  it('says a saved view is published when it is', () => {
    const items = paletteItems([], [], false, [
      { id: 'a', name: 'mine', view: 'checks' },
      { id: 'b', name: 'ours', view: 'checks', shared: true },
    ]);
    const mine = items.find((one) => one.label === 'mine');
    const ours = items.find((one) => one.label === 'ours');
    expect(mine?.hint).toBe('checks');
    expect(ours?.hint).toBe('checks · everybody');
  });
});
