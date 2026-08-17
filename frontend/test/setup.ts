import '@testing-library/jest-dom/vitest';

import { beforeEach } from 'vitest';
import { configure } from '@testing-library/react';
import { installMatchMedia } from './helpers';
import { usePanelsStore } from '../src/store/panels';
import { useClusterStore } from '../src/store/cluster';
import { useSessionStore } from '../src/store/session';
import { useContextsStore } from '../src/store/contexts';
import { useForwardsStore } from '../src/store/forwards';
import { hydrate, resetStored } from '../src/lib/persist';

configure({ asyncUtilTimeout: 5000 });

beforeEach(() => {
  window.localStorage.clear();
  delete window.__SPINOZA_SETTINGS__;
  resetStored();
  hydrate();
  window.history.replaceState(null, '', '/');
  usePanelsStore.getState().reset();
  useClusterStore.getState().reset();
  useSessionStore.getState().reset();
  useContextsStore.getState().reset();
  useForwardsStore.getState().setForwards([]);
});

class ResizeObserverStub {
  observe(): void {
    return undefined;
  }

  unobserve(): void {
    return undefined;
  }

  disconnect(): void {
    return undefined;
  }
}

globalThis.ResizeObserver = ResizeObserverStub;

class MemoryStorage implements Storage {
  private entries = new Map<string, string>();

  get length(): number {
    return this.entries.size;
  }

  clear(): void {
    this.entries.clear();
  }

  getItem(key: string): string | null {
    return this.entries.get(key) ?? null;
  }

  key(index: number): string | null {
    return [...this.entries.keys()][index] ?? null;
  }

  removeItem(key: string): void {
    this.entries.delete(key);
  }

  setItem(key: string, value: string): void {
    this.entries.set(key, value);
  }
}

Object.defineProperty(window, 'localStorage', {
  configurable: true,
  writable: true,
  value: new MemoryStorage(),
});

installMatchMedia();

Object.defineProperty(HTMLElement.prototype, 'offsetHeight', {
  configurable: true,
  get() {
    return 600;
  },
});

Object.defineProperty(HTMLElement.prototype, 'offsetWidth', {
  configurable: true,
  get() {
    return 800;
  },
});
