import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import FilterBar from '../../src/components/FilterBar';
import { fieldsOf } from '../../src/lib/filterChips';
import { useFiltersStore } from '../../src/store/filters';
import { ALL, useNamespaceStore } from '../../src/store/namespace';

const KEY = '/v1/pods';

const fields = fieldsOf([{ name: 'Status' }], true);

const clusterFields = fieldsOf([{ name: 'Status' }], false);

function chipsOf() {
  return useFiltersStore.getState().chips[KEY];
}

function Harness({ own = fields }: { own?: typeof fields }) {
  const [text, setText] = useState('');
  return <FilterBar stateKey={KEY} fields={own} text={text} onText={setText} />;
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

    expect(useNamespaceStore.getState().namespace).toBe(ALL);
  });

  it('changes the namespace scope when one is typed', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.type(filterBox(), 'ns:kube-system{Enter}');

    expect(useNamespaceStore.getState().namespace).toBe('kube-system');
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
    render(<FilterBar stateKey={KEY} fields={fields} text="" onText={onText} />);

    await user.type(filterBox(), 'w');

    expect(onText).toHaveBeenCalledWith('w');
  });
});
