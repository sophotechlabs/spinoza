import { create } from 'zustand';
import type { LogView, NamespaceStart, Settings } from '../lib/settings';
import { readSettings, writeSettings } from '../lib/settings';

interface SettingsState extends Settings {
  setLogView: (logView: LogView) => void;
  setScreenReader: (screenReader: boolean) => void;
  setNamespaceStart: (namespaceStart: NamespaceStart) => void;
  markNamespaceAsked: () => void;
}

const stored = readSettings();

function saved(state: SettingsState): Settings {
  return {
    logView: state.logView,
    screenReader: state.screenReader,
    namespaceStart: state.namespaceStart,
    namespaceAsked: state.namespaceAsked,
  };
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  logView: stored.logView,
  screenReader: stored.screenReader,
  namespaceStart: stored.namespaceStart,
  namespaceAsked: stored.namespaceAsked,
  setLogView: (logView) => {
    writeSettings({ ...saved(get()), logView });
    set({ logView });
  },
  setScreenReader: (screenReader) => {
    writeSettings({ ...saved(get()), screenReader });
    set({ screenReader });
  },
  setNamespaceStart: (namespaceStart) => {
    writeSettings({ ...saved(get()), namespaceStart });
    set({ namespaceStart });
  },
  markNamespaceAsked: () => {
    writeSettings({ ...saved(get()), namespaceAsked: true });
    set({ namespaceAsked: true });
  },
}));

export function useLogView(): LogView {
  return useSettingsStore((state) => state.logView);
}

export function useScreenReader(): boolean {
  return useSettingsStore((state) => state.screenReader);
}

export function useNamespaceStart(): NamespaceStart {
  return useSettingsStore((state) => state.namespaceStart);
}

export function namespaceStart(): NamespaceStart {
  return useSettingsStore.getState().namespaceStart;
}
