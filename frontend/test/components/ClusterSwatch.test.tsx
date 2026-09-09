import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import ClusterSwatch from '../../src/components/ClusterSwatch';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { MK2, listOf } from '../helpers-clusters';

describe('the colour the active cluster wears', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  it('names the cluster it stands for', () => {
    adoptClusters(listOf(MK2));

    render(<ClusterSwatch />);

    expect(screen.getByRole('img', { name: 'p-mk2 is colour 2' })).toHaveStyle({
      backgroundColor: 'var(--cluster-2)',
    });
  });

  it('shows nothing while no cluster is open', () => {
    const { container } = render(<ClusterSwatch />);

    expect(container).toBeEmptyDOMElement();
  });
});
