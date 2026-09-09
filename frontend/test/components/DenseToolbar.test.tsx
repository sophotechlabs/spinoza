import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DenseToolbar, {
  FilterInput,
  ToolbarCount,
  ToolbarEnd,
} from '../../src/components/DenseToolbar';

describe('the row that narrows a view', () => {
  it('names what it narrows', () => {
    render(
      <DenseToolbar label="Release filters">
        <ToolbarCount>3 of 9</ToolbarCount>
      </DenseToolbar>,
    );

    expect(screen.getByRole('toolbar', { name: 'Release filters' })).toBeInTheDocument();
  });

  it('wraps instead of pushing its controls out of a narrow panel', () => {
    render(
      <DenseToolbar label="Release filters">
        <ToolbarCount>3 of 9</ToolbarCount>
      </DenseToolbar>,
    );

    expect(screen.getByRole('toolbar').getAttribute('class')).toContain('flex-wrap');
  });

  it('pushes what belongs at the end to the end', () => {
    render(
      <DenseToolbar label="Release filters">
        <ToolbarEnd>
          <span>Install chart</span>
        </ToolbarEnd>
      </DenseToolbar>,
    );

    expect(screen.getByText('Install chart').parentElement?.className).toContain('ml-auto');
  });
});

describe('the filter box', () => {
  it('reports what is typed into it', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<FilterInput label="Filter releases" value="" onChange={onChange} />);

    await user.type(screen.getByRole('searchbox', { name: 'Filter releases' }), 'pod');

    expect(onChange).toHaveBeenCalledTimes(3);
    expect(onChange).toHaveBeenLastCalledWith('d');
  });

  it('says Filter unless the view asks for something else', () => {
    render(
      <>
        <FilterInput label="Filter releases" value="" onChange={vi.fn()} />
        <FilterInput label="Filter subjects" value="" onChange={vi.fn()} placeholder="name" />
      </>,
    );

    expect(screen.getByRole('searchbox', { name: 'Filter releases' })).toHaveAttribute(
      'placeholder',
      'Filter',
    );
    expect(screen.getByRole('searchbox', { name: 'Filter subjects' })).toHaveAttribute(
      'placeholder',
      'name',
    );
  });

  it('takes the width the view has room for', () => {
    render(<FilterInput label="Filter log lines" value="" onChange={vi.fn()} width="w-40" />);

    expect(screen.getByRole('searchbox').className).toContain('w-40');
  });
});
