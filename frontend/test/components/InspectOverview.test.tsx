import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import InspectOverview from '../../src/components/InspectOverview';
import type { ContainerState, ObjectDetail } from '../../src/lib/types';

function detail(overrides: Partial<ObjectDetail> = {}): ObjectDetail {
  return {
    apiVersion: 'v1',
    kind: 'Pod',
    name: 'web',
    namespace: 'flux-system',
    uid: 'pod-uid',
    createdAt: '2026-07-27T09:00:00Z',
    yaml: 'kind: Pod\n',
    ...overrides,
  };
}

describe('InspectOverview', () => {
  it('renders the metadata block', () => {
    render(<InspectOverview detail={detail()} />);

    expect(screen.getByText('Metadata')).toBeInTheDocument();
    expect(screen.getByText('web')).toBeInTheDocument();
    expect(screen.getByText('flux-system')).toBeInTheDocument();
    expect(screen.getByText('pod-uid')).toBeInTheDocument();
  });

  it('omits optional sections when there is nothing to show', () => {
    render(<InspectOverview detail={detail()} />);

    expect(screen.queryByText('Conditions')).not.toBeInTheDocument();
    expect(screen.queryByText('Labels')).not.toBeInTheDocument();
    expect(screen.queryByText('Annotations')).not.toBeInTheDocument();
    expect(screen.queryByText('Owner references')).not.toBeInTheDocument();
    expect(screen.queryByText('Containers')).not.toBeInTheDocument();
  });

  it('renders conditions with their status colour and message', () => {
    render(
      <InspectOverview
        detail={detail({
          conditions: [
            { type: 'Ready', status: 'True', message: 'all good', updated: '2026-07-27T09:01:00Z' },
            { type: 'Stalled', status: 'False' },
            { type: 'Unknown', status: 'Unknown' },
          ],
        })}
      />,
    );

    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText('True')).toHaveClass('text-ok');
    expect(screen.getByText('False')).toHaveClass('text-error');
    expect(screen.getByText('Unknown', { selector: 'span.text-fg-muted' })).toBeInTheDocument();
    expect(screen.getByText('all good')).toBeInTheDocument();
  });

  it('hides an empty condition message', () => {
    render(
      <InspectOverview
        detail={detail({ conditions: [{ type: 'Ready', status: 'True', message: '' }] })}
      />,
    );

    expect(screen.getByText('Ready')).toBeInTheDocument();
  });

  it('renders sorted labels and annotations', () => {
    render(
      <InspectOverview
        detail={detail({
          labels: { zone: 'b', app: 'web' },
          annotations: { note: 'keep' },
        })}
      />,
    );

    expect(screen.getByText('Labels')).toBeInTheDocument();
    const labelKeys = screen.getAllByRole('term').map((node) => node.textContent);
    expect(labelKeys).toContain('app');
    expect(labelKeys.indexOf('app')).toBeLessThan(labelKeys.indexOf('zone'));
    expect(screen.getByText('Annotations')).toBeInTheDocument();
    expect(screen.getByText('keep')).toBeInTheDocument();
  });

  it('renders owner references', () => {
    render(
      <InspectOverview
        detail={detail({ owners: [{ kind: 'ReplicaSet', name: 'web-abc', uid: 'rs-uid' }] })}
      />,
    );

    expect(screen.getByText('Owner references')).toBeInTheDocument();
    expect(screen.getByText('web-abc')).toBeInTheDocument();
  });

  it('renders container states with their colours', () => {
    const containers: ContainerState[] = [
      { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
      {
        name: 'crashed',
        state: 'terminated',
        reason: 'Error',
        ready: false,
        restarts: 4,
        init: false,
      },
      {
        name: 'pending',
        state: 'waiting',
        reason: 'PodInitializing',
        ready: false,
        restarts: 0,
        init: true,
      },
    ];
    const { container } = render(<InspectOverview detail={detail()} containers={containers} />);

    expect(screen.getByText('Containers')).toBeInTheDocument();
    expect(screen.getByText('app')).toBeInTheDocument();
    expect(screen.getByText('4 restarts')).toBeInTheDocument();
    expect(container.querySelectorAll('.bg-ok-solid')).toHaveLength(1);
    expect(container.querySelectorAll('.bg-error-solid')).toHaveLength(1);
    expect(container.querySelectorAll('.bg-warn-solid')).toHaveLength(1);
  });

  it('treats a running but unready container as pending', () => {
    const containers: ContainerState[] = [
      { name: 'app', state: 'running', ready: false, restarts: 0, init: false },
    ];
    const { container } = render(<InspectOverview detail={detail()} containers={containers} />);

    expect(container.querySelectorAll('.bg-warn-solid')).toHaveLength(1);
  });
});

describe('container dots agree with the table', () => {
  it('greys a completed init container instead of colouring it like a crash', () => {
    render(
      <InspectOverview
        detail={detail()}
        containers={[
          {
            name: 'copy-libs',
            state: 'terminated',
            reason: 'Completed',
            ready: false,
            restarts: 0,
            init: true,
          },
        ]}
      />,
    );

    const dot = document.querySelector('.bg-idle-solid');
    expect(dot).not.toBeNull();
  });

  it('reds a container stuck in CrashLoopBackOff', () => {
    render(
      <InspectOverview
        detail={detail()}
        containers={[
          {
            name: 'app',
            state: 'waiting',
            reason: 'CrashLoopBackOff',
            ready: false,
            restarts: 7,
            init: false,
          },
        ]}
      />,
    );

    const dot = document.querySelector('.bg-error-solid');
    expect(dot).not.toBeNull();
  });
});

describe('copying a metadata value', () => {
  it('offers a copy button per pair, named after the field', () => {
    render(<InspectOverview detail={detail()} />);

    expect(screen.getByRole('button', { name: 'Copy UID' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy Name' })).toBeInTheDocument();
  });

  it('leaves an empty field without a button to press', () => {
    render(<InspectOverview detail={detail({ namespace: '' })} />);

    expect(screen.queryByRole('button', { name: 'Copy Namespace' })).not.toBeInTheDocument();
  });
});
