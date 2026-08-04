import { describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useThemeStore } from '../../src/store/theme';

interface EditorStubProps {
  value: string;
  language: string;
  theme: string;
  onChange: (value: string | undefined) => void;
  options: { readOnly: boolean };
}

vi.mock('../../src/lib/monaco', () => ({}));

vi.mock('@monaco-editor/react', () => ({
  default: ({ value, language, theme, onChange, options }: EditorStubProps) => (
    <div>
      <span data-testid="language">{language}</span>
      <span data-testid="theme">{theme}</span>
      <span data-testid="read-only">{String(options.readOnly)}</span>
      <textarea
        aria-label="yaml"
        value={value}
        onChange={(event) => {
          onChange(event.target.value);
        }}
      />
      <button
        type="button"
        onClick={() => {
          onChange(undefined);
        }}
      >
        emit-undefined
      </button>
    </div>
  ),
}));

import YamlEditor from '../../src/components/YamlEditor';

describe('YamlEditor', () => {
  it('renders yaml content in a read-write editor', () => {
    render(
      <YamlEditor
        value="kind: Pod"
        path="spinoza/core/v1/Pod.yaml"
        readOnly={false}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId('language')).toHaveTextContent('yaml');
    expect(screen.getByTestId('read-only')).toHaveTextContent('false');
    expect(screen.getByLabelText('yaml')).toHaveValue('kind: Pod');
  });

  it('passes the read-only flag through', () => {
    render(
      <YamlEditor value="" path="spinoza/core/v1/Pod.yaml" readOnly={true} onChange={vi.fn()} />,
    );

    expect(screen.getByTestId('read-only')).toHaveTextContent('true');
  });

  it('reports edits', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <YamlEditor value="" path="spinoza/core/v1/Pod.yaml" readOnly={false} onChange={onChange} />,
    );

    await user.type(screen.getByLabelText('yaml'), 'a');

    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('ignores an undefined value from the editor', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <YamlEditor value="" path="spinoza/core/v1/Pod.yaml" readOnly={false} onChange={onChange} />,
    );

    await user.click(screen.getByRole('button', { name: 'emit-undefined' }));

    expect(onChange).not.toHaveBeenCalled();
  });

  it('follows the app theme so the editor does not sit lighter than the panel', () => {
    render(
      <YamlEditor value="" path="spinoza/core/v1/Pod.yaml" readOnly={false} onChange={vi.fn()} />,
    );

    expect(screen.getByTestId('theme')).toHaveTextContent('spinoza-dark');

    act(() => {
      useThemeStore.getState().setPreference('light');
    });

    expect(screen.getByTestId('theme')).toHaveTextContent('spinoza-light');

    act(() => {
      useThemeStore.getState().setPreference('dark');
    });
  });
});
