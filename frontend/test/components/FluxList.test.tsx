import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { FluxDashboard as FluxDashboardData } from '../../src/lib/types';
import FluxList from '../../src/components/FluxList';
import { makeFluxResource } from '../helpers';

function stubFlux(dashboard: FluxDashboardData): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(dashboard) }),
  );
}

function stubThenFail(dashboard: FluxDashboardData, message: string): void {
  let call = 0;
  vi.stubGlobal(
    'fetch',
    vi.fn(() => {
      call += 1;
      if (call === 1) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(dashboard) });
      }
      return Promise.reject(new Error(message));
    }),
  );
}

const dashboard: FluxDashboardData = {
  groups: [
    {
      name: 'Sources',
      ready: 1,
      reporting: 3,
      total: 4,
      resources: [
        makeFluxResource({
          kind: 'GitRepository',
          name: 'res-a',
          ready: 'True',
          suspended: false,
          message: '',
          createdAt: '2026-07-24T09:00:00Z',
        }),
        makeFluxResource({
          kind: 'HelmRelease',
          name: 'res-b',
          ready: 'False',
          suspended: false,
          message: 'install retries exhausted',
          createdAt: '2026-07-20T09:00:00Z',
        }),
        makeFluxResource({
          kind: 'Bucket',
          name: 'res-c',
          ready: '',
          suspended: false,
          message: '',
          createdAt: '',
        }),
        makeFluxResource({
          kind: 'OCIRepository',
          name: 'res-d',
          ready: 'False',
          suspended: true,
          message: 'suppressed while suspended',
          createdAt: '2026-07-19T09:00:00Z',
        }),
      ],
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('FluxList', () => {
  it('shows a loading state before the dashboard resolves', async () => {
    stubFlux({ groups: [] });
    render(<FluxList onSelect={vi.fn()} />);
    expect(screen.getByText('Loading Flux resources…')).toBeInTheDocument();
    expect(await screen.findByText('No Flux resources found.')).toBeInTheDocument();
  });

  it('renders each group with its resources and a combined status', async () => {
    stubFlux(dashboard);
    render(<FluxList onSelect={vi.fn()} />);
    expect(await screen.findByText('res-a')).toBeInTheDocument();
    expect(screen.getByText('res-b')).toBeInTheDocument();
    expect(screen.getByText('res-c')).toBeInTheDocument();
    expect(screen.getByText('res-d')).toBeInTheDocument();
    expect(screen.getByText('Sources')).toBeInTheDocument();
    expect(screen.getByText('1/3 ready · 1 no status')).toBeInTheDocument();
    expect(screen.getByText('Ready', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText('Not ready')).toBeInTheDocument();
    expect(screen.getByText('No status')).toBeInTheDocument();
    expect(screen.getByText('Suspended', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByTitle('install retries exhausted')).toBeInTheDocument();
    expect(screen.getByText('2026-07-24')).toBeInTheDocument();
    expect(screen.getByText('2026-07-20')).toBeInTheDocument();
  });

  it('gives each column a width and a drag-to-resize handle', async () => {
    stubFlux(dashboard);
    const { container } = render(<FluxList onSelect={vi.fn()} />);
    await screen.findByText('res-a');
    const headers = screen.getAllByRole('columnheader');
    expect(headers[0].getAttribute('style')).toContain('width');
    expect(container.querySelectorAll('.cursor-col-resize').length).toBeGreaterThan(0);
  });

  it('resizes a column from the keyboard', async () => {
    const user = userEvent.setup();
    stubFlux(dashboard);
    render(<FluxList onSelect={vi.fn()} />);
    await screen.findByText('res-a');
    const width = () => screen.getAllByRole('columnheader')[1].style.width;
    const before = width();

    screen.getByRole('button', { name: 'Resize the Name column' }).focus();
    await user.keyboard('{ArrowRight}');
    const widened = width();
    await user.keyboard('{Home}');

    expect(widened).not.toBe(before);
    expect(width()).toBe(before);
  });

  it('shows the error message when the fetch rejects with an error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('flux down')));
    render(<FluxList onSelect={vi.fn()} />);
    expect(await screen.findByText('flux down')).toBeInTheDocument();
  });

  it('shows a generic message when the fetch rejects with a non-error value', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));
    render(<FluxList onSelect={vi.fn()} />);
    expect(await screen.findByText('flux request failed')).toBeInTheDocument();
  });

  it('re-fetches on the poll interval', async () => {
    vi.useFakeTimers();
    const first: FluxDashboardData = {
      groups: [
        {
          name: 'Sources',
          ready: 1,
          reporting: 1,
          total: 1,
          resources: [makeFluxResource({ name: 'first' })],
        },
      ],
    };
    const second: FluxDashboardData = {
      groups: [
        {
          name: 'Sources',
          ready: 1,
          reporting: 1,
          total: 1,
          resources: [makeFluxResource({ name: 'second' })],
        },
      ],
    };
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(first) })
      .mockResolvedValue({ ok: true, json: () => Promise.resolve(second) });
    vi.stubGlobal('fetch', fetchMock);
    render(<FluxList onSelect={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('first')).toBeInTheDocument();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(screen.getByText('second')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
  it('reports the clicked resource to the caller', async () => {
    stubFlux(dashboard);
    const onSelect = vi.fn();
    render(<FluxList onSelect={onSelect} />);

    await userEvent.click(await screen.findByRole('button', { name: 'res-a' }));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ name: 'res-a' }));
  });
});

describe('FluxList partial failures', () => {
  it('says Flux could not be loaded rather than that there is none', async () => {
    stubFlux({
      groups: [],
      error: '4 of 9 resource types could not be listed; kustomizations: is forbidden',
    });

    render(<FluxList onSelect={vi.fn()} />);

    expect(await screen.findByText('Flux resources could not be loaded')).toBeInTheDocument();
    expect(screen.queryByText('No Flux resources found.')).not.toBeInTheDocument();
  });

  it('warns above the table when only some lists failed', async () => {
    stubFlux({ ...dashboard, error: '1 of 9 resource types could not be listed; buckets: nope' });

    render(<FluxList onSelect={vi.fn()} />);

    expect(await screen.findByRole('status')).toHaveTextContent('buckets: nope');
  });

  it('shows no warning when every list worked', async () => {
    stubFlux(dashboard);

    render(<FluxList onSelect={vi.fn()} />);

    await screen.findAllByRole('row');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('says the table stopped updating once a later poll fails', async () => {
    vi.useFakeTimers();
    stubThenFail(dashboard, 'flux endpoint is down');

    render(<FluxList onSelect={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('res-a')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('flux endpoint is down');
    expect(screen.getByText('res-a')).toBeInTheDocument();
  });

  it('keeps the staleness notice above an emptied list', async () => {
    vi.useFakeTimers();
    stubThenFail({ groups: [] }, 'flux endpoint is down');

    render(<FluxList onSelect={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('flux endpoint is down');
    expect(screen.getByText('No Flux resources found.')).toBeInTheDocument();
  });
});
