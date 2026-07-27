import Editor from '@monaco-editor/react';

interface YamlEditorProps {
  value: string;
  readOnly: boolean;
  onChange: (value: string) => void;
}

export default function YamlEditor({ value, readOnly, onChange }: YamlEditorProps) {
  function handleChange(next: string | undefined) {
    if (next === undefined) {
      return;
    }
    onChange(next);
  }

  return (
    <Editor
      language="yaml"
      theme="vs-dark"
      value={value}
      onChange={handleChange}
      loading={<div className="p-3 text-xs text-neutral-600">Loading editor…</div>}
      options={{
        readOnly,
        minimap: { enabled: false },
        fontSize: 12,
        lineNumbers: 'on',
        scrollBeyondLastLine: false,
        tabSize: 2,
        renderWhitespace: 'selection',
        automaticLayout: true,
      }}
    />
  );
}
