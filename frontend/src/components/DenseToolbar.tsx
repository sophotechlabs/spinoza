import type { ReactNode } from 'react';
import { FIELD, ROW } from '../lib/rows';

interface DenseToolbarProps {
  label: string;
  children: ReactNode;
}

interface FilterInputProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  width?: string;
  id?: string;
}

export function FilterInput({
  label,
  value,
  onChange,
  placeholder = 'Filter',
  width = 'w-56',
  id,
}: FilterInputProps) {
  return (
    <input
      id={id}
      type="search"
      aria-label={label}
      placeholder={placeholder}
      value={value}
      onChange={(event) => {
        onChange(event.target.value);
      }}
      className={`${FIELD} ${width}`}
    />
  );
}

export function ToolbarCount({ children }: { children: ReactNode }) {
  return <span className="text-fg-muted">{children}</span>;
}

export function ToolbarEnd({ children }: { children: ReactNode }) {
  return <span className="ml-auto flex items-center gap-2">{children}</span>;
}

export default function DenseToolbar({ label, children }: DenseToolbarProps) {
  return (
    <div role="toolbar" aria-label={label} className={ROW}>
      {children}
    </div>
  );
}
