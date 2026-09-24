import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import RecordedSessions from '../../src/components/RecordedSessions';

function delayedRead() {
  let resolve!: (response: Response) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Response>((done, failed) => {
    resolve = done;
    reject = failed;
  });
  return { promise, resolve, reject };
}

function recordings() {
  return {
    recording: true,
    sessions: [
      {
        id: 'first',
        at: '2026-09-23T12:00:00Z',
        actor: 'reader@example.com',
        target: 'example/first',
        kind: 'exec',
        bytes: 20,
      },
      {
        id: 'second',
        at: '2026-09-23T12:01:00Z',
        actor: 'reader@example.com',
        target: 'example/second',
        kind: 'exec',
        bytes: 30,
      },
    ],
  };
}

function serveDelayedFirst() {
  const first = delayedRead();
  const fetcher = vi.fn((url: string) => {
    if (url === '/api/transcripts/text?id=first') {
      return first.promise;
    }
    if (url === '/api/transcripts/text?id=second') {
      return Promise.resolve(new Response('output from the second session'));
    }
    return Promise.resolve(Response.json(recordings()));
  });
  vi.stubGlobal('fetch', fetcher);
  return { first, fetcher };
}

async function openFirst() {
  const target = await screen.findByText('example/first');
  const row = target.closest('li');
  expect(row).not.toBeNull();
  if (row === null) {
    throw new Error('the first recording has no row');
  }
  await userEvent.click(within(row).getByRole('button', { name: 'Read' }));
  expect(within(row).getByText('reading it…')).toBeInTheDocument();
  return row;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('recorded session response ordering', () => {
  for (const outcome of ['success', 'failure'] as const) {
    it(`keeps the second session when the first read finishes with ${outcome}`, async () => {
      const { first, fetcher } = serveDelayedFirst();
      render(<RecordedSessions />);
      await openFirst();
      await userEvent.click(screen.getByRole('button', { name: 'Read' }));
      expect(await screen.findByText('output from the second session')).toBeInTheDocument();

      await act(async () => {
        if (outcome === 'success') {
          first.resolve(new Response('obsolete first output'));
        } else {
          first.reject(new Error('obsolete first failure'));
        }
        await first.promise.catch(() => undefined);
      });

      expect(screen.getByText('output from the second session')).toBeInTheDocument();
      expect(screen.queryByText(/obsolete first/)).not.toBeInTheDocument();
      expect(fetcher.mock.calls.map(([url]) => url)).toEqual([
        '/api/transcripts',
        '/api/transcripts/text?id=first',
        '/api/transcripts/text?id=second',
      ]);
    });

    it(`keeps a hidden session closed when its read finishes with ${outcome}`, async () => {
      const { first } = serveDelayedFirst();
      render(<RecordedSessions />);
      const row = await openFirst();
      await userEvent.click(within(row).getByRole('button', { name: 'Hide' }));

      await act(async () => {
        if (outcome === 'success') {
          first.resolve(new Response('obsolete hidden output'));
        } else {
          first.reject(new Error('obsolete hidden failure'));
        }
        await first.promise.catch(() => undefined);
      });

      expect(within(row).getByRole('button', { name: 'Read' })).toBeInTheDocument();
      expect(screen.queryByText(/obsolete hidden/)).not.toBeInTheDocument();
      expect(screen.queryByText('reading it…')).not.toBeInTheDocument();
    });
  }

  it('a reopened recording receives a new read instead of the hidden response', async () => {
    const { first, fetcher } = serveDelayedFirst();
    render(<RecordedSessions />);
    const row = await openFirst();
    await userEvent.click(within(row).getByRole('button', { name: 'Hide' }));
    fetcher.mockImplementationOnce(() => Promise.resolve(new Response('fresh reopened output')));
    await userEvent.click(within(row).getByRole('button', { name: 'Read' }));
    expect(await screen.findByText('fresh reopened output')).toBeInTheDocument();

    await act(async () => {
      first.resolve(new Response('old hidden output'));
      await first.promise;
    });

    expect(screen.getByText('fresh reopened output')).toBeInTheDocument();
    expect(screen.queryByText('old hidden output')).not.toBeInTheDocument();
    expect(fetcher.mock.calls.filter(([url]) => url.endsWith('id=first'))).toHaveLength(2);
  });

  it('a failed recording can be reopened and read successfully', async () => {
    const { first, fetcher } = serveDelayedFirst();
    render(<RecordedSessions />);
    const row = await openFirst();
    await act(async () => {
      first.reject(new Error('temporary recording read failure'));
      await first.promise.catch(() => undefined);
    });
    expect(await screen.findByText('temporary recording read failure')).toBeInTheDocument();
    await userEvent.click(within(row).getByRole('button', { name: 'Hide' }));
    fetcher.mockImplementationOnce(() => Promise.resolve(new Response('recovered recording')));
    await userEvent.click(within(row).getByRole('button', { name: 'Read' }));

    expect(await screen.findByText('recovered recording')).toBeInTheDocument();
    expect(screen.queryByText('temporary recording read failure')).not.toBeInTheDocument();
  });
});
