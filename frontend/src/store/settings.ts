import { create } from 'zustand';
import type {
  CheckInterval,
  LogView,
  NamespaceStart,
  SeverityFloor,
  Settings,
} from '../lib/settings';
import {
  readNodeShell,
  readSettings,
  readCheckRules,
  readUpdateCheck,
  writeNodeShell,
  writeSettings,
  writeCheckRules,
  writeUpdateCheck,
} from '../lib/settings';

interface SettingsState extends Settings {
  nodeShell: boolean;
  updateCheck: boolean;
  checkRules: string;
  setLogView: (logView: LogView) => void;
  setScreenReader: (screenReader: boolean) => void;
  setNamespaceStart: (cluster: string, namespaceStart: NamespaceStart) => void;
  setChecksInterval: (checksInterval: CheckInterval) => void;
  setChecksDisabled: (checksDisabled: string[]) => void;
  setChecksSkipNamespaces: (checksSkipNamespaces: string[]) => void;
  setChecksMinSeverity: (checksMinSeverity: SeverityFloor) => void;
  setChecksWholeCluster: (checksWholeCluster: boolean) => void;
  setChecksEveryKind: (checksEveryKind: boolean) => void;
  setChecksNamespace: (checksNamespace: string) => void;
  setChecksOnlyNew: (checksOnlyNew: boolean) => void;
  setChecksShowMuted: (checksShowMuted: boolean) => void;
  setNodeShell: (nodeShell: boolean) => Promise<void>;
  setUpdateCheck: (updateCheck: boolean) => Promise<void>;
  setCheckRules: (checkRules: string) => Promise<void>;
}

const stored = readSettings();

function saved(state: SettingsState): Settings {
  return {
    logView: state.logView,
    screenReader: state.screenReader,
    namespaceStart: state.namespaceStart,
    namespaceStarts: state.namespaceStarts,
    checksInterval: state.checksInterval,
    checksDisabled: state.checksDisabled,
    checksSkipNamespaces: state.checksSkipNamespaces,
    checksMinSeverity: state.checksMinSeverity,
    checksWholeCluster: state.checksWholeCluster,
    checksEveryKind: state.checksEveryKind,
    checksNamespace: state.checksNamespace,
    checksOnlyNew: state.checksOnlyNew,
    checksShowMuted: state.checksShowMuted,
  };
}

export const useSettingsStore = create<SettingsState>((set, get) => ({
  logView: stored.logView,
  screenReader: stored.screenReader,
  namespaceStart: stored.namespaceStart,
  namespaceStarts: stored.namespaceStarts,
  checksInterval: stored.checksInterval,
  checksDisabled: stored.checksDisabled,
  checksSkipNamespaces: stored.checksSkipNamespaces,
  checksMinSeverity: stored.checksMinSeverity,
  checksWholeCluster: stored.checksWholeCluster,
  checksEveryKind: stored.checksEveryKind,
  checksNamespace: stored.checksNamespace,
  checksOnlyNew: stored.checksOnlyNew,
  checksShowMuted: stored.checksShowMuted,
  nodeShell: readNodeShell(),
  updateCheck: readUpdateCheck(),
  checkRules: readCheckRules(),
  setLogView: (logView) => {
    writeSettings({ ...saved(get()), logView });
    set({ logView });
  },
  setScreenReader: (screenReader) => {
    writeSettings({ ...saved(get()), screenReader });
    set({ screenReader });
  },
  setNamespaceStart: (cluster, namespaceStart) => {
    if (cluster === '') {
      writeSettings({ ...saved(get()), namespaceStart });
      set({ namespaceStart });
      return;
    }
    const namespaceStarts = { ...get().namespaceStarts, [cluster]: namespaceStart };
    writeSettings({ ...saved(get()), namespaceStarts });
    set({ namespaceStarts });
  },
  setChecksInterval: (checksInterval) => {
    writeSettings({ ...saved(get()), checksInterval });
    set({ checksInterval });
  },
  setChecksDisabled: (checksDisabled) => {
    writeSettings({ ...saved(get()), checksDisabled });
    set({ checksDisabled });
  },
  setChecksSkipNamespaces: (checksSkipNamespaces) => {
    writeSettings({ ...saved(get()), checksSkipNamespaces });
    set({ checksSkipNamespaces });
  },
  setChecksMinSeverity: (checksMinSeverity) => {
    writeSettings({ ...saved(get()), checksMinSeverity });
    set({ checksMinSeverity });
  },
  setChecksWholeCluster: (checksWholeCluster) => {
    writeSettings({ ...saved(get()), checksWholeCluster });
    set({ checksWholeCluster });
  },
  setChecksEveryKind: (checksEveryKind) => {
    writeSettings({ ...saved(get()), checksEveryKind });
    set({ checksEveryKind });
  },
  setChecksNamespace: (checksNamespace) => {
    writeSettings({ ...saved(get()), checksNamespace });
    set({ checksNamespace });
  },
  setChecksOnlyNew: (checksOnlyNew) => {
    writeSettings({ ...saved(get()), checksOnlyNew });
    set({ checksOnlyNew });
  },
  setChecksShowMuted: (checksShowMuted) => {
    writeSettings({ ...saved(get()), checksShowMuted });
    set({ checksShowMuted });
  },
  setNodeShell: async (nodeShell) => {
    await writeNodeShell(nodeShell);
    set({ nodeShell });
  },
  setUpdateCheck: async (updateCheck) => {
    await writeUpdateCheck(updateCheck);
    set({ updateCheck });
  },
  setCheckRules: async (checkRules) => {
    await writeCheckRules(checkRules);
    set({ checkRules });
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

export interface ChecksFilter {
  disabled: string[];
  skipNamespaces: string[];
  namespace: string;
  minSeverity: SeverityFloor;
  wholeCluster: boolean;
  everyKind: boolean;
  onlyNew: boolean;
  showMuted: boolean;
}

export function useChecksFilter(): ChecksFilter {
  const disabled = useSettingsStore((state) => state.checksDisabled);
  const skipNamespaces = useSettingsStore((state) => state.checksSkipNamespaces);
  const namespace = useSettingsStore((state) => state.checksNamespace);
  const minSeverity = useSettingsStore((state) => state.checksMinSeverity);
  const wholeCluster = useSettingsStore((state) => state.checksWholeCluster);
  const everyKind = useSettingsStore((state) => state.checksEveryKind);
  const onlyNew = useSettingsStore((state) => state.checksOnlyNew);
  const showMuted = useSettingsStore((state) => state.checksShowMuted);
  return {
    disabled,
    skipNamespaces,
    namespace,
    minSeverity,
    wholeCluster,
    everyKind,
    onlyNew,
    showMuted,
  };
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

export function namespaceStart(cluster: string, context = ''): NamespaceStart {
  const state = useSettingsStore.getState();
  const held = answerFor(state.namespaceStarts, cluster, context);
  if (held !== undefined) {
    return held;
  }
  return state.namespaceStart;
}

// The answer is kept against the cluster now. One written before that, against
// a context name, still counts until the next answer replaces it.
function answerFor(
  starts: Partial<Record<string, NamespaceStart>>,
  cluster: string,
  context: string,
): NamespaceStart | undefined {
  const own = starts[cluster];
  if (own !== undefined) {
    return own;
  }
  if (context === '') {
    return undefined;
  }
  return starts[context];
}

export function namespaceAnswered(cluster: string, context = ''): boolean {
  const starts = useSettingsStore.getState().namespaceStarts;
  return answerFor(starts, cluster, context) !== undefined;
}
