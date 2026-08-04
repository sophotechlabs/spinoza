import { create } from 'zustand';
import type { LogView } from '../lib/settings';
import { readSettings, writeSettings } from '../lib/settings';

interface SettingsState {
  logView: LogView;
  setLogView: (logView: LogView) => void;
}

export const useSettingsStore = create<SettingsState>((set) => ({
  logView: readSettings().logView,
  setLogView: (logView) => {
    writeSettings({ logView });
    set({ logView });
  },
}));

export function useLogView(): LogView {
  return useSettingsStore((state) => state.logView);
}
