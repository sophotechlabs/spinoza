import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DebugPrompt from '../../src/components/DebugPrompt';
import { useToastsStore } from '../../src/store/toasts';

const target = { namespace: 'monitoring', pod: 'loki-0', container: 'loki' };

function renderPrompt() {
  const onAttached = vi.fn();
  render(<DebugPrompt target={target} onAttached={onAttached} />);
  return { onAttached };
}

function stubStart(container: string) {
  const fetchMock = vi.fn((url: string) => {
    if (url.startsWith('/api/debug/support')) {
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            namespace: 'monitoring',
            pod: 'loki-0',
            allowed: true,
            image: 'busybox:1.37',
          }),
      });
    }
    return Promise.resolve({
      ok: true,
      json: () =>
        Promise.resolve({ container, created: true, image: 'busybox:1.37', profile: 'general' }),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function stubRefusal(reason: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            namespace: 'monitoring',
            pod: 'loki-0',
            allowed: false,
            reason,
            image: 'busybox:1.37',
          }),
      }),
    ),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('DebugPrompt', () => {
  it('explains why exec is unavailable', () => {
    stubStart('spinoza-debug-1');
    renderPrompt();

    expect(screen.getByText(/loki has no shell/)).toBeInTheDocument();
    expect(screen.getByText(/cannot be removed afterwards/)).toBeInTheDocument();
  });

  it('names the image the server would actually run', async () => {
    const seen: string[] = [];
    const fetchMock = vi.fn((url: string) => {
      seen.push(url);
      return Promise.resolve({
        ok: true,
        json: () =>
          Promise.resolve({
            namespace: 'monitoring',
            pod: 'loki-0',
            allowed: true,
            image: 'ghcr.io/acme/toolbox:2.1',
          }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderPrompt();

    expect(await screen.findByText('ghcr.io/acme/toolbox:2.1')).toBeInTheDocument();
    expect(screen.queryByText('busybox:1.37')).not.toBeInTheDocument();
    expect(seen[0]).toContain('pod=loki-0');
  });

  it('defaults to the non-privileged profile', () => {
    renderPrompt();

    expect(screen.getByLabelText('Debug profile')).toHaveValue('general');
    expect(screen.queryByText(/runs the debug container privileged/)).not.toBeInTheDocument();
  });

  it('reports the attached container upward', async () => {
    const user = userEvent.setup();
    const fetchMock = stubStart('spinoza-debug-1');
    const { onAttached } = renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    await waitFor(() => {
      expect(onAttached).toHaveBeenCalledWith('spinoza-debug-1');
    });
    const post = fetchMock.mock.calls.find((call) => call[0].startsWith('/api/debug?'));
    expect(post?.[0]).toContain('profile=general');
  });

  it('sends the selected profile', async () => {
    const user = userEvent.setup();
    const fetchMock = stubStart('spinoza-debug-1');
    renderPrompt();

    await user.selectOptions(screen.getByLabelText('Debug profile'), 'netadmin');
    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    await waitFor(() => {
      const post = fetchMock.mock.calls.find((call) => call[0].startsWith('/api/debug?'));
      expect(post?.[0]).toContain('profile=netadmin');
    });
  });

  it('warns before running privileged', async () => {
    const user = userEvent.setup();
    renderPrompt();

    await user.selectOptions(screen.getByLabelText('Debug profile'), 'sysadmin');

    expect(screen.getByText(/runs the debug container privileged/)).toBeInTheDocument();
  });

  it('surfaces a failure and does not report success', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'cannot patch ephemeralcontainers' }),
      }),
    );
    const { onAttached } = renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByText('cannot patch ephemeralcontainers')).toBeInTheDocument();
    expect(onAttached).not.toHaveBeenCalled();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByText('starting a debug container failed')).toBeInTheDocument();
  });

  it('disables the button while starting', async () => {
    const user = userEvent.setup();
    const deferred = {
      release: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            deferred.release = () => {
              resolve({
                ok: true,
                json: () =>
                  Promise.resolve({
                    container: 'spinoza-debug-1',
                    created: true,
                    image: '',
                    profile: '',
                  }),
              });
            };
          }),
      ),
    );
    renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    expect(await screen.findByRole('button', { name: 'Starting…' })).toBeDisabled();
    deferred.release();
  });

  it('refuses to offer an attach the kubeconfig cannot perform', async () => {
    stubRefusal('no RBAC policy matched');
    renderPrompt();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Attach debug container' })).toBeDisabled();
    });
    expect(
      screen.getByText(/Your kubeconfig cannot add debug containers in monitoring/),
    ).toBeInTheDocument();
    expect(screen.getByText(/no RBAC policy matched/)).toBeInTheDocument();
  });

  it('refuses without a reason when the server gives none', async () => {
    stubRefusal('');
    renderPrompt();

    await waitFor(() => {
      expect(
        screen.getByText('Your kubeconfig cannot add debug containers in monitoring.'),
      ).toBeInTheDocument();
    });
  });

  it('stays usable when the support check itself fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    renderPrompt();

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Attach debug container' })).not.toBeDisabled();
    });
  });

  it('says the support check failed instead of swallowing it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    renderPrompt();

    expect(
      await screen.findByText(/Could not check whether debug containers are allowed/),
    ).toBeInTheDocument();
    expect(screen.getByText(/offline/)).toBeInTheDocument();
  });

  it('drops a support failure that lands after the prompt is gone', async () => {
    const deferred = {
      reject: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            deferred.reject = () => {
              reject(new Error('too late'));
            };
          }),
      ),
    );
    const view = render(<DebugPrompt target={target} onAttached={vi.fn()} />);
    view.unmount();

    deferred.reject();

    await waitFor(() => {
      expect(screen.queryByText(/too late/)).not.toBeInTheDocument();
    });
  });

  it('names no cause when the support check rejects with a non-Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderPrompt();

    expect(
      await screen.findByText(/Could not check whether debug containers are allowed here\./),
    ).toBeInTheDocument();
  });

  it('says when it reused a container running under another profile', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/debug/support')) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({ namespace: 'monitoring', allowed: true, image: 'busybox:1.37' }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              container: 'spinoza-debug-1',
              created: false,
              image: 'busybox:1.37',
              profile: 'general',
            }),
        });
      }),
    );
    const { onAttached } = renderPrompt();

    await user.selectOptions(screen.getByLabelText('Debug profile'), 'netadmin');
    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    await waitFor(() => {
      expect(onAttached).toHaveBeenCalledWith('spinoza-debug-1');
    });
    const toast = useToastsStore.getState().toasts.at(-1);
    expect(toast?.tone).toBe('warn');
    expect(toast?.message).toContain('runs under the general profile, not netadmin');
  });

  it('stays quiet when the container it reused matches the profile asked for', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/debug/support')) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({ namespace: 'monitoring', allowed: true, image: 'busybox:1.37' }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              container: 'spinoza-debug-1',
              created: false,
              image: 'busybox:1.37',
              profile: 'general',
            }),
        });
      }),
    );
    const { onAttached } = renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    await waitFor(() => {
      expect(onAttached).toHaveBeenCalledWith('spinoza-debug-1');
    });
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('stays quiet when the server reports no profile at all', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/debug/support')) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({ namespace: 'monitoring', allowed: true, image: 'busybox:1.37' }),
          });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              container: 'spinoza-debug-1',
              created: false,
              image: 'busybox:1.37',
              profile: '',
            }),
        });
      }),
    );
    const { onAttached } = renderPrompt();

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));

    await waitFor(() => {
      expect(onAttached).toHaveBeenCalledWith('spinoza-debug-1');
    });
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('drops a debug container that finishes starting after the pod changed', async () => {
    const user = userEvent.setup();
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (url: string) =>
          new Promise((resolve) => {
            if (url.startsWith('/api/debug/support')) {
              resolve({
                ok: true,
                json: () =>
                  Promise.resolve({ namespace: 'monitoring', allowed: true, image: 'busybox' }),
              });
              return;
            }
            deferred.settle = () => {
              resolve({
                ok: true,
                json: () =>
                  Promise.resolve({
                    container: 'debugger-from-loki',
                    created: true,
                    image: '',
                    profile: '',
                  }),
              });
            };
          }),
      ),
    );
    const onAttached = vi.fn();
    const view = render(<DebugPrompt target={target} onAttached={onAttached} />);

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));
    view.rerender(
      <DebugPrompt
        target={{ namespace: 'monitoring', pod: 'promtail-9', container: 'promtail' }}
        onAttached={onAttached}
      />,
    );
    deferred.settle();

    await waitFor(() => {
      expect(screen.getByText(/promtail has no shell/)).toBeInTheDocument();
    });
    expect(onAttached).not.toHaveBeenCalled();
  });

  it('drops an attach failure that lands after the pod changed', async () => {
    const user = userEvent.setup();
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (url: string) =>
          new Promise((resolve, reject) => {
            if (url.startsWith('/api/debug/support')) {
              resolve({
                ok: true,
                json: () =>
                  Promise.resolve({ namespace: 'monitoring', allowed: true, image: 'busybox' }),
              });
              return;
            }
            deferred.settle = () => {
              reject(new Error('image pull failed for loki'));
            };
          }),
      ),
    );
    const view = render(<DebugPrompt target={target} onAttached={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Attach debug container' }));
    view.rerender(
      <DebugPrompt
        target={{ namespace: 'monitoring', pod: 'promtail-9', container: 'promtail' }}
        onAttached={vi.fn()}
      />,
    );
    deferred.settle();

    await waitFor(() => {
      expect(screen.getByText(/promtail has no shell/)).toBeInTheDocument();
    });
    expect(screen.queryByText('image pull failed for loki')).not.toBeInTheDocument();
  });

  it('drops a support answer that lands after unmount', () => {
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.settle = () => {
              resolve({
                ok: true,
                json: () =>
                  Promise.resolve({ namespace: 'monitoring', allowed: false, reason: 'too late' }),
              });
            };
          }),
      ),
    );
    const view = render(<DebugPrompt target={target} onAttached={vi.fn()} />);

    view.unmount();
    deferred.settle();

    expect(screen.queryByText(/too late/)).not.toBeInTheDocument();
  });
});
