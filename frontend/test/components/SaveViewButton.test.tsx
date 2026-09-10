import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SaveViewButton from '../../src/components/SaveViewButton';
import { useToastsStore } from '../../src/store/toasts';

function stub(mayShare: boolean, saveOk = true) {
  const calls: { url: string; init?: RequestInit }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (init?.method === 'PUT') {
        return Promise.resolve({
          ok: saveOk,
          status: saveOk ? 200 : 400,
          json: () =>
            Promise.resolve(
              saveOk
                ? { id: 'one', name: 'crashing pods' }
                : { message: 'a saved view needs a name' },
            ),
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ views: [], mayShare }),
      });
    }),
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.setState({ toasts: [], history: [] });
});

describe('SaveViewButton', () => {
  it('asks for a name before it saves anything', async () => {
    const calls = stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));

    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(calls.some((one) => one.init?.method === 'PUT')).toBe(false);
  });

  it('sends what the table is showing', async () => {
    const calls = stub(false);

    render(
      <SaveViewButton
        view="resources"
        resource="pods"
        namespace="prod"
        filter="status:CrashLoopBackOff"
        columns={['name', 'status']}
      />,
    );
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), 'crashing pods');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      const put = calls.find((one) => one.init?.method === 'PUT');
      expect(put).toBeDefined();
      const body = put?.init?.body;
      expect(typeof body).toBe('string');
      const sent = JSON.parse(body as string) as Record<string, unknown>;
      expect(sent.name).toBe('crashing pods');
      expect(sent.resource).toBe('pods');
      expect(sent.namespace).toBe('prod');
      expect(sent.filter).toBe('status:CrashLoopBackOff');
      expect(sent.columns).toEqual(['name', 'status']);
      expect(sent.shared).toBe(false);
    });
  });

  it('offers to publish for everybody only when the cluster allows it', async () => {
    stub(true);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));

    expect(await screen.findByLabelText('everybody')).toBeInTheDocument();
  });

  it('does not offer to publish when the cluster refuses it', async () => {
    stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));

    await waitFor(() => {
      expect(screen.queryByLabelText('everybody')).not.toBeInTheDocument();
    });
  });

  it('says what went wrong rather than pretending it saved', async () => {
    stub(false, false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.some((one) => one.tone === 'error')).toBe(true);
    });
  });

  it('saves on Enter without reaching for the button', async () => {
    const calls = stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), 'crashing pods{Enter}');

    await waitFor(() => {
      expect(calls.some((one) => one.init?.method === 'PUT')).toBe(true);
    });
  });

  it('does nothing on Enter while the name is still empty', async () => {
    const calls = stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), '{Enter}');

    expect(calls.some((one) => one.init?.method === 'PUT')).toBe(false);
  });

  it('closes on Escape', async () => {
    stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), '{Escape}');

    expect(await screen.findByRole('button', { name: 'Save this view' })).toBeInTheDocument();
  });

  it('closes on Cancel', async () => {
    stub(false);

    render(<SaveViewButton view="resources" resource="pods" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(await screen.findByRole('button', { name: 'Save this view' })).toBeInTheDocument();
  });

  it('publishes for everybody once that is ticked', async () => {
    const calls = stub(true);

    render(<SaveViewButton view="checks" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.click(await screen.findByRole('checkbox'));
    await userEvent.type(screen.getByLabelText('Name for this view'), 'posture');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      const put = calls.find((one) => one.init?.method === 'PUT');
      const sent = typeof put?.init?.body === 'string' ? put.init.body : '';
      expect(sent).toContain('"shared":true');
    });
  });

  it('offers no publishing when it cannot ask whether this person may', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ id: 'one', name: 'x' }),
          });
        }
        return Promise.reject(new Error('the connection went away'));
      }),
    );

    render(<SaveViewButton view="checks" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));

    await waitFor(() => {
      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    });
  });

  it('says what stopped the save rather than pretending it landed', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          return Promise.reject(new Error('the connection went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ views: [], mayShare: false }),
        });
      }),
    );

    render(<SaveViewButton view="checks" filter="" columns={[]} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save this view' }));
    await userEvent.type(screen.getByLabelText('Name for this view'), 'posture');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(
        useToastsStore.getState().toasts.some((one) => one.message === 'the connection went away'),
      ).toBe(true);
    });
  });
});
