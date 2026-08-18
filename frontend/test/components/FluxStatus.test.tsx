import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import FluxStatus from '../../src/components/FluxStatus';
import type { FluxOverview } from '../../src/lib/types';

function overview(patch: Partial<FluxOverview> = {}): FluxOverview {
  return {
    ready: true,
    summary: 'the cluster is in sync with what the repository asks for',
    namespace: 'flux-system',
    kubernetes: 'v1.36.2+k3s1',
    nodes: 1,
    operator: 'v0.58.0',
    distribution: 'v2.9.4',
    controllers: [
      {
        name: 'source-controller',
        version: 'v2.9.4',
        ready: true,
        replicas: 1,
        wanted: 1,
        namespace: 'flux-system',
      },
    ],
    sync: {
      namespace: 'flux-system',
      name: 'flux-system',
      kind: 'Kustomization',
      source: 'GitRepository',
      url: 'ssh://git@github.com/sophotechlabs/hetzner-gitops',
      ref: 'refs/heads/main',
      path: './clusters/p-mk2',
      revision: 'refs/heads/main@sha1:abc',
      ready: true,
    },
    usage: {
      cpuMilli: 31,
      memoryMi: 540,
      cpuRequestMilli: 400,
      memRequestMi: 256,
      cpuLimitMilli: 0,
      memLimitMi: 4096,
      known: true,
    },
    ...patch,
  };
}

describe('FluxStatus', () => {
  it('says the cluster is operational and what it is synced to', () => {
    render(<FluxStatus overview={overview()} />);

    expect(screen.getByText('All systems operational')).toBeInTheDocument();
    expect(screen.getByText(/in sync with what the repository asks for/)).toBeInTheDocument();
    expect(screen.getByText('refs/heads/main@sha1:abc')).toBeInTheDocument();
    expect(
      screen.getByText('ssh://git@github.com/sophotechlabs/hetzner-gitops'),
    ).toBeInTheDocument();
    expect(screen.getByText('./clusters/p-mk2')).toBeInTheDocument();
  });

  it('names the versions and the cluster it is watching', () => {
    render(<FluxStatus overview={overview()} />);

    expect(screen.getByText('v0.58.0')).toBeInTheDocument();
    expect(screen.getByText('v2.9.4', { selector: 'span' })).toBeInTheDocument();
    expect(screen.getByText(/v1.36.2\+k3s1 · 1 node/)).toBeInTheDocument();
  });

  it('leaves the operator line out when there is none', () => {
    render(<FluxStatus overview={overview({ operator: undefined })} />);

    expect(screen.queryByText('Flux Operator')).not.toBeInTheDocument();
    expect(screen.getByText('Flux')).toBeInTheDocument();
  });

  it('shows usage against requests and limits', () => {
    render(<FluxStatus overview={overview()} />);

    expect(screen.getByText('31m')).toBeInTheDocument();
    expect(screen.getByText(/8% of request/)).toBeInTheDocument();
    expect(screen.getByText(/13% of limit/)).toBeInTheDocument();
  });

  it('fills the bar against the limit, not the request', () => {
    const { container } = render(<FluxStatus overview={overview()} />);
    const bars = [...container.querySelectorAll('span[style]')].map((one) =>
      one.getAttribute('style'),
    );

    expect(bars[1]).toContain('width: 13%');
  });

  it('falls back to the request when nothing sets a limit', () => {
    const { container } = render(
      <FluxStatus
        overview={overview({
          usage: { ...overview().usage, cpuLimitMilli: 0, memLimitMi: 0 },
        })}
      />,
    );
    const bars = [...container.querySelectorAll('span[style]')].map((one) =>
      one.getAttribute('style'),
    );

    expect(bars[1]).toContain('width: 100%');
  });

  it('says when usage needs metrics-server', () => {
    render(<FluxStatus overview={overview({ usage: { ...overview().usage, known: false } })} />);

    expect(screen.getByText('Usage needs metrics-server.')).toBeInTheDocument();
  });

  it('lists each controller with its version and state', () => {
    render(
      <FluxStatus
        overview={overview({
          ready: false,
          summary: 'helm-controller is not ready',
          controllers: [
            {
              name: 'helm-controller',
              version: 'v2.9.4',
              ready: false,
              replicas: 0,
              wanted: 1,
              namespace: 'flux-system',
            },
          ],
        })}
      />,
    );

    expect(screen.getByText('Something needs attention')).toBeInTheDocument();
    const row = screen.getByText('helm-controller').closest('tr');
    expect(within(row as HTMLElement).getByText('0 of 1 running')).toBeInTheDocument();
  });

  it('says so when the control plane could not be read', () => {
    render(<FluxStatus overview={overview({ error: 'deployments is forbidden' })} />);

    expect(screen.getByText(/deployments is forbidden/)).toBeInTheDocument();
  });

  it('shows nothing on a cluster with no controllers', () => {
    const { container } = render(<FluxStatus overview={overview({ controllers: [] })} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('says when no sync was found', () => {
    render(
      <FluxStatus
        overview={overview({
          sync: { ...overview().sync, kind: '', url: '', ref: '', path: '', revision: '' },
        })}
      />,
    );

    expect(screen.getByText(/No flux-system sync was found/)).toBeInTheDocument();
  });

  it('counts more than one node', () => {
    render(<FluxStatus overview={overview({ nodes: 3 })} />);

    expect(screen.getByText(/3 nodes/)).toBeInTheDocument();
  });

  it('says when nothing constrains the controllers', () => {
    render(
      <FluxStatus
        overview={overview({
          usage: {
            cpuMilli: 31,
            memoryMi: 540,
            cpuRequestMilli: 0,
            memRequestMi: 0,
            cpuLimitMilli: 0,
            memLimitMi: 0,
            known: true,
          },
        })}
      />,
    );

    expect(screen.getAllByText('no request or limit set')).toHaveLength(2);
  });
});
