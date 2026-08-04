import Editor from '@monaco-editor/react';
import { useResolvedTheme } from '../store/theme';

interface YamlEditorProps {
  value: string;
  path: string;
  readOnly: boolean;
  onChange: (value: string) => void;
}

export default function YamlEditor({ value, path, readOnly, onChange }: YamlEditorProps) {
  const theme = `spinoza-${useResolvedTheme().base}`;

  function handleChange(next: string | undefined) {
    if (next === undefined) {
      return;
    }
    onChange(next);
  }

  return (
    <Editor
      language="yaml"
      theme={theme}
      path={path}
      value={value}
      onChange={handleChange}
      loading={<div className="p-3 text-xs text-fg-muted">Loading editor…</div>}
      options={{
        readOnly,
        minimap: { enabled: false },
        fontSize: 12,
        lineNumbers: 'on',
        scrollBeyondLastLine: false,
        tabSize: 2,
        renderWhitespace: 'selection',
        automaticLayout: true,
        links: false,
        quickSuggestions: { other: true, comments: false, strings: true },
      }}
    />
  );
}
