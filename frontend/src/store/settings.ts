import { create } from 'zustand';
import type { LogView, NamespaceStart, Settings } from '../lib/settings';
import { readSettings, writeSettings } from '../lib/settings';

interface SettingsState extends Settings {
  setLogView: (logView: LogView) => void;
  setScreenReader: (screenReader: boolean) => void;
  setNamespaceStart: (context: string, namespaceStart: NamespaceStart) => void;
}

const stored = readSettings();

function saved(state: SettingsState): Settings {
  return {
    logView: state.logView,
    screenReader: state.screenReader,
    namespaceStart: state.namespaceStart,
    namespaceStarts: state.namespaceStarts,
  };
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  logView: stored.logView,
  screenReader: stored.screenReader,
  namespaceStart: stored.namespaceStart,
  namespaceStarts: stored.namespaceStarts,
  setLogView: (logView) => {
    writeSettings({ ...saved(get()), logView });
    set({ logView });
  },
  setScreenReader: (screenReader) => {
    writeSettings({ ...saved(get()), screenReader });
    set({ screenReader });
  },
  setNamespaceStart: (context, namespaceStart) => {
    if (context === '') {
      writeSettings({ ...saved(get()), namespaceStart });
      set({ namespaceStart });
      return;
    }
    const namespaceStarts = { ...get().namespaceStarts, [context]: namespaceStart };
    writeSettings({ ...saved(get()), namespaceStarts });
    set({ namespaceStarts });
  },
}));

export function useLogView(): LogView {
  return useSettingsStore((state) => state.logView);
}

export function useScreenReader(): boolean {
  return useSettingsStore((state) => state.screenReader);
}

export function useNamespaceStart(context: string): NamespaceStart {
  const held = useSettingsStore((state) => state.namespaceStarts[context]);
  const fallback = useSettingsStore((state) => state.namespaceStart);
  if (held !== undefined) {
    return held;
  }
  return fallback;
}

export function namespaceStart(context: string): NamespaceStart {
  const state = useSettingsStore.getState();
  const held = state.namespaceStarts[context];
  if (held !== undefined) {
    return held;
  }
  return state.namespaceStart;
}

export function namespaceAnswered(context: string): boolean {
  return useSettingsStore.getState().namespaceStarts[context] !== undefined;
}
