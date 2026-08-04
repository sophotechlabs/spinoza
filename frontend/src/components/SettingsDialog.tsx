import { useEffect, useRef, useState } from 'react';
import { THEMES } from '../lib/theme';
import type { ThemePreference } from '../lib/theme';
import { LOG_VIEWS } from '../lib/settings';
import type { LogView } from '../lib/settings';
import { useThemePreference, useThemeStore } from '../store/theme';
import { useLogView, useSettingsStore } from '../store/settings';
import { usePanelsStore } from '../store/panels';

const SECTIONS = ['Appearance', 'Logs', 'Panels'] as const;

type Section = (typeof SECTIONS)[number];

interface SettingsDialogProps {
  open: boolean;
  onClose: () => void;
}

function sectionClass(active: boolean): string {
  const base = 'w-full rounded px-2 py-1 text-left';
  if (active) {
    return `${base} bg-surface-active text-fg-strong`;
  }
  return `${base} text-fg-soft hover:bg-surface-raised`;
}

function Row({
  label,
  hint,
  children,
}: {
  label: string;
  hint: string;
  children: React.ReactNode;
}) {
  return (
    <div className="border-b border-edge px-1 py-3 last:border-b-0">
      <div className="flex items-center justify-between gap-4">
        <span className="text-fg">{label}</span>
        {children}
      </div>
      <p className="mt-1 text-[11px] text-fg-muted">{hint}</p>
    </div>
  );
}

export default function SettingsDialog({ open, onClose }: SettingsDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [section, setSection] = useState<Section>('Appearance');
  const preference = useThemePreference();
  const setPreference = useThemeStore((state) => state.setPreference);
  const logView = useLogView();
  const setLogView = useSettingsStore((state) => state.setLogView);
  const resetPanels = usePanelsStore((state) => state.reset);

  useEffect(() => {
    const dialog = ref.current;
    if (open && dialog?.open === false) {
      dialog.showModal();
    }
    if (!open && dialog?.open === true) {
      dialog.close();
    }
  }, [open]);

  return (
    <dialog
      ref={ref}
      aria-label="Settings"
      onClose={onClose}
      className="backdrop:bg-black/50 m-auto w-[34rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      <div className="flex items-center justify-between border-b border-edge px-3 py-2">
        <h2 className="text-xs font-semibold tracking-wide text-fg-strong uppercase">Settings</h2>
        <button
          type="button"
          onClick={onClose}
          className="rounded border border-edge-strong px-2 py-0.5 text-xs text-fg-soft hover:bg-surface-active"
        >
          Close
        </button>
      </div>
      <div className="flex min-h-[16rem] text-xs">
        <nav aria-label="Settings sections" className="w-32 shrink-0 border-r border-edge p-2">
          {SECTIONS.map((name) => (
            <button
              key={name}
              type="button"
              aria-current={section === name}
              onClick={() => {
                setSection(name);
              }}
              className={sectionClass(section === name)}
            >
              {name}
            </button>
          ))}
        </nav>
        <div className="min-w-0 flex-1 p-3">
          {section === 'Appearance' && (
            <Row label="Theme" hint="Follow the system, or pick one and keep it.">
              <select
                aria-label="Theme preference"
                value={preference}
                onChange={(event) => {
                  setPreference(event.target.value as ThemePreference);
                }}
                className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
              >
                {THEMES.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            </Row>
          )}
          {section === 'Logs' && (
            <Row label="Default view" hint="How log lines open before you toggle them.">
              <select
                aria-label="Default log view"
                value={logView}
                onChange={(event) => {
                  setLogView(event.target.value as LogView);
                }}
                className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
              >
                {LOG_VIEWS.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            </Row>
          )}
          {section === 'Panels' && (
            <Row label="Dock layout" hint="Put every panel back where it started.">
              <button
                type="button"
                onClick={resetPanels}
                className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
              >
                Reset
              </button>
            </Row>
          )}
        </div>
      </div>
    </dialog>
  );
}
