import { describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { useThemeStore } from '../../src/store/theme';

interface DiffStubProps {
  original: string;
  modified: string;
  language: string;
  theme: string;
  options: { renderSideBySide: boolean; readOnly: boolean };
}

const defineEditorTheme = vi.fn<(spec: unknown) => void>();

vi.mock('../../src/lib/monaco', () => ({
  defineEditorTheme: (spec: unknown) => {
    defineEditorTheme(spec);
  },
}));

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: ({ original, modified, language, theme, options }: DiffStubProps) => (
    <div>
      <span data-testid="language">{language}</span>
      <span data-testid="theme">{theme}</span>
      <span data-testid="side-by-side">{String(options.renderSideBySide)}</span>
      <span data-testid="read-only">{String(options.readOnly)}</span>
      <span data-testid="original">{original}</span>
      <span data-testid="modified">{modified}</span>
    </div>
  ),
}));

import YamlDiff from '../../src/components/YamlDiff';

describe('the diff editor', () => {
  it('hands both sides to monaco as yaml', () => {
    render(<YamlDiff left="a: 1\n" right="a: 2\n" sideBySide />);

    expect(screen.getByTestId('language')).toHaveTextContent('yaml');
    expect(screen.getByTestId('original')).toHaveTextContent('a: 1');
    expect(screen.getByTestId('modified')).toHaveTextContent('a: 2');
  });

  it('never lets either side be edited', () => {
    render(<YamlDiff left="a: 1\n" right="a: 2\n" sideBySide />);

    expect(screen.getByTestId('read-only')).toHaveTextContent('true');
  });

  it('lays the two out side by side, or inline when told', () => {
    const view = render(<YamlDiff left="a" right="b" sideBySide />);
    expect(screen.getByTestId('side-by-side')).toHaveTextContent('true');

    view.rerender(<YamlDiff left="a" right="b" sideBySide={false} />);

    expect(screen.getByTestId('side-by-side')).toHaveTextContent('false');
  });

  it('registers the theme the app is wearing', () => {
    act(() => {
      useThemeStore.getState().setPreference('light');
    });

    render(<YamlDiff left="a" right="b" sideBySide />);

    expect(defineEditorTheme).toHaveBeenCalled();
    expect(screen.getByTestId('theme')).toHaveTextContent('spinoza-light');
  });
});
