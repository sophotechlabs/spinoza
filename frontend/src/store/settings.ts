import { create } from 'zustand';
import type { CheckInterval, LogView, NamespaceStart, Settings } from '../lib/settings';
import {
  readNodeShell,
  readSettings,
  readUpdateCheck,
  writeNodeShell,
  writeSettings,
  writeUpdateCheck,
} from '../lib/settings';

interface SettingsState extends Settings {
  nodeShell: boolean;
  updateCheck: boolean;
  setLogView: (logView: LogView) => void;
  setScreenReader: (screenReader: boolean) => void;
  setNamespaceStart: (context: string, namespaceStart: NamespaceStart) => void;
  setChecksInterval: (checksInterval: CheckInterval) => void;
  setNodeShell: (nodeShell: boolean) => Promise<void>;
  setUpdateCheck: (updateCheck: boolean) => Promise<void>;
}

const stored = readSettings();

function saved(state: SettingsState): Settings {
  return {
    logView: state.logView,
    screenReader: state.screenReader,
    namespaceStart: state.namespaceStart,
    namespaceStarts: state.namespaceStarts,
    checksInterval: state.checksInterval,
  };
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  logView: stored.logView,
  screenReader: stored.screenReader,
  namespaceStart: stored.namespaceStart,
  namespaceStarts: stored.namespaceStarts,
  checksInterval: stored.checksInterval,
  nodeShell: readNodeShell(),
  updateCheck: readUpdateCheck(),
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
  setChecksInterval: (checksInterval) => {
    writeSettings({ ...saved(get()), checksInterval });
    set({ checksInterval });
  },
  setNodeShell: async (nodeShell) => {
    await writeNodeShell(nodeShell);
    set({ nodeShell });
  },
  setUpdateCheck: async (updateCheck) => {
    await writeUpdateCheck(updateCheck);
    set({ updateCheck });
  },
}));

export function useLogView(): LogView {
  return useSettingsStore((state) => state.logView);
}

export function useScreenReader(): boolean {
  return useSettingsStore((state) => state.screenReader);
}

export function useChecksInterval(): CheckInterval {
  return useSettingsStore((state) => state.checksInterval);
}

export function useNodeShell(): boolean {
  return useSettingsStore((state) => state.nodeShell);
}

export function useUpdateCheck(): boolean {
  return useSettingsStore((state) => state.updateCheck);
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
