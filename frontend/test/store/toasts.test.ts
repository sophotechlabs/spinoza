import { beforeEach, describe, expect, it } from 'vitest';
import {
  MAX_HISTORY,
  MAX_TOASTS,
  clearHistory,
  notifyError,
  notifyOk,
  notifyWarn,
  useToastsStore,
} from '../../src/store/toasts';
import type { ObjectRef } from '../../src/lib/types';

const podRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web-0',
};

function history() {
  return useToastsStore.getState().history;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

describe('toasts', () => {
  it('keeps only the last few on screen', () => {
    for (let index = 0; index < MAX_TOASTS + 3; index += 1) {
      notifyOk(`message ${String(index)}`);
    }

    expect(useToastsStore.getState().toasts).toHaveLength(MAX_TOASTS);
    expect(useToastsStore.getState().toasts[0].message).toBe('message 3');
  });

  it('carries the tone that was asked for', () => {
    notifyOk('done');
    notifyWarn('careful');
    notifyError('broken');

    expect(history().map((note) => note.tone)).toEqual(['ok', 'warn', 'error']);
  });

  it('drops one from the screen without touching the history', () => {
    notifyOk('done');
    const [toast] = useToastsStore.getState().toasts;

    useToastsStore.getState().dismiss(toast.id);

    expect(useToastsStore.getState().toasts).toHaveLength(0);
    expect(history()).toHaveLength(1);
  });
});

describe('an offer toast', () => {
  it('carries its action on screen and into the history', () => {
    const run = () => undefined;

    useToastsStore.getState().ask('Open on default instead?', { label: 'Open on default', run });

    const [toast] = useToastsStore.getState().toasts;
    expect(toast.action?.label).toBe('Open on default');
    expect(history()).toHaveLength(1);
    expect(history()[0].message).toBe('Open on default instead?');
    expect(history()[0].action?.label).toBe('Open on default');
  });
});

describe('the notification history', () => {
  it('keeps everything the toasts dropped', () => {
    for (let index = 0; index < MAX_TOASTS + 3; index += 1) {
      notifyOk(`message ${String(index)}`);
    }

    expect(history()).toHaveLength(MAX_TOASTS + 3);
    expect(history()[0].message).toBe('message 0');
  });

  it('stamps each entry with the time it arrived', () => {
    notifyOk('done');

    expect(Number.isNaN(new Date(history()[0].at).getTime())).toBe(false);
  });

  it('remembers where an entry came from when it was given one', () => {
    notifyOk('Deleted Pod web-0', podRef);
    notifyOk('Switched to p-mk2');

    expect(history()[0].ref).toEqual(podRef);
    expect(history()[1].ref).toBeUndefined();
  });

  it('stops growing once it is full', () => {
    for (let index = 0; index < MAX_HISTORY + 5; index += 1) {
      notifyOk(`message ${String(index)}`);
    }

    expect(history()).toHaveLength(MAX_HISTORY);
    expect(history()[0].message).toBe('message 5');
  });

  it('empties when the cluster changes, leaving what is on screen alone', () => {
    notifyOk('done');

    clearHistory();

    expect(history()).toHaveLength(0);
    expect(useToastsStore.getState().toasts).toHaveLength(1);
  });
});
