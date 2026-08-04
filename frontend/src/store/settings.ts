import { create } from 'zustand';
import type { LogView } from '../lib/settings';
import { readSettings, writeSettings } from '../lib/settings';

interface SettingsState {
  logView: LogView;
  screenReader: boolean;
  setLogView: (logView: LogView) => void;
  setScreenReader: (screenReader: boolean) => void;
}

const stored = readSettings();

export const useSettingsStore = create<SettingsState>((set, get) => ({
  logView: stored.logView,
  screenReader: stored.screenReader,
  setLogView: (logView) => {
    writeSettings({ logView, screenReader: get().screenReader });
    set({ logView });
  },
  setScreenReader: (screenReader) => {
    writeSettings({ logView: get().logView, screenReader });
    set({ screenReader });
  },
}));

export function useLogView(): LogView {
  return useSettingsStore((state) => state.logView);
}

export function useScreenReader(): boolean {
  return useSettingsStore((state) => state.screenReader);
}
