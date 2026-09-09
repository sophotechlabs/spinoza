import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import FrameworkPosture from '../../src/components/FrameworkPosture';
import type { FrameworkPosture as Posture } from '../../src/lib/types';

const posture: Posture = {
  frameworks: ['CIS Kubernetes Benchmark', 'PSS baseline'],
  controls: [
    {
      framework: 'CIS Kubernetes Benchmark',
      control: '5.2.2',
      title: 'Minimize the admission of privileged containers',
      scope: 'covered',
      checks: ['privileged-container'],
      failing: 3,
      objects: 2,
      covered: true,
    },
    {
      framework: 'CIS Kubernetes Benchmark',
      control: '5.2.4',
      title: 'Minimize the admission of hostPID containers',
      scope: 'covered',
      checks: ['host-namespaces'],
      failing: 0,
      covered: true,
    },
    {
      framework: 'CIS Kubernetes Benchmark',
      control: '5.1.9',
      title: 'Minimize access to create persistent volumes',
      scope: 'uncovered',
      failing: 0,
    },
    {
      framework: 'CIS Kubernetes Benchmark',
      control: '4.2',
      title: 'Kubelet',
      scope: 'out of scope',
      reason: 'the kubelet reads this from its own configuration on the node',
      failing: 0,
    },
  ],
};

function stub(body: unknown, ok = true) {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      asked.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
      });
    }),
  );
  return asked;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('FrameworkPosture', () => {
  it('says what fails a control and what nothing fails', async () => {
    stub(posture);

    render(<FrameworkPosture />);

    expect(await screen.findByText('5.2.2')).toBeInTheDocument();
    expect(screen.getByText('3 findings on 2 objects')).toBeInTheDocument();
    expect(screen.getByText('nothing fails this')).toBeInTheDocument();
  });

  it('shows an uncovered control rather than dropping it', async () => {
    stub(posture);

    render(<FrameworkPosture />);

    expect(await screen.findByText('5.1.9')).toBeInTheDocument();
    expect(screen.getByText('no check answers this')).toBeInTheDocument();
  });

  it('says why a control is out of scope', async () => {
    stub(posture);

    render(<FrameworkPosture />);

    expect(await screen.findByText('4.2')).toBeInTheDocument();
    expect(
      screen.getByText('the kubelet reads this from its own configuration on the node'),
    ).toBeInTheDocument();
  });

  it('counts only the controls it actually checked', async () => {
    stub(posture);

    render(<FrameworkPosture />);

    expect(
      await screen.findByText('1 of 2 checked controls have something failing'),
    ).toBeInTheDocument();
  });

  it('narrows to one framework', async () => {
    const asked = stub(posture);

    render(<FrameworkPosture />);
    await screen.findByText('5.2.2');
    await userEvent.selectOptions(screen.getByLabelText('Framework'), 'PSS baseline');

    await waitFor(() => {
      expect(asked.some((one) => one.includes('framework=PSS+baseline'))).toBe(true);
    });
  });

  it('says so when a framework has nothing cataloged', async () => {
    stub({ frameworks: ['NSA/CISA'], controls: [], reason: 'no numbered controls are cataloged' });

    render(<FrameworkPosture />);

    expect(await screen.findByText(/no numbered controls are cataloged/)).toBeInTheDocument();
    expect(screen.getByText('No controls are cataloged for that framework.')).toBeInTheDocument();
  });

  it('names what went wrong rather than an empty list', async () => {
    stub({ message: 'this view reads the whole cluster' }, false);

    render(<FrameworkPosture />);

    expect(await screen.findByText(/this view reads the whole cluster/)).toBeInTheDocument();
  });
});
