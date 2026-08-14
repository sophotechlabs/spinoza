import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useShellSupport } from '../../src/lib/useShellSupport';

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
});
