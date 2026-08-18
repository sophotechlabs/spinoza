import { useEffect, useRef, useState } from 'react';
import { SYSTEM } from '../lib/theme';
import { validateTheme } from '../lib/customThemes';
import { LOG_VIEWS, NAMESPACE_STARTS } from '../lib/settings';
import type { LogView, NamespaceStart } from '../lib/settings';
import { useResolvedTheme, useThemePreference, useThemeStore, useThemes } from '../store/theme';
import {
  useLogView,
  useNamespaceStart,
  useScreenReader,
  useSettingsStore,
} from '../store/settings';
import { usePanelsStore } from '../store/panels';
import { useContextList } from '../store/contexts';
import { useNamespaceStore } from '../store/namespace';
import { shortcuts } from '../lib/hotkeys';
import { copyText } from '../lib/clipboard';
import { FRONTEND_VERSION, fetchBackendVersion } from '../lib/version';

const SECTIONS = [
  'Appearance',
  'Cluster',
  'Logs',
  'Terminal',
  'Panels',
  'Keyboard',
  'About',
] as const;

export type Section = (typeof SECTIONS)[number];

interface SettingsDialogProps {
  open: boolean;
  section?: Section;
  onClose: () => void;
}

function versionLabel(version: string): string {
  if (version === '') {
    return '-';
  }
  return version;
}

function startLabel(start: NamespaceStart): string {
  if (start === 'default') {
    return 'the default namespace';
  }
  return 'every namespace';
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

function openOnHint(cluster: string): string {
  if (cluster === '') {
    return 'The namespace a cluster opens on. The picker in the top bar still switches it for this session.';
  }
  return `The namespace ${cluster} opens on. Each cluster keeps its own answer; the picker in the top bar still switches it for this session.`;
}

export default function SettingsDialog({
  open,
  section: wanted = 'Appearance',
  onClose,
}: SettingsDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);
  const [section, setSection] = useState<Section>(wanted);
  const [lastWanted, setLastWanted] = useState(wanted);
  if (wanted !== lastWanted) {
    setLastWanted(wanted);
    setSection(wanted);
  }
  const preference = useThemePreference();
  const setPreference = useThemeStore((state) => state.setPreference);
  const logView = useLogView();
  const setLogView = useSettingsStore((state) => state.setLogView);
  const screenReader = useScreenReader();
  const setScreenReader = useSettingsStore((state) => state.setScreenReader);
  const cluster = useContextList().current.name;
  const start = useNamespaceStart(cluster);
  const setStart = useSettingsStore((state) => state.setNamespaceStart);
  const openOn = useNamespaceStore((state) => state.reset);
  const resetPanels = usePanelsStore((state) => state.reset);
  const themes = useThemes();
  const sortedThemes = [...themes].sort((a, b) => a.name.localeCompare(b.name));
  const custom = useThemeStore((state) => state.custom);
  const addTheme = useThemeStore((state) => state.addTheme);
  const removeTheme = useThemeStore((state) => state.removeTheme);
  const resolved = useResolvedTheme();
  const [draft, setDraft] = useState('');
  const [backend, setBackend] = useState('');
  const [report, setReport] = useState<{ errors: string[]; warnings: string[] }>({
    errors: [],
    warnings: [],
  });

  function importText(text: string) {
    let parsed: unknown = null;
    try {
      parsed = JSON.parse(text);
    } catch {
      setReport({ errors: ['that is not valid JSON'], warnings: [] });
      return;
    }
    const checked = validateTheme(parsed);
    setReport({ errors: checked.errors, warnings: checked.warnings });
    if (checked.theme === null) {
      return;
    }
    addTheme(checked.theme);
    setPreference(checked.theme.id);
    setDraft('');
  }

  function handleImport() {
    importText(draft);
  }

  function handleFile(event: React.ChangeEvent<HTMLInputElement>) {
    const input = event.target;
    const file = input.files?.[0];
    input.value = '';
    if (file === undefined) {
      return;
    }
    void file.text().then((text) => {
      setDraft(text);
      importText(text);
    });
  }

  function chooseFile() {
    fileRef.current?.click();
  }

  function handleExport() {
    void copyText('the current theme', JSON.stringify(resolved, null, 2));
  }

  useEffect(() => {
    if (!open) {
      return;
    }
    let live = true;
    fetchBackendVersion()
      .then((found) => {
        if (live) {
          setBackend(found);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [open]);

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
      <div className="flex h-[24rem] text-xs">
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
        <div className="min-w-0 flex-1 overflow-y-auto p-3">
          {section === 'Appearance' && (
            <>
              <Row label="Theme" hint="Follow the system, or pick one and keep it.">
                <select
                  aria-label="Theme preference"
                  value={preference}
                  onChange={(event) => {
                    setPreference(event.target.value);
                  }}
                  className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
                >
                  {sortedThemes.map((theme) => (
                    <option key={theme.id} value={theme.id}>
                      {theme.name}
                    </option>
                  ))}
                  <option value={SYSTEM}>System</option>
                </select>
              </Row>
              <Row label="Your themes" hint="Imported themes live here until you remove them.">
                <button
                  type="button"
                  onClick={handleExport}
                  className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
                >
                  Copy current as JSON
                </button>
              </Row>
              {custom.length > 0 && (
                <ul className="mt-2 space-y-1">
                  {custom.map((theme) => (
                    <li key={theme.id} className="flex items-center gap-2">
                      <span className="min-w-0 flex-1 truncate text-fg-soft">
                        {theme.name} <span className="text-fg-muted">({theme.base})</span>
                      </span>
                      <button
                        type="button"
                        aria-label={`Remove ${theme.name}`}
                        onClick={() => {
                          removeTheme(theme.id);
                        }}
                        className="rounded border border-error-line px-1.5 py-0.5 text-error hover:bg-error-tint"
                      >
                        Remove
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              <div className="mt-3">
                <div className="flex items-center justify-between gap-4">
                  <label htmlFor="theme-import" className="text-fg">
                    Import a theme
                  </label>
                  <button
                    type="button"
                    onClick={chooseFile}
                    className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
                  >
                    Choose file
                  </button>
                  <input
                    ref={fileRef}
                    type="file"
                    accept="application/json,.json"
                    aria-label="Theme file"
                    onChange={handleFile}
                    className="hidden"
                  />
                </div>
                <textarea
                  id="theme-import"
                  value={draft}
                  spellCheck={false}
                  onChange={(event) => {
                    setDraft(event.target.value);
                  }}
                  placeholder='{"id":"solarized","name":"Solarized","base":"light","tokens":{"surface":"#fdf6e3"}}'
                  className="mt-1 h-24 w-full rounded border border-edge-strong bg-surface-raised p-2 font-mono text-[11px] text-fg"
                />
                <div className="mt-1 flex items-center gap-2">
                  <button
                    type="button"
                    onClick={handleImport}
                    className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active"
                  >
                    Import
                  </button>
                  {report.errors.map((message) => (
                    <span key={message} className="text-error">
                      {message}
                    </span>
                  ))}
                  {report.warnings.map((message) => (
                    <span key={message} className="text-warn">
                      {message}
                    </span>
                  ))}
                </div>
              </div>
            </>
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
          {section === 'About' && (
            <>
              <Row label="Spinoza" hint="The interface bundled with this binary.">
                <span className="font-mono text-fg-soft">{FRONTEND_VERSION}</span>
              </Row>
              <Row label="Backend" hint="Reported by the server this window is talking to.">
                <span className="font-mono text-fg-soft">{versionLabel(backend)}</span>
              </Row>
            </>
          )}
          {section === 'Keyboard' && (
            <table className="w-full text-left">
              <caption className="sr-only">Keyboard shortcuts</caption>
              <tbody>
                {shortcuts().map((hotkey) => (
                  <tr key={hotkey.keys} className="border-b border-edge last:border-b-0">
                    <th scope="row" className="w-24 py-2 pr-4 align-top font-normal text-fg-muted">
                      <kbd className="inline-block rounded border border-edge-strong px-1.5 py-0.5 whitespace-nowrap text-fg">
                        {hotkey.keys}
                      </kbd>
                    </th>
                    <td className="py-2 align-top text-fg">{hotkey.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          {section === 'Terminal' && (
            <Row
              label="Screen reader mode"
              hint="Let a screen reader read the terminal. Slower to draw."
            >
              <input
                type="checkbox"
                aria-label="Screen reader mode"
                checked={screenReader}
                onChange={(event) => {
                  setScreenReader(event.target.checked);
                }}
              />
            </Row>
          )}
          {section === 'Cluster' && (
            <Row label="Open on" hint={openOnHint(cluster)}>
              <select
                aria-label="Namespace to open on"
                value={start}
                onChange={(event) => {
                  setStart(cluster, event.target.value as NamespaceStart);
                  openOn();
                }}
                className="rounded border border-edge-strong bg-surface-raised px-2 py-0.5 text-fg"
              >
                {NAMESPACE_STARTS.map((option) => (
                  <option key={option} value={option}>
                    {startLabel(option)}
                  </option>
                ))}
              </select>
            </Row>
          )}
          {section === 'Panels' && (
            <Row label="Dock layout" hint="Put every panel and dock back where it started.">
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
