import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import Sidebar from '../../src/components/Sidebar';

describe('Sidebar', () => {
  it('renders the Workloads group with Pods active', () => {
    render(<Sidebar />);
    expect(screen.getByText('Workloads')).toBeInTheDocument();
    expect(screen.getByText('Pods')).toBeInTheDocument();
  });

  it('renders the inactive resource groups', () => {
    render(<Sidebar />);
    const groups = ['Config', 'Network', 'Storage', 'Access Control', 'Custom Resources'];
    for (const group of groups) {
      expect(screen.getByText(group)).toBeInTheDocument();
    }
  });
});
