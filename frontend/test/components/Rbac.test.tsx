import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Rbac from '../../src/components/Rbac';
import { useToastsStore } from '../../src/store/toasts';
import type { RBACIndex, RBACSubject } from '../../src/lib/types';

function subject(extra: Partial<RBACSubject> = {}): RBACSubject {
  return {
    kind: 'ServiceAccount',
    name: 'api',
    namespace: 'web',
    label: 'system:serviceaccount:web:api',
    grants: [
      {
        binding: 'read-web',
        bindingKind: 'RoleBinding',
        role: 'reader',
        roleKind: 'Role',
        namespace: 'web',
        rules: [{ verbs: ['get'], resources: ['pods'], groups: [''] }],
      },
    ],
    namespaces: ['web'],
    ...extra,
  };
}

function stub(body: RBACIndex, who?: RBACIndex) {
  const fetcher = vi.fn((url: string) => {
    const answer = url.includes('/who') && who !== undefined ? who : body;
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(answer) });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.setState({ toasts: [], history: [] });
});

describe('who can do what', () => {
  it('lists every subject and what it holds', async () => {
    stub({ subjects: [subject({ powers: ['reads secrets'] })] });

    render(<Rbac />);

    expect(await screen.findByText('system:serviceaccount:web:api')).toBeTruthy();
    expect(screen.getByText('reads secrets')).toBeTruthy();
    expect(screen.getByText('web')).toBeTruthy();
  });

  it('says when a subject holds nothing worth naming', async () => {
    stub({ subjects: [subject()] });

    render(<Rbac />);

    expect(await screen.findByText('nothing worth naming')).toBeTruthy();
  });

  it('opens a subject to show the bindings behind it', async () => {
    const user = userEvent.setup();
    stub({ subjects: [subject()] });
    render(<Rbac />);
    await screen.findByText('system:serviceaccount:web:api');

    await user.click(screen.getByRole('button', { expanded: false }));

    expect(await screen.findByText(/RoleBinding read-web/)).toBeTruthy();
    expect(screen.getByText('get on pods')).toBeTruthy();
  });

  it('says when a binding names a role that is not there', async () => {
    const user = userEvent.setup();
    stub({
      subjects: [
        subject({
          grants: [
            {
              binding: 'read-web',
              bindingKind: 'RoleBinding',
              role: 'gone',
              roleKind: 'Role',
              namespace: 'web',
              missing: true,
            },
          ],
        }),
      ],
    });
    render(<Rbac />);
    await screen.findByText('system:serviceaccount:web:api');

    await user.click(screen.getByRole('button', { expanded: false }));

    expect(await screen.findByText('the role it names does not exist')).toBeTruthy();
  });

  it('asks who can do a thing and shows only them', async () => {
    const user = userEvent.setup();
    const calls = stub(
      { subjects: [subject(), subject({ name: 'other', label: 'other' })] },
      { subjects: [subject()] },
    );
    render(<Rbac />);
    await screen.findByText('system:serviceaccount:web:api');

    await user.type(screen.getByLabelText('Verb'), 'create');
    await user.type(screen.getByLabelText('Resource'), 'pods/exec');
    await user.click(screen.getByRole('button', { name: 'Ask' }));

    await waitFor(() => {
      expect(calls.mock.calls.some((call) => call[0].includes('/api/rbac/who'))).toBe(true);
    });
    expect(await screen.findByText('1 can')).toBeTruthy();
  });

  it('narrows the question by api group and namespace', async () => {
    const user = userEvent.setup();
    const calls = stub({ subjects: [subject()] }, { subjects: [] });
    render(<Rbac />);
    await screen.findByText('system:serviceaccount:web:api');

    await user.type(screen.getByLabelText('Verb'), 'create');
    await user.type(screen.getByLabelText('Resource'), 'deployments');
    await user.type(screen.getByLabelText('API group'), 'apps');
    await user.type(screen.getByLabelText('Namespace'), 'web');
    await user.click(screen.getByRole('button', { name: 'Ask' }));

    await waitFor(() => {
      const asked = calls.mock.calls.map((call) => call[0]).filter((url) => url.includes('/who'));
      expect(asked.some((url) => url.includes('group=apps') && url.includes('namespace=web'))).toBe(
        true,
      );
    });
  });

  it('will not ask without a verb and a resource', async () => {
    stub({ subjects: [subject()] });

    render(<Rbac />);

    expect(await screen.findByRole('button', { name: 'Ask' })).toBeDisabled();
  });

  it('goes back to everyone', async () => {
    const user = userEvent.setup();
    stub({ subjects: [subject(), subject({ name: 'other', label: 'other' })] }, { subjects: [] });
    render(<Rbac />);
    await screen.findByText('2 subjects');
    await user.type(screen.getByLabelText('Verb'), 'create');
    await user.type(screen.getByLabelText('Resource'), 'pods');
    await user.click(screen.getByRole('button', { name: 'Ask' }));
    await screen.findByText('0 can');

    await user.click(screen.getByRole('button', { name: 'Everyone' }));

    expect(await screen.findByText('2 subjects')).toBeTruthy();
  });

  it('says so when the question fails', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/who')) {
          return Promise.resolve({
            ok: false,
            status: 400,
            json: () => Promise.resolve({ message: 'verb and resource are both required' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ subjects: [subject()] }),
        });
      }),
    );
    render(<Rbac />);
    await screen.findByText('system:serviceaccount:web:api');
    await user.type(screen.getByLabelText('Verb'), 'create');
    await user.type(screen.getByLabelText('Resource'), 'pods');

    await user.click(screen.getByRole('button', { name: 'Ask' }));

    await waitFor(() => {
      const said = useToastsStore.getState().toasts.map((toast) => toast.message);
      expect(said.some((message) => message.includes('Asking who can'))).toBe(true);
    });
  });

  it('filters the list', async () => {
    const user = userEvent.setup();
    stub({ subjects: [subject(), subject({ name: 'other', label: 'other', namespace: 'db' })] });
    render(<Rbac />);
    await screen.findByText('2 subjects');

    await user.type(screen.getByLabelText('Filter subjects'), 'other');

    expect(await screen.findByText('1 subjects')).toBeTruthy();
  });

  it('says nobody when nothing matches', async () => {
    const user = userEvent.setup();
    stub({ subjects: [subject()] });
    render(<Rbac />);
    await screen.findByText('1 subjects');

    await user.type(screen.getByLabelText('Filter subjects'), 'zzz');

    expect(await screen.findByText('Nobody.')).toBeTruthy();
  });

  it('says how many it left out at the cap', async () => {
    stub({ subjects: [subject()], dropped: 40 });

    render(<Rbac />);

    expect(await screen.findByText('40 more are not shown')).toBeTruthy();
  });

  it('says how many bindings point at nothing', async () => {
    stub({ subjects: [subject()], absent: ['RoleBinding web/read wants Role gone'] });

    render(<Rbac />);

    expect(await screen.findByText(/1 bindings name a role that does not exist/)).toBeTruthy();
  });

  it('says what it could not read', async () => {
    stub({ subjects: [], error: 'not discovered: clusterroles' });

    render(<Rbac />);

    expect(await screen.findByText('not discovered: clusterroles')).toBeTruthy();
  });

  it('says so when the index cannot be read at all', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));

    render(<Rbac />);

    expect(await screen.findByText(/network down/)).toBeTruthy();
  });
});
