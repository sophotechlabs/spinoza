import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ActionGroup, { Action, ActionNote } from '../../src/components/ActionGroup';
import { actionClass } from '../../src/lib/actions';

describe('a group of actions', () => {
  it('names what the actions act on', () => {
    render(
      <ActionGroup label="Object actions">
        <Action label="Restart" onClick={vi.fn()} />
      </ActionGroup>,
    );

    expect(screen.getByRole('group', { name: 'Object actions' })).toBeInTheDocument();
  });

  it('wraps rather than pushing actions off a narrow panel', () => {
    render(
      <ActionGroup>
        <Action label="Restart" onClick={vi.fn()} />
      </ActionGroup>,
    );

    expect(screen.getByRole('group').getAttribute('class')).toContain('flex-wrap');
  });

  it('says what is happening beside the actions', () => {
    render(
      <ActionGroup>
        <ActionNote>working</ActionNote>
      </ActionGroup>,
    );

    expect(screen.getByText('working')).toBeInTheDocument();
  });
});

describe('one action', () => {
  it('runs what it is for', async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<Action label="Reconcile" onClick={onClick} />);

    await user.click(screen.getByRole('button', { name: 'Reconcile' }));

    expect(onClick).toHaveBeenCalledOnce();
  });

  it('refuses the click when it is disabled, and says why', async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <>
        <Action label="Drain" onClick={onClick} disabled describedBy="why" title="read-only" />
        <span id="why">this cluster is read-only</span>
      </>,
    );
    const button = screen.getByRole('button', { name: 'Drain' });

    await user.click(button);

    expect(onClick).not.toHaveBeenCalled();
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-describedby', 'why');
    expect(button).toHaveAttribute('title', 'read-only');
  });

  it('carries the colour of what it does', () => {
    render(
      <>
        <Action label="Reconcile" onClick={vi.fn()} />
        <Action label="Resume" tone="good" onClick={vi.fn()} />
        <Action label="Suspend" tone="caution" onClick={vi.fn()} />
        <Action label="Delete" tone="danger" onClick={vi.fn()} />
      </>,
    );

    expect(screen.getByRole('button', { name: 'Reconcile' }).className).toContain('text-fg');
    expect(screen.getByRole('button', { name: 'Resume' }).className).toContain('text-ok');
    expect(screen.getByRole('button', { name: 'Suspend' }).className).toContain('text-warn');
    expect(screen.getByRole('button', { name: 'Delete' }).className).toContain('text-error');
  });

  it('comes in one height for panels and a shorter one for dense bars', () => {
    render(
      <>
        <Action label="Restart" onClick={vi.fn()} />
        <Action label="Delete" size="dense" onClick={vi.fn()} />
      </>,
    );

    expect(screen.getByRole('button', { name: 'Restart' }).className).toContain('py-1');
    expect(screen.getByRole('button', { name: 'Delete' }).className).toContain('py-0.5');
  });

  it('offers the same shape to a button that cannot be one', () => {
    expect(actionClass()).toBe(actionClass('plain', 'normal'));
    expect(actionClass('danger', 'dense')).toContain('py-0.5');
    expect(actionClass('danger', 'dense')).toContain('text-error');
  });
});
