import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

const crashes = vi.hoisted(() => ({ topBar: false, sidebar: false, palette: false }));

vi.mock('../src/lib/feed', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../src/lib/feed')>()),
  useResourceFeed: () => ({
    status: 'connected',
    attempt: 0,
    subscribe: vi.fn(),
    unsubscribe: vi.fn(),
    subscribeLogs: vi.fn(),
    unsubscribeLogs: vi.fn(),
    reconnect: vi.fn(),
  }),
}));

vi.mock('../src/components/TopBar', () => ({
  default: () => {
    if (crashes.topBar) {
      throw new Error('the top bar exploded');
    }
    return <div>top bar</div>;
  },
}));

vi.mock('../src/components/Sidebar', () => ({
  default: () => {
    if (crashes.sidebar) {
      throw new Error('the sidebar exploded');
    }
    return <div>sidebar</div>;
  },
}));

vi.mock('../src/components/CommandPalette', () => ({
  default: () => {
    if (crashes.palette) {
      throw new Error('the palette exploded');
    }
    return <div>palette</div>;
  },
}));

import App from '../src/App';

beforeEach(() => {
  crashes.topBar = false;
  crashes.sidebar = false;
  crashes.palette = false;
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ categories: [] }) }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('a component that stops rendering', () => {
  it('takes down the top bar without taking the rest of the app', () => {
    crashes.topBar = true;

    render(<App />);

    expect(screen.getByRole('alert')).toHaveTextContent('The top bar stopped rendering');
    expect(screen.getByRole('alert')).toHaveTextContent('the top bar exploded');
    expect(screen.getByText('sidebar')).toBeInTheDocument();
    expect(screen.getByRole('main')).toBeInTheDocument();
  });

  it('takes down the sidebar without taking the rest of the app', () => {
    crashes.sidebar = true;

    render(<App />);

    expect(screen.getByRole('alert')).toHaveTextContent('The sidebar stopped rendering');
    expect(screen.getByText('top bar')).toBeInTheDocument();
    expect(screen.getByRole('main')).toBeInTheDocument();
  });

  it('takes down the command palette without taking the rest of the app', () => {
    crashes.palette = true;

    render(<App />);

    expect(screen.getByRole('alert')).toHaveTextContent('The command palette stopped rendering');
    expect(screen.getByText('top bar')).toBeInTheDocument();
    expect(screen.getByText('sidebar')).toBeInTheDocument();
  });

  it('leaves no alert at all when nothing crashes', () => {
    render(<App />);

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
