import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import ManifestDiff from '../../src/components/ManifestDiff';
import { defineEditorTheme } from '../../src/lib/monaco';

vi.mock('../../src/lib/monaco', () => ({
  defineEditorTheme: vi.fn(),
}));

vi.mock('@monaco-editor/react', () => ({
  DiffEditor: ({
    original,
    modified,
    language,
    options,
  }: {
    original: string;
    modified: string;
    language: string;
    options: { readOnly: boolean };
  }) => (
    <div
      data-testid="diff"
      data-original={original}
      data-modified={modified}
      data-language={language}
      data-read-only={String(options.readOnly)}
    />
  ),
}));

describe('ManifestDiff', () => {
  it('hands both manifests to a read-only yaml diff', () => {
    render(<ManifestDiff original={'a: 1\n'} modified={'a: 2\n'} />);

    const diff = screen.getByTestId('diff');
    expect(diff.dataset.original).toBe('a: 1\n');
    expect(diff.dataset.modified).toBe('a: 2\n');
    expect(diff.dataset.language).toBe('yaml');
    expect(diff.dataset.readOnly).toBe('true');
    expect(defineEditorTheme).toHaveBeenCalled();
  });
});
