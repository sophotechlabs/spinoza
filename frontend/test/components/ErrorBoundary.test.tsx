import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ErrorBoundary from '../../src/components/ErrorBoundary';

function Boom({ thrown }: { thrown: unknown }): never {
  throw thrown;
}

function Recoverable() {
  const [broken, setBroken] = useState(true);
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          setBroken(false);
        }}
      >
        fix it
      </button>
      <ErrorBoundary label="Overview">
        {broken && <Boom thrown={new Error('render blew up')} />}
        <span>the panel is back</span>
      </ErrorBoundary>
    </div>
  );
}

describe('ErrorBoundary', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders its children while nothing throws', () => {
    render(
      <ErrorBoundary label="Overview">
        <span>the panel</span>
      </ErrorBoundary>,
    );

    expect(screen.getByText('the panel')).toBeInTheDocument();
  });

  it('names what broke instead of leaving a blank window', () => {
    render(
      <ErrorBoundary label="Spinoza">
        <Boom thrown={new Error('cannot read properties of undefined')} />
      </ErrorBoundary>,
    );

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText('Spinoza stopped rendering')).toBeInTheDocument();
    expect(screen.getByText('cannot read properties of undefined')).toBeInTheDocument();
  });

  it('survives a throw that was not an Error', () => {
    render(
      <ErrorBoundary label="Logs">
        <Boom thrown="nope" />
      </ErrorBoundary>,
    );

    expect(screen.getByText('the cause was not an Error')).toBeInTheDocument();
  });

  it('renders again from the retry button', async () => {
    const user = userEvent.setup();
    render(<Recoverable />);
    expect(screen.getByText('Overview stopped rendering')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'fix it' }));
    expect(screen.getByText('Overview stopped rendering')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Try again' }));

    expect(screen.queryByText('Overview stopped rendering')).not.toBeInTheDocument();
    expect(screen.getByText('the panel is back')).toBeInTheDocument();
  });
});
