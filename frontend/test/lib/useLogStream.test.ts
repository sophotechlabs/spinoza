import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { TAIL_LINES, useLogStream } from '../../src/lib/useLogStream';

function setup(container: string) {
  const subscribeLogs = vi.fn<(subId: string, request: unknown) => void>();
  const unsubscribeLogs = vi.fn<(subId: string) => void>();
  const view = renderHook(
    (props: { container: string }) =>
      useLogStream({
        prefix: 'logs',
        namespace: 'flux-system',
        name: 'web',
        container: props.container,
        subscribeLogs,
        unsubscribeLogs,
      }),
    { initialProps: { container } },
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

    view.rerender({ container: 'sidecar' });

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

    view.rerender({ container: '' });

    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
    expect(view.result.current).toBe('');
  });

  it('stops the stream when it goes away', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');
    const first = subIdOf(subscribeLogs, 0);

    view.unmount();

    expect(unsubscribeLogs).toHaveBeenCalledWith(first);
  });
});
