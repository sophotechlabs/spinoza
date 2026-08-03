import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { TAIL_LINES, useLogStream } from '../../src/lib/useLogStream';

function setup(container: string) {
  const subscribeLogs = vi.fn();
  const unsubscribeLogs = vi.fn();
  const view = renderHook(
    (props: { container: string }) => {
      useLogStream({
        subId: 'logs',
        namespace: 'flux-system',
        name: 'web',
        container: props.container,
        subscribeLogs,
        unsubscribeLogs,
      });
    },
    { initialProps: { container } },
  );
  return { subscribeLogs, unsubscribeLogs, view };
}

describe('useLogStream', () => {
  it('asks for a followed tail of the container', () => {
    const { subscribeLogs } = setup('app');

    expect(subscribeLogs).toHaveBeenCalledWith('logs', {
      namespace: 'flux-system',
      name: 'web',
      container: 'app',
      tailLines: TAIL_LINES,
      follow: true,
    });
  });

  it('waits until a container is known', () => {
    const { subscribeLogs, unsubscribeLogs } = setup('');

    expect(subscribeLogs).not.toHaveBeenCalled();
    expect(unsubscribeLogs).not.toHaveBeenCalled();
  });

  it('swaps the stream when the container changes', () => {
    const { subscribeLogs, unsubscribeLogs, view } = setup('app');

    view.rerender({ container: 'sidecar' });

    expect(unsubscribeLogs).toHaveBeenCalledWith('logs');
    expect(subscribeLogs).toHaveBeenLastCalledWith(
      'logs',
      expect.objectContaining({ container: 'sidecar' }),
    );
  });

  it('stops the stream when it goes away', () => {
    const { unsubscribeLogs, view } = setup('app');

    view.unmount();

    expect(unsubscribeLogs).toHaveBeenCalledWith('logs');
  });
});
