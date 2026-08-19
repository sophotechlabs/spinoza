import { useEffect } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { defineEditorTheme } from '../lib/monaco';
import { editorTheme } from '../lib/themeColors';
import { useResolvedTheme } from '../store/theme';

interface YamlDiffProps {
  left: string;
  right: string;
  sideBySide: boolean;
}

export default function YamlDiff({ left, right, sideBySide }: YamlDiffProps) {
  const spec = editorTheme(useResolvedTheme());

  useEffect(() => {
    defineEditorTheme(spec);
  }, [spec.name, spec.base, spec.background, spec.foreground, spec]);

  return (
    <DiffEditor
      language="yaml"
      theme={spec.name}
      original={left}
      modified={right}
      loading={<div className="p-3 text-xs text-fg-muted">Loading the diff</div>}
      options={{
        renderSideBySide: sideBySide,
        readOnly: true,
        minimap: { enabled: false },
        fontSize: 12,
        fontFamily: "'IBM Plex Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
        scrollBeyondLastLine: false,
        automaticLayout: true,
      }}
    />
  );
}
