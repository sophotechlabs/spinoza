import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useShellSupport } from '../../src/lib/useShellSupport';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

function stubShell(shell: string) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ shell }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    useClusterStore.getState().reset();
  });
});

describe('useShellSupport', () => {
  it('reports what the probe found', async () => {
    stubShell('present');

    const { result } = renderHook(() => useShellSupport('prod', 'web', 'app'));

    await waitFor(() => {
      expect(result.current.shell).toBe('present');
    });
    expect(result.current.error).toBeNull();
  });

  it('asks nothing without a pod to ask about', () => {
    const fetchMock = stubShell('present');

    const { result } = renderHook(() => useShellSupport('prod', '', 'app'));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.shell).toBe('unknown');
  });

  it('asks nothing without a container to ask about', () => {
    const fetchMock = stubShell('present');

    const { result } = renderHook(() => useShellSupport('prod', 'web', ''));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.shell).toBe('unknown');
  });

  it('takes the caller at their word that the shell is gone', async () => {
    stubShell('present');
    const { result } = renderHook(() => useShellSupport('prod', 'web', 'app'));
    await waitFor(() => {
      expect(result.current.shell).toBe('present');
    });

    act(() => {
      result.current.markMissing();
    });

    expect(result.current.shell).toBe('absent');
  });

  it('hides the previous pods answer while the replacement loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ shell: 'present' }),
      })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const { result, rerender } = renderHook(
      ({ pod }: { pod: string }) => useShellSupport('prod', pod, 'app'),
      { initialProps: { pod: 'web' } },
    );
    await waitFor(() => {
      expect(result.current.shell).toBe('present');
    });

    rerender({ pod: 'api' });

    expect(result.current.shell).toBe('unknown');
    expect(result.current.error).toBeNull();
  });

  it('hides the previous clusters answer while the replacement loads', async () => {
    const pending = new Promise<never>(() => undefined);
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ shell: 'present' }),
      })
      .mockImplementationOnce(() => pending);
    vi.stubGlobal('fetch', fetchMock);
    const { result, unmount } = renderHook(() => useShellSupport('prod', 'web', 'app'));
    await waitFor(() => {
      expect(result.current.shell).toBe('present');
    });

    await act(async () => {
      bumpClusterEpoch();
      await Promise.resolve();
    });

    expect(result.current.shell).toBe('unknown');
    expect(result.current.error).toBeNull();
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
    unmount();
  });
});
