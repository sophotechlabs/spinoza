import { useEffect } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { defineEditorTheme } from '../lib/monaco';
import { editorTheme } from '../lib/themeColors';
import { useResolvedTheme } from '../store/theme';

interface ManifestDiffProps {
  original: string;
  modified: string;
}

export default function ManifestDiff({ original, modified }: ManifestDiffProps) {
  const spec = editorTheme(useResolvedTheme());

  useEffect(() => {
    defineEditorTheme(spec);
  }, [spec.name, spec.base, spec.background, spec.foreground, spec]);

  return (
    <DiffEditor
      language="yaml"
      theme={spec.name}
      original={original}
      modified={modified}
      loading={<div className="p-3 text-xs text-fg-muted">Loading the diff…</div>}
      options={{
        readOnly: true,
        renderSideBySide: false,
        minimap: { enabled: false },
        fontSize: 12,
        scrollBeyondLastLine: false,
        automaticLayout: true,
        links: false,
      }}
    />
  );
}
