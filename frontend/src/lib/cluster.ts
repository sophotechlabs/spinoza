let current = '';

export function activeCluster(): string {
  return current;
}

export function setActiveCluster(id: string): void {
  current = id;
}

function alreadyNamed(url: string): boolean {
  return /[?&]cluster=/.test(url);
}

export function onCluster(url: string): string {
  if (current === '') {
    return url;
  }
  if (alreadyNamed(url)) {
    return url;
  }
  let joiner = '?';
  if (url.includes('?')) {
    joiner = '&';
  }
  return `${url}${joiner}cluster=${encodeURIComponent(current)}`;
}
