import { create } from 'zustand';
import type { ObjectRef } from '../lib/types';

export const MAX_TOASTS = 4;

export const MAX_HISTORY = 200;

export type ToastTone = 'ok' | 'warn' | 'error';

export interface ToastAction {
  label: string;
  run: () => void;
}

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
  action?: ToastAction;
}

export interface Notification extends Toast {
  at: string;
  ref?: ObjectRef;
}

interface ToastsState {
  toasts: Toast[];
  history: Notification[];
  push: (tone: ToastTone, message: string, ref?: ObjectRef) => void;
  ask: (message: string, action: ToastAction) => void;
  dismiss: (id: number) => void;
  clear: () => void;
  clearHistory: () => void;
}

let seq = 0;

function trim(toasts: Toast[]): Toast[] {
  if (toasts.length <= MAX_TOASTS) {
    return toasts;
  }
  return toasts.slice(toasts.length - MAX_TOASTS);
}

function cap(history: Notification[]): Notification[] {
  if (history.length <= MAX_HISTORY) {
    return history;
  }
  return history.slice(history.length - MAX_HISTORY);
}

export const useToastsStore = create<ToastsState>((set) => ({
  toasts: [],
  history: [],
  push: (tone, message, ref) => {
    seq += 1;
    const toast: Toast = { id: seq, tone, message };
    const note: Notification = { ...toast, at: new Date().toISOString() };
    if (ref !== undefined) {
      note.ref = ref;
    }
    set((state) => ({
      toasts: trim([...state.toasts, toast]),
      history: cap([...state.history, note]),
    }));
  },
  ask: (message, action) => {
    seq += 1;
    const toast: Toast = { id: seq, tone: 'ok', message, action };
    const note: Notification = { ...toast, at: new Date().toISOString() };
    set((state) => ({
      toasts: trim([...state.toasts, toast]),
      history: cap([...state.history, note]),
    }));
  },
  dismiss: (id) => {
    set((state) => ({ toasts: state.toasts.filter((toast) => toast.id !== id) }));
  },
  clear: () => {
    set({ toasts: [], history: [] });
  },
  clearHistory: () => {
    set({ history: [] });
  },
}));

export function notifyOk(message: string, ref?: ObjectRef): void {
  useToastsStore.getState().push('ok', message, ref);
}

export function notifyWarn(message: string, ref?: ObjectRef): void {
  useToastsStore.getState().push('warn', message, ref);
}

export function notifyError(message: string, ref?: ObjectRef): void {
  useToastsStore.getState().push('error', message, ref);
}

export function askToast(message: string, action: ToastAction): void {
  useToastsStore.getState().ask(message, action);
}

export function clearHistory(): void {
  useToastsStore.getState().clearHistory();
}
