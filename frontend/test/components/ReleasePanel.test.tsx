import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import ReleasePanel from '../../src/components/ReleasePanel';

let mounts = 0;

vi.mock('../../src/components/HelmReleaseDetail', () => ({
  default: ({ namespace, name }: { namespace: string; name: string }) => {
    mounts += 1;
    return (
      <div data-testid="release-detail" data-mounts={mounts}>
        {namespace}/{name}
      </div>
    );
  },
}));

describe('ReleasePanel', () => {
  it('asks for a release when none is selected', () => {
    render(
      <ReleasePanel
        target={null}
        onSelectResource={vi.fn()}
        onOpenResource={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByText('Select a Helm release to inspect it.')).toBeInTheDocument();
  });

  it('shows the detail of the selected release', () => {
    render(
      <ReleasePanel
        target={{ namespace: 'demo', name: 'podinfo' }}
        onSelectResource={vi.fn()}
        onOpenResource={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId('release-detail')).toHaveTextContent('demo/podinfo');
  });

  it('starts the detail over when another release is picked', () => {
    const { rerender } = render(
      <ReleasePanel
        target={{ namespace: 'demo', name: 'podinfo' }}
        onSelectResource={vi.fn()}
        onOpenResource={vi.fn()}
        onClose={vi.fn()}
      />,
    );
    const before = Number(screen.getByTestId('release-detail').dataset.mounts);

    rerender(
      <ReleasePanel
        target={{ namespace: 'demo', name: 'grafana' }}
        onSelectResource={vi.fn()}
        onOpenResource={vi.fn()}
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId('release-detail')).toHaveTextContent('demo/grafana');
    expect(Number(screen.getByTestId('release-detail').dataset.mounts)).toBeGreaterThan(before);
  });
});
