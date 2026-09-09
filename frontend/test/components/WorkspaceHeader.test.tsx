import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import WorkspaceHeader from '../../src/components/WorkspaceHeader';

describe('the header every view carries', () => {
  it('names the view on its own when there is nothing else to say', () => {
    render(<WorkspaceHeader title="Issues" />);

    expect(screen.getByRole('heading', { name: 'Issues' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Issues' }).parentElement?.textContent).toBe(
      'Issues',
    );
  });

  it('carries the scope, the scale it counts in, and how old the data is', () => {
    render(
      <WorkspaceHeader
        title="Cluster checks"
        scope="p-mk2, every namespace"
        scale="high, medium and low by rule"
        stale="09:31:04"
      />,
    );

    const heading = screen.getByRole('heading', { name: 'Cluster checks' });
    const row = heading.parentElement;
    expect(row).toHaveTextContent('p-mk2, every namespace');
    expect(row).toHaveTextContent('high, medium and low by rule');
    expect(row).toHaveTextContent('as of 09:31:04');
  });

  it('leaves out an empty scope rather than drawing a bare separator', () => {
    render(<WorkspaceHeader title="History" scope="" scale="" stale="" />);

    expect(screen.getByRole('heading', { name: 'History' }).parentElement?.textContent).toBe(
      'History',
    );
  });

  it('makes room for the controls a view puts beside its title', () => {
    render(
      <WorkspaceHeader title="Helm releases" scope="p-mk2">
        <button type="button">Install chart</button>
      </WorkspaceHeader>,
    );

    expect(screen.getByRole('button', { name: 'Install chart' })).toBeVisible();
  });
});
