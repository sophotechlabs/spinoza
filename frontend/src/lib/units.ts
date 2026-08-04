export type Millicores = number;
export type Cores = number;
export type Mebibytes = number;
export type Bytes = number;

const MIB = 1024 * 1024;

export function cpuFromMilli(cpu: Millicores): string {
  if (cpu <= 0) {
    return '';
  }
  return `${cpu}m`;
}

export function cpuFromCores(cpu: Cores): string {
  return `${(cpu * 1000).toFixed(0)}m`;
}

export function memFromMi(mem: Mebibytes): string {
  if (mem <= 0) {
    return '';
  }
  if (mem >= 1024) {
    return `${(mem / 1024).toFixed(1)}Gi`;
  }
  return `${mem}Mi`;
}

export function memFromBytes(mem: Bytes): string {
  const mib = mem / MIB;
  if (mib >= 1024) {
    return `${(mib / 1024).toFixed(2)} GiB`;
  }
  return `${mib.toFixed(0)} MiB`;
}
