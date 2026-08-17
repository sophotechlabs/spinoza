import { readStored, writeStored } from './persist';
export const SIDEBAR_STATE_KEY = 'spinoza.sidebar.v1';

export const GITOPS_SECTION = 'GitOps';

export type SidebarSections = Partial<Record<string, boolean>>;

export function sectionOpen(sections: SidebarSections, key: string): boolean {
  const stored = sections[key];
  if (stored !== undefined) {
    return stored;
  }
  return key === GITOPS_SECTION;
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
  return parseSections(readStored(SIDEBAR_STATE_KEY));
}

export function writeSections(sections: SidebarSections): void {
  writeStored(SIDEBAR_STATE_KEY, JSON.stringify(sections));
}
