import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Checks from '../../src/components/Checks';
import { useSettingsStore } from '../../src/store/settings';
import { useClustersStore } from '../../src/store/clusters';
import { MK1, showing } from '../helpers-clusters';
import type { CheckFinding, CheckGroup, CheckObject, CheckReport } from '../../src/lib/types';

const OBJECTS: CheckObject[] = [
  {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    namespace: 'apps',
    name: 'api',
    kind: 'Deployment',
  },
];

function askedFor(target: RequestInfo | URL): string {
  if (typeof target === 'string') {
    return target;
  }
  return target instanceof URL ? target.href : target.url;
}

function makeFinding(extra: Partial<CheckFinding> = {}): CheckFinding {
  return {
    ref: 0,
    container: 'app',
    detail: 'securityContext.privileged is true',
    severity: 'high',
    ...extra,
  };
}

function makeGroup(id: string, extra: Partial<CheckGroup> = {}): CheckGroup {
  const findings = extra.findings ?? [];
  return {
    id,
    title: 'Privileged containers',
    category: 'security',
    severity: 'high',
    wrong: 'a privileged container holds every capability on the node',
    remedy: 'remove securityContext.privileged',
    total: findings.length,
    ...extra,
    findings,
  };
}

function stub(body: unknown, ok = true) {
  const fetcher = vi.fn((url: string) => {
    void url;
    return Promise.resolve({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

function asked(fetcher: ReturnType<typeof stub>): string[] {
  return fetcher.mock.calls.map((call) => call[0]);
}

function stubReportThenPages(report: unknown, pages: unknown[], pagesOk = true) {
  let at = -1;
  const fetchMock = vi.fn((url: string) => {
    if (!url.startsWith('/api/checks/findings')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(report) });
    }
    at += 1;
    return Promise.resolve({
      ok: pagesOk,
      status: pagesOk ? 200 : 500,
      json: () => Promise.resolve(pages[Math.min(at, pages.length - 1)]),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function cappedReport() {
  return {
    scanned: 7072,
    objects: [OBJECTS[0], { ...OBJECTS[0], name: 'web' }, { ...OBJECTS[0], name: 'db' }],
    groups: [
      makeGroup('limits-missing', {
        total: 3,
        truncated: true,
        next: 'Y3Vyc29yLTE',
        findings: [makeFinding()],
      }),
    ],
  };
}

function show(report: Partial<CheckReport>) {
  const onOpen = vi.fn();
  stub({ groups: [], objects: OBJECTS, scanned: 0, ...report });
  render(<Checks onOpen={onOpen} />);
  return { onOpen };
}

afterEach(() => {
  vi.unstubAllGlobals();
  useSettingsStore.setState({
    checksDisabled: [],
    checksSkipNamespaces: [],
    checksMinSeverity: '',
    checksWholeCluster: true,
    checksEveryKind: false,
    checkRules: '',
  });
});

describe('Checks', () => {
  it('says it is loading before the first answer', () => {
    stub({ groups: [], objects: OBJECTS, scanned: 0 });

    render(<Checks onOpen={vi.fn()} />);

    expect(screen.getByText(/Loading the cluster audit/)).toBeInTheDocument();
  });

  it('reports a request that never succeeded', async () => {
    stub({ message: 'no cluster' }, false);

    render(<Checks onOpen={vi.fn()} />);

    expect(await screen.findByText('no cluster')).toBeInTheDocument();
  });

  it('counts what it scanned and what it found', async () => {
    show({
      scanned: 12,
      groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })],
    });

    expect(await screen.findByText('1 findings across 12 workloads')).toBeInTheDocument();
  });

  it('lists a check under its category with its severity and count', async () => {
    show({
      scanned: 3,
      groups: [
        makeGroup('privileged-containers', { findings: [makeFinding()] }),
        makeGroup('image-latest', {
          title: 'Image tagged :latest',
          category: 'reliability',
          severity: 'medium',
        }),
      ],
    });

    expect(await screen.findByText('Security')).toBeInTheDocument();
    expect(screen.getByText('Reliability')).toBeInTheDocument();
    expect(screen.getByText('high')).toBeInTheDocument();
    expect(screen.getByText('clean')).toBeInTheDocument();
  });

  it('leaves out a category no check belongs to', async () => {
    show({ groups: [makeGroup('privileged-containers')] });

    await screen.findByText('Security');
    expect(screen.queryByText('Efficiency')).not.toBeInTheDocument();
  });

  it('opens a check to show what is wrong, what to do and where', async () => {
    show({
      scanned: 1,
      groups: [
        makeGroup('privileged-containers', {
          findings: [makeFinding({ patch: 'spec:\n  privileged: false\n' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText(/holds every capability/)).toBeInTheDocument();
    expect(screen.getByText(/remove securityContext.privileged/)).toBeInTheDocument();
    expect(screen.getByText('Deployment · apps/api · container app')).toBeInTheDocument();
    expect(screen.getByText(/privileged: false/)).toBeInTheDocument();
  });

  it('lists a finding that landed on the workload rather than a container', async () => {
    show({
      groups: [
        makeGroup('host-namespaces', {
          findings: [makeFinding({ container: undefined, detail: 'shares the host namespaces' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('Deployment · apps/api')).toBeInTheDocument();
  });

  it('marks a finding on something a package installed, and leaves your own bare', async () => {
    const onOpen = vi.fn();
    stub({
      scanned: 2,
      objects: [
        OBJECTS[0],
        { ...OBJECTS[0], name: 'kube-proxy', namespace: 'kube-system', origin: 'system' },
        { ...OBJECTS[0], name: 'nginx', origin: 'packaged', managedBy: 'Flux: ingress-nginx' },
      ],
      groups: [
        makeGroup('privileged-containers', {
          findings: [makeFinding(), makeFinding({ ref: 1 }), makeFinding({ ref: 2 })],
        }),
      ],
    });
    render(<Checks onOpen={onOpen} />);

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('Flux: ingress-nginx')).toBeInTheDocument();
    expect(screen.getByText('cluster')).toBeInTheDocument();
    expect(screen.getAllByText('Flux: ingress-nginx')).toHaveLength(1);
  });

  it('says how much of a capped group is on screen', async () => {
    show({
      scanned: 7072,
      groups: [
        makeGroup('limits-missing', { total: 7087, truncated: true, findings: [makeFinding()] }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('Showing 1 of 7087.')).toBeInTheDocument();
  });

  it('counts what the cluster has on the badge, not what fitted in the response', async () => {
    show({
      scanned: 7072,
      groups: [
        makeGroup('limits-missing', { total: 7087, truncated: true, findings: [makeFinding()] }),
      ],
    });

    expect(await screen.findByText('7087')).toBeInTheDocument();
    expect(screen.getByText('7087 findings across 7072 workloads')).toBeInTheDocument();
  });

  it('says nothing about truncation on a group that fits', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.queryByText(/Showing/)).not.toBeInTheDocument();
  });

  it('offers to load the findings a capped group left behind', async () => {
    stubReportThenPages(cappedReport(), []);
    render(<Checks onOpen={vi.fn()} />);

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('Showing 1 of 3.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show 2 more' })).toBeInTheDocument();
  });

  it('appends the next page and asks the backend for the cursor it was given', async () => {
    const fetchMock = stubReportThenPages(cappedReport(), [
      {
        findings: [{ ref: 0 }],
        objects: [{ name: 'web', kind: 'Deployment' }],
        next: 'Y3Vyc29yLTI',
      },
    ]);
    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));

    expect(screen.getByText('Showing 2 of 3.')).toBeInTheDocument();
    const asked = fetchMock.mock.calls.map((call) => call[0]);
    expect(asked).toContain('/api/checks/findings?check=limits-missing&after=Y3Vyc29yLTE');
  });

  it('follows the cursor the page returned, not the one the report started with', async () => {
    const fetchMock = stubReportThenPages(cappedReport(), [
      {
        findings: [{ ref: 0 }],
        objects: [{ name: 'web', kind: 'Deployment' }],
        next: 'Y3Vyc29yLTI',
      },
      { findings: [{ ref: 0 }], objects: [{ name: 'db', kind: 'Deployment' }], next: '' },
    ]);
    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));
    await userEvent.click(screen.getByRole('button', { name: 'Show 1 more' }));

    const asked = fetchMock.mock.calls.map((call) => call[0]);
    expect(asked).toContain('/api/checks/findings?check=limits-missing&after=Y3Vyc29yLTI');
  });

  it('stops offering more once every finding is on screen', async () => {
    stubReportThenPages(cappedReport(), [
      { findings: [{ ref: 0 }, { ref: 1 }], objects: [{ name: 'web' }, { name: 'db' }], next: '' },
    ]);
    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));

    expect(screen.queryByRole('button', { name: /Show \d+ more/ })).not.toBeInTheDocument();
    expect(screen.queryByText(/Showing/)).not.toBeInTheDocument();
  });

  it('says so when the failure was not an error object at all', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (!url.startsWith('/api/checks/findings')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(cappedReport()),
          });
        }
        // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors -- a socket failure need not reject with an Error
        return Promise.reject('the socket went away');
      }),
    );
    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('the findings request failed');
  });

  it('says so when a page could not be fetched and keeps what is already there', async () => {
    stubReportThenPages(cappedReport(), [{ message: 'the cluster went away' }], false);
    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('the cluster went away');
    expect(screen.getByText('Showing 1 of 3.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show 2 more' })).toBeInTheDocument();
  });

  it('forgets the pages it loaded when the check is closed again', async () => {
    stubReportThenPages(cappedReport(), [
      { findings: [{ ref: 0 }], objects: [{ name: 'web' }], next: '' },
    ]);
    render(<Checks onOpen={vi.fn()} />);
    const header = await screen.findByRole('button', { name: /Privileged containers/ });
    await userEvent.click(header);
    await userEvent.click(screen.getByRole('button', { name: 'Show 2 more' }));

    await userEvent.click(header);
    await userEvent.click(header);

    expect(screen.getByText('Showing 1 of 3.')).toBeInTheDocument();
  });

  it('closes a check that was open', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });

    const header = await screen.findByRole('button', { name: /Privileged containers/ });
    await userEvent.click(header);
    await userEvent.click(header);

    expect(screen.queryByText(/holds every capability/)).not.toBeInTheDocument();
  });

  it('shows no patch when the fix needs a value only the owner knows', async () => {
    show({ groups: [makeGroup('probes-missing', { findings: [makeFinding()] })] });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.queryByRole('figure')).not.toBeInTheDocument();
    expect(document.querySelector('pre')).toBeNull();
  });

  it('opens the object a finding landed on', async () => {
    const { onOpen } = show({
      groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Deployment · apps\/api/ }));

    expect(onOpen).toHaveBeenCalledWith(
      { group: 'apps', version: 'v1', resource: 'deployments', namespace: 'apps', name: 'api' },
      'Deployment',
    );
  });

  it('refuses to open a check that found nothing', async () => {
    show({ groups: [makeGroup('privileged-containers')] });

    const header = await screen.findByRole('button', { name: /Privileged containers/ });

    expect(header).toBeDisabled();
  });

  it('says why a check could not run', async () => {
    show({
      groups: [
        makeGroup('requests-far-above-usage', {
          title: 'Requests far above measured usage',
          category: 'efficiency',
          severity: 'low',
          skipped: 'metrics-server did not answer',
        }),
      ],
    });

    expect(await screen.findByText('metrics-server did not answer')).toBeInTheDocument();
    expect(screen.getByText('no data')).toBeInTheDocument();
  });

  it('shows the framework a check comes from', async () => {
    show({
      groups: [makeGroup('privileged-containers', { frameworks: ['PSS baseline', 'NSA/CISA'] })],
    });

    expect(await screen.findByText('PSS baseline')).toBeInTheDocument();
    expect(screen.getByText('NSA/CISA')).toBeInTheDocument();
  });

  it('passes on a partial read of the cluster', async () => {
    show({ groups: [], error: 'deployments.apps is forbidden' });

    expect(await screen.findByText('deployments.apps is forbidden')).toBeInTheDocument();
  });

  it('says when the audit stopped updating', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ groups: [], objects: [] }),
      })
      .mockResolvedValue({ ok: false, status: 500, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers({ shouldAdvanceTime: true });

    render(<Checks onOpen={vi.fn()} />);
    await screen.findByText('0 findings across 0 workloads');
    await vi.advanceTimersByTimeAsync(61000);

    await waitFor(() => {
      expect(screen.getByText(/stopped updating/)).toBeInTheDocument();
    });
    vi.useRealTimers();
  });
});

describe('audit controls', () => {
  it('remembers the severity floor and asks the backend for it', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });
    await screen.findByRole('button', { name: /Privileged containers/ });

    await userEvent.selectOptions(screen.getByLabelText('Lowest severity to show'), 'high');

    expect(useSettingsStore.getState().checksMinSeverity).toBe('high');
    await waitFor(() => {
      const urls = vi.mocked(fetch).mock.calls.map((call) => askedFor(call[0]));
      expect(urls.some((url) => url.includes('minSeverity=high'))).toBe(true);
    });
  });

  it('narrows to workloads when the whole-cluster box is cleared', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });
    await screen.findByRole('button', { name: /Privileged containers/ });

    await userEvent.click(screen.getByLabelText('Audit the whole cluster'));

    expect(useSettingsStore.getState().checksWholeCluster).toBe(false);
    await waitFor(() => {
      const urls = vi.mocked(fetch).mock.calls.map((call) => askedFor(call[0]));
      expect(urls.some((url) => url.includes('wholeCluster=0'))).toBe(true);
    });
  });

  it('asks for every kind when told to, and not before', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });
    await screen.findByRole('button', { name: /Privileged containers/ });

    await userEvent.click(screen.getByLabelText('Read every kind'));

    expect(useSettingsStore.getState().checksEveryKind).toBe(true);
    await waitFor(() => {
      const urls = vi.mocked(fetch).mock.calls.map((call) => askedFor(call[0]));
      expect(urls.some((url) => url.includes('everyKind=1'))).toBe(true);
    });
  });

  it('saves the rules you wrote and stops offering to', async () => {
    show({ groups: [] });
    await screen.findByLabelText('Your own rules');

    const editor = screen.getByLabelText('Your own rules');
    await userEvent.type(editor, 'rules');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(useSettingsStore.getState().checkRules).toBe('rules');
    });
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  });

  it('says so when the rules could not be saved', async () => {
    show({ groups: [] });
    await screen.findByLabelText('Your own rules');
    vi.spyOn(useSettingsStore.getState(), 'setCheckRules').mockRejectedValue(new Error('nope'));

    await userEvent.type(screen.getByLabelText('Your own rules'), 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText(/nope/)).toBeInTheDocument();
  });

  it('turns one check off and offers to put it back', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });

    await userEvent.click(
      await screen.findByRole('button', { name: 'Turn off privileged-containers' }),
    );

    expect(useSettingsStore.getState().checksDisabled).toEqual(['privileged-containers']);
    expect(await screen.findByRole('button', { name: /1 turned off/ })).toBeInTheDocument();
  });

  it('puts every turned-off check back at once', async () => {
    useSettingsStore.setState({ checksDisabled: ['a', 'b'] });
    show({ groups: [] });

    await userEvent.click(await screen.findByRole('button', { name: /2 turned off/ }));

    expect(useSettingsStore.getState().checksDisabled).toEqual([]);
  });

  it('skips the namespaces you name and drops the empty ones', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });
    await screen.findByRole('button', { name: /Privileged containers/ });

    await userEvent.type(screen.getByLabelText('Namespaces to skip'), 'kube-system, ,flux-system');

    expect(useSettingsStore.getState().checksSkipNamespaces).toEqual([
      'kube-system',
      'flux-system',
    ]);
  });

  it('does not open a check when its off button is pressed', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding()] })] });

    await userEvent.click(
      await screen.findByRole('button', { name: 'Turn off privileged-containers' }),
    );

    expect(screen.queryByText(/holds every capability/)).not.toBeInTheDocument();
  });
  it('widens to every open cluster when asked', async () => {
    const user = userEvent.setup();
    const calls = stub({ groups: [], objects: OBJECTS, scanned: 0 });
    act(() => {
      showing(MK1);
    });

    render(<Checks onOpen={vi.fn()} />);
    await screen.findByText('Severity');
    await user.click(screen.getByLabelText('Every open cluster'));

    await waitFor(() => {
      expect(asked(calls).some((url) => url.includes('/api/checks/fleet'))).toBe(true);
    });
    useClustersStore.getState().reset();
  });

  it('names the cluster a fleet finding is on', async () => {
    act(() => {
      showing(MK1);
    });
    stub({
      scanned: 1,
      objects: [{ ...OBJECTS[0], cluster: MK1 }],
      groups: [makeGroup('limits-missing', { total: 1, findings: [makeFinding()] })],
    });

    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(await screen.findByText('p-mk1')).toBeTruthy();
    useClustersStore.getState().reset();
  });

  it('says so when a fleet finding is on a cluster that is gone', async () => {
    act(() => {
      showing(MK1);
    });
    stub({
      scanned: 1,
      objects: [{ ...OBJECTS[0], cluster: 'https://gone:6443' }],
      groups: [makeGroup('limits-missing', { total: 1, findings: [makeFinding()] })],
    });

    render(<Checks onOpen={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(await screen.findByText('unknown')).toBeTruthy();
    useClustersStore.getState().reset();
  });

  it('offers nothing to widen when one cluster is open', async () => {
    stub({ groups: [], objects: OBJECTS, scanned: 0 });

    render(<Checks onOpen={vi.fn()} />);

    await screen.findByText('Severity');
    expect(screen.queryByLabelText('Every open cluster')).toBeNull();
  });
});

describe('a finding shows how much it matters here', () => {
  it("shows the severity derived for the object, not the check's", async () => {
    show({
      groups: [
        makeGroup('privileged-containers', {
          severity: 'high',
          findings: [makeFinding({ severity: 'low' })],
        }),
      ],
    });
    await screen.findByText('Privileged containers');

    await userEvent.click(screen.getByRole('button', { name: /Privileged containers/ }));

    expect(await screen.findByText('low')).toBeInTheDocument();
  });

  it('explains a level that was softened because the object is not yours', async () => {
    stub({
      groups: [
        makeGroup('privileged-containers', {
          severity: 'high',
          findings: [makeFinding({ severity: 'low' })],
        }),
      ],
      objects: [{ ...OBJECTS[0], namespace: 'kube-system', name: 'cilium', origin: 'system' }],
      scanned: 1,
    });
    render(<Checks onOpen={vi.fn()} />);
    await screen.findByText('Privileged containers');
    await userEvent.click(screen.getByRole('button', { name: /Privileged containers/ }));

    const badge = await screen.findByText('low');

    expect(badge.getAttribute('title')).toContain('do not own');
  });
});
