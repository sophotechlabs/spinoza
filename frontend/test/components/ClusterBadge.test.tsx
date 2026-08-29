import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import ClusterBadge from '../../src/components/ClusterBadge';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { MK2, listOf } from '../helpers-clusters';

describe('the cluster a dialog is about', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  it('is named in words, not only in colour', () => {
    adoptClusters(listOf(MK2));

    render(<ClusterBadge />);

    expect(screen.getByText('on p-mk2')).toBeInTheDocument();
  });

  it('carries the colour that tab wears', () => {
    adoptClusters(listOf(MK2));

    const { container } = render(<ClusterBadge />);

    expect(screen.getByLabelText('p-mk2 is colour 2')).toHaveStyle({
      backgroundColor: 'var(--cluster-2)',
    });
    expect(container).not.toBeEmptyDOMElement();
  });

  it('says nothing when no cluster is open', () => {
    const { container } = render(<ClusterBadge />);

    expect(container).toBeEmptyDOMElement();
  });
});
