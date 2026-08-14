import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import KubeconfigBanner from '../../src/components/KubeconfigBanner';
import { useContextsStore } from '../../src/store/contexts';

function connectedThrough(path: string, error?: string): void {
  useContextsStore.getState().setList({
    current: { kubeconfig: path, name: 'p-mk1' },
    kubeconfigs: [
      { label: '/tmp/work.yaml', path: '/tmp/work.yaml', removable: true, contexts: [], error },
    ],
  });
}

describe('KubeconfigBanner', () => {
  it('says nothing while the kubeconfig reads fine', () => {
    connectedThrough('/tmp/work.yaml');

    render(<KubeconfigBanner />);

    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('names the file and the reason once it stops reading', () => {
    connectedThrough(
      '/tmp/work.yaml',
      'kubeconfig: stat /tmp/work.yaml: no such file or directory',
    );

    render(<KubeconfigBanner />);

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('/tmp/work.yaml');
    expect(banner).toHaveTextContent('no such file or directory');
  });

  it('says the running connection is unaffected', () => {
    connectedThrough('/tmp/work.yaml', 'gone');

    render(<KubeconfigBanner />);

    expect(screen.getByRole('status')).toHaveTextContent('The live connection still works');
  });
});
