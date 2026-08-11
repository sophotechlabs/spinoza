export const SIDEBAR_STATE_KEY = 'spinoza.sidebar.v1';

export const CLUSTER_SECTION = 'Cluster';

export const HELM_SECTION = 'Helm';

export const GITOPS_SECTION = 'GitOps';

const OPEN_BY_DEFAULT = new Set([CLUSTER_SECTION, HELM_SECTION, GITOPS_SECTION]);

export type SidebarSections = Partial<Record<string, boolean>>;

export function sectionOpen(sections: SidebarSections, key: string): boolean {
  const stored = sections[key];
  if (stored !== undefined) {
    return stored;
  }
  return OPEN_BY_DEFAULT.has(key);
}

export function parseSections(raw: string | null): SidebarSections {
  if (raw === null) {
    return {};
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {};
  }
  if (typeof parsed !== 'object') {
    return {};
  }
  if (parsed === null) {
    return {};
  }
  if (Array.isArray(parsed)) {
    return {};
  }
  const sections: SidebarSections = {};
  for (const [key, value] of Object.entries(parsed)) {
    if (typeof value === 'boolean') {
      sections[key] = value;
    }
  }
  return sections;
}

export function readSections(): SidebarSections {
  try {
    return parseSections(window.localStorage.getItem(SIDEBAR_STATE_KEY));
  } catch {
    return {};
  }
}

export function writeSections(sections: SidebarSections): void {
  try {
    window.localStorage.setItem(SIDEBAR_STATE_KEY, JSON.stringify(sections));
  } catch {
    return;
  }
}
