import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RecordedSessions from '../../src/components/RecordedSessions';

function stub(page: unknown, text = 'uid=0(root)\n', textOk = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.startsWith('/api/transcripts/text')) {
        return Promise.resolve({
          ok: textOk,
          status: textOk ? 200 : 404,
          text: () => Promise.resolve(text),
          json: () => Promise.resolve({ message: 'no session by that name was recorded' }),
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(page),
      });
    }),
  );
}

const recorded = {
  recording: true,
  sessions: [
    {
      id: 'abc',
      at: '2026-09-09T12:00:00Z',
      actor: 'alice@example.com',
      target: 'prod/web',
      kind: 'exec',
      bytes: 2048,
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('RecordedSessions', () => {
  it('says so when this deployment records nothing', async () => {
    stub({
      recording: false,
      sessions: [],
      reason: 'this deployment does not record terminal sessions',
    });

    render(<RecordedSessions />);

    expect(
      await screen.findByText(/this deployment does not record terminal sessions/),
    ).toBeInTheDocument();
  });

  it('lists who opened what, and how big it is', async () => {
    stub(recorded);

    render(<RecordedSessions />);

    expect(await screen.findByText('alice@example.com')).toBeInTheDocument();
    expect(screen.getByText('prod/web')).toBeInTheDocument();
    expect(screen.getByText('2 kB')).toBeInTheDocument();
  });

  it('reads a session on request and hides it again', async () => {
    stub(recorded);

    render(<RecordedSessions />);
    await userEvent.click(await screen.findByRole('button', { name: 'Read' }));

    expect(await screen.findByText(/uid=0\(root\)/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Hide' }));
    expect(screen.queryByText(/uid=0\(root\)/)).not.toBeInTheDocument();
  });

  it('says what went wrong rather than an empty transcript', async () => {
    stub(recorded, '', false);

    render(<RecordedSessions />);
    await userEvent.click(await screen.findByRole('button', { name: 'Read' }));

    expect(await screen.findByText(/no session by that name was recorded/)).toBeInTheDocument();
  });

  it('says nothing has been opened yet rather than showing an empty list', async () => {
    stub({ recording: true, sessions: [] });

    render(<RecordedSessions />);

    expect(
      await screen.findByText('No terminal has been opened since recording was turned on.'),
    ).toBeInTheDocument();
  });
});
