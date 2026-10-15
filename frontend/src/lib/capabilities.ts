import type { Capabilities } from './types';
import { request } from './http';
import { parseCapabilities } from './parse';
import { activeCluster } from './cluster';
import { useClusterStore } from '../store/cluster';

interface SharedRead {
  cluster: string;
  epoch: number;
  reading: Promise<Capabilities>;
}

let sharedRead: SharedRead | null = null;

async function readCapabilities(): Promise<Capabilities> {
  const response = await request('/api/capabilities');
  if (!response.ok) {
    throw new Error(`capabilities request failed with status ${response.status}`);
  }
  return parseCapabilities(await response.json());
}

function stillShared(read: SharedRead, cluster: string, epoch: number): boolean {
  if (read.cluster !== cluster) {
    return false;
  }
  return read.epoch === epoch;
}

export function fetchCapabilities(): Promise<Capabilities> {
  const cluster = activeCluster();
  const epoch = useClusterStore.getState().epoch;
  if (sharedRead !== null && stillShared(sharedRead, cluster, epoch)) {
    return sharedRead.reading;
  }
  const held = { cluster, epoch, reading: readCapabilities() };
  sharedRead = held;
  const forget = () => {
    if (sharedRead === held) {
      sharedRead = null;
    }
  };
  held.reading.then(forget, forget);
  return held.reading;
}

export function forgetCapabilities(): void {
  sharedRead = null;
}
