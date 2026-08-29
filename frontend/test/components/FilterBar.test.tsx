import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import FilterBar from '../../src/components/FilterBar';
import { fieldsOf } from '../../src/lib/filterChips';
import { useFiltersStore } from '../../src/store/filters';
import { ALL, namespaceNow, useNamespaceStore } from '../../src/store/namespace';
import { makeRow } from '../helpers';
import { activeClusterNow } from '../../src/store/clusters';
import type { Chip } from '../../src/lib/filterChips';

const KEY = '/v1/pods';

const fields = fieldsOf([{ name: 'Status' }], true);

const clusterFields = fieldsOf([{ name: 'Status' }], false);

function chipsOf() {
  return chipsByKind()[KEY];
}

const rows = [
  makeRow({ uid: 'a', name: 'web-1', namespace: 'prod', cells: ['Running'] }),
  makeRow({ uid: 'b', name: 'api-1', namespace: 'staging', cells: ['CrashLoopBackOff'] }),
];

function Harness({ own = fields, held = rows }: { own?: typeof fields; held?: typeof rows }) {
  const [text, setText] = useState('');
  return <FilterBar stateKey={KEY} fields={own} rows={held} text={text} onText={setText} />;
}

function filterBox(): HTMLElement {
  return screen.getByLabelText('Filter');
}

describe('FilterBar', () => {
  beforeEach(() => {
    useFiltersStore.getState().clear();
    useNamespaceStore.getState().choose(ALL);
  });

  afterEach(() => {
    useFiltersStore.getState().clear();
    useNamespaceStore.getState().choose(ALL);
  });

  it('keeps typed text as a chip when Enter is pressed', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'web{Enter}');

    expect(chipsOf()).toEqual([{ field: 'name', value: 'web' }]);
    expect(filterBox()).toHaveValue('');
  });

  it('shows the field a chip filters on', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'status:Running{Enter}');

    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
  });

  it('has nothing to keep when the text is only a field', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'status:{Enter}');

    expect(chipsOf()).toBeUndefined();
    expect(filterBox()).toHaveValue('status:');
  });

  it('drops a chip on its remove button', async () => {
    const user = userEvent.setup();
    useFiltersStore.getState().add(KEY, { field: 'name', value: 'web' });
    render(<Harness />);

    await user.click(screen.getByRole('button', { name: 'Remove the Name web filter' }));

    expect(chipsOf()).toBeUndefined();
  });

  it('drops the last chip on Backspace in an empty box', async () => {
    const user = userEvent.setup();
    useFiltersStore.getState().add(KEY, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(KEY, { field: 'status', value: 'Running' });
    render(<Harness />);

    await user.type(filterBox(), '{Backspace}');

    expect(chipsOf()).toEqual([{ field: 'name', value: 'web' }]);
  });

  it('leaves the chips alone while Backspace still has text to delete', async () => {
    const user = userEvent.setup();
    useFiltersStore.getState().add(KEY, { field: 'name', value: 'web' });
    render(<Harness />);

    await user.type(filterBox(), 'ap{Backspace}');

    expect(chipsOf()).toHaveLength(1);
    expect(filterBox()).toHaveValue('a');
  });

  it('has nothing to drop on Backspace with no chips at all', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), '{Backspace}');

    expect(chipsOf()).toBeUndefined();
  });

  it('ignores keys that neither commit nor delete', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'we{ArrowLeft}b');

    expect(chipsOf()).toBeUndefined();
    expect(filterBox()).toHaveValue('wbe');
  });

  it('shows the chosen namespace as a chip', () => {
    useNamespaceStore.getState().choose('prod');
    render(<Harness />);

    expect(screen.getByText('Namespace:')).toBeInTheDocument();
    expect(screen.getByText('prod')).toBeInTheDocument();
  });

  it('goes back to every namespace when that chip is dropped', async () => {
    const user = userEvent.setup();
    useNamespaceStore.getState().choose('prod');
    render(<Harness />);

    await user.click(screen.getByRole('button', { name: 'Remove the Namespace prod filter' }));

    expect(namespaceNow()).toBe(ALL);
  });

  it('changes the namespace scope when one is typed', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'ns:kube-system{Enter}');

    expect(namespaceNow()).toBe('kube-system');
    expect(chipsOf()).toBeUndefined();
  });

  it('leaves the namespace out for a cluster-scoped kind', () => {
    useNamespaceStore.getState().choose('prod');
    render(<Harness own={clusterFields} />);

    expect(screen.queryByText('Namespace:')).not.toBeInTheDocument();
  });

  it('asks for a filter until there is one, then for another', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    expect(filterBox()).toHaveAttribute('placeholder', 'Filter by name, or field:value');

    await user.type(filterBox(), 'web{Enter}');

    expect(filterBox()).toHaveAttribute('placeholder', 'Add a filter');
  });

  it('reports every keystroke so the list can narrow as you type', async () => {
    const user = userEvent.setup();
    const onText = vi.fn();
    render(<FilterBar stateKey={KEY} fields={fields} rows={rows} text="" onText={onText} />);

    await user.type(filterBox(), 'w');

    expect(onText).toHaveBeenCalledWith('w');
  });
});

describe('completing what is typed', () => {
  beforeEach(() => {
    useFiltersStore.getState().clear();
    setScope(ALL, ['airbyte', 'airbyte-jobs', 'prod']);
  });

  afterEach(() => {
    useFiltersStore.getState().clear();
    setScope(ALL, []);
  });

  it('offers nothing until something is typed', () => {
    render(<Harness />);

    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(filterBox()).toHaveAttribute('aria-expanded', 'false');
  });

  it('waits rather than scoping to a namespace the cluster does not have', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'ns:kube{Enter}');

    expect(namespaceNow()).toBe(ALL);
    expect(filterBox()).toHaveValue('ns:kube');
  });

  it('scopes to a namespace typed in full', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'ns:prod{Enter}');

    expect(namespaceNow()).toBe('prod');
  });

  it('leaves the arrows and Tab alone while no list is open', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(filterBox());

    await user.keyboard('{ArrowDown}{ArrowUp}');
    await user.tab();

    expect(chipsOf()).toBeUndefined();
    expect(filterBox()).not.toHaveFocus();
  });

  it('offers the fields a kind can be filtered by', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'stat');

    expect(screen.getByRole('option', { name: /Status:/ })).toBeInTheDocument();
  });

  it('offers the namespaces the cluster reported', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'ns:airbyte-');

    expect(screen.getByRole('option', { name: /airbyte-jobs/ })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /^prod/ })).not.toBeInTheDocument();
  });

  it('offers the values on screen for a column', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'status:');

    expect(screen.getByRole('option', { name: /Running/ })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: /CrashLoopBackOff/ })).toBeInTheDocument();
  });

  it('fills the box with a field and waits for its value', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'stat');

    await user.keyboard('{ArrowDown}{Enter}');

    expect(filterBox()).toHaveValue('status:');
    expect(chipsOf()).toBeUndefined();
  });

  it('keeps a value as a chip as soon as it is chosen', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:run');

    await user.keyboard('{ArrowDown}{Enter}');

    expect(chipsOf()).toEqual([{ field: 'status', value: 'Running' }]);
    expect(filterBox()).toHaveValue('');
  });

  it('takes a chosen namespace as the scope', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'ns:airbyte-');

    await user.keyboard('{ArrowDown}{Enter}');

    expect(namespaceNow()).toBe('airbyte-jobs');
  });

  it('takes the first suggestion on Tab', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:crash');

    await user.tab();

    expect(chipsOf()).toEqual([{ field: 'status', value: 'CrashLoopBackOff' }]);
  });

  it('takes what was typed when nothing is picked', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'status:run{Enter}');

    expect(chipsOf()).toEqual([{ field: 'status', value: 'run' }]);
  });

  it('walks down the list and back off the top of it', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:');

    await user.keyboard('{ArrowDown}{ArrowDown}{ArrowUp}{ArrowUp}{Enter}');

    expect(chipsOf()).toBeUndefined();
    expect(filterBox()).toHaveValue('status:');
  });

  it('stops at the end of the list', async () => {
    const user = userEvent.setup();
    render(<Harness held={[rows[0]]} />);
    await user.type(filterBox(), 'status:');

    await user.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{Enter}');

    expect(chipsOf()).toEqual([{ field: 'status', value: 'Running' }]);
  });

  it('puts the list away on Escape and brings it back on the next keystroke', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:');
    expect(screen.getByRole('listbox')).toBeInTheDocument();

    await user.keyboard('{Escape}');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();

    await user.type(filterBox(), 'r');
    expect(screen.getByRole('listbox')).toBeInTheDocument();
  });

  it('leaves Escape to the rest of the app when no list is open', async () => {
    const user = userEvent.setup();
    const seen: string[] = [];
    const onKeyDown = (event: KeyboardEvent) => {
      seen.push(event.key);
    };
    window.addEventListener('keydown', onKeyDown);
    render(<Harness />);

    await user.click(filterBox());
    await user.keyboard('{Escape}');

    expect(seen).toContain('Escape');
    window.removeEventListener('keydown', onKeyDown);
  });

  it('takes a suggestion that is clicked', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:');

    await user.click(screen.getByRole('option', { name: /Running/ }));

    expect(chipsOf()).toEqual([{ field: 'status', value: 'Running' }]);
  });

  it('marks the highlighted option for assistive technology', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(filterBox(), 'status:');

    await user.keyboard('{ArrowDown}');

    const active = filterBox().getAttribute('aria-activedescendant');
    expect(active).not.toBeNull();
    expect(screen.getByRole('option', { selected: true })).toHaveAttribute('id', active);
  });
});

function setScope(namespace: string, names: string[]): void {
  useNamespaceStore.getState().offer(names);
  useNamespaceStore.getState().choose(namespace);
}

function chipsByKind(): Partial<Record<string, Chip[]>> {
  return useFiltersStore.getState().byCluster[activeClusterNow()] ?? {};
}
