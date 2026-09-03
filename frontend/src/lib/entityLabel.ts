export interface EntityIdentity {
  name: string;
  kind?: string;
  group?: string;
  version?: string;
  namespace?: string;
  cluster?: string;
}

export interface EntityDetail {
  kind?: boolean;
  apiVersion?: boolean;
  namespace?: boolean;
  cluster?: boolean;
}

function present(value: string | undefined): value is string {
  if (value === undefined) {
    return false;
  }
  return value !== '';
}

export function apiVersionLabel(group: string | undefined, version: string | undefined): string {
  let namedGroup = group;
  if (!present(namedGroup)) {
    namedGroup = 'core';
  }
  if (!present(version)) {
    return namedGroup;
  }
  return `${namedGroup}/${version}`;
}

export function entityDetails(identity: EntityIdentity, detail: EntityDetail): string {
  const parts: string[] = [];
  if (detail.kind === true && present(identity.kind)) {
    parts.push(identity.kind);
  }
  if (detail.namespace === true && present(identity.namespace)) {
    parts.push(identity.namespace);
  }
  if (detail.cluster === true && present(identity.cluster)) {
    parts.push(identity.cluster);
  }
  if (detail.apiVersion === true) {
    parts.push(apiVersionLabel(identity.group, identity.version));
  }
  return parts.join(' · ');
}

export function entityLabelText(identity: EntityIdentity, detail: EntityDetail): string {
  const secondary = entityDetails(identity, detail);
  if (secondary === '') {
    return identity.name;
  }
  return `${identity.name} · ${secondary}`;
}

export function resourceIdentity(key: string): EntityIdentity {
  const parts = key.split('/').filter((part) => part !== '');
  if (parts.length < 2) {
    return { name: key };
  }
  if (parts.length === 2) {
    return { name: parts[1], group: '', version: parts[0] };
  }
  return {
    name: parts[parts.length - 1],
    group: parts[parts.length - 3],
    version: parts[parts.length - 2],
  };
}
