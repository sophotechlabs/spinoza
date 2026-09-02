import { describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { TAIL_LINES, useLogStream } from '../../src/lib/useLogStream';
import { bumpClusterEpoch } from '../../src/store/cluster';

interface StreamProps {
  container: string;
  enabled: boolean;
}

function setup(container: string, enabled = true) {
  const subscribeLogs = vi.fn<(subId: string, request: unknown) => void>();
  const unsubscribeLogs = vi.fn<(subId: string) => void>();
  const view = renderHook(
    (props: StreamProps) =>
      useLogStream({
        prefix: 'logs',
        namespace: 'flux-system',
        name: 'web',
        container: props.container,
        enabled: props.enabled,
        subscribeLogs,
        unsubscribeLogs,
      }),
    { initialProps: { container, enabled } },
  );
  return { subscribeLogs, unsubscribeLogs, view };
}

function subIdOf(mock: ReturnType<typeof vi.fn>, call: number): string {
  return mock.mock.calls[call][0] as string;
}

describe('useLogStream', () => {
  it('asks for a followed tail of the container', () => {
    const { subscribeLogs, view } = setup('app');

    expect(subscribeLogs).toHaveBeenCalledWith(expect.stringMatching(/^logs#\d+$/), {
      namespace: 'flux-system',
      name: 'web',
      container: 'app',
      tailLines: TAIL_LINES,
      follow: true,
    });
    expect(view.result.current).toBe(subIdOf(subscribeLogs, 0));
  });

  it('waits until a container is known', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('');

    expect(subscribeLogs).not.toHaveBeenCalled();
    expect(unsubscribeLogs).not.toHaveBeenCalled();
    expect(view.result.current).toBe('');
  });

  it('mints a fresh subId when the container changes, so late frames cannot land in it', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    view.rerender({ container: 'sidecar', enabled: true });

    const second = subIdOf(subscribeLogs, 1);
    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
    expect(second).not.toBe(first);
    expect(subscribeLogs).toHaveBeenLastCalledWith(
      second,
      expect.objectContaining({ container: 'sidecar' }),
    );
    expect(view.result.current).toBe(second);
  });

  it('drops the subId when the container goes away', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    view.rerender({ container: '', enabled: true });

    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
    expect(view.result.current).toBe('');
  });

  it('does not open a stream for a panel nobody is looking at', () => {
    const { subscribeLogs, view } = setup('app', false);

    expect(subscribeLogs).not.toHaveBeenCalled();
    expect(view.result.current).toBe('');
  });

  it('drops the stream when its panel is hidden and opens a fresh one when it comes back', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    view.rerender({ container: 'app', enabled: false });

    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
    expect(view.result.current).toBe('');

    view.rerender({ container: 'app', enabled: true });

    const second = subIdOf(subscribeLogs, 1);
    expect(second).not.toBe(first);
    expect(view.result.current).toBe(second);
  });

  it('stops the stream when it goes away', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    view.unmount();

    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
  });

  it('replaces the stream after a cluster switch', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    act(() => {
      bumpClusterEpoch();
    });

    const second = subIdOf(subscribeLogs, 1);
    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
    expect(second).not.toBe(first);
    expect(view.result.current).toBe(second);
  });
});
