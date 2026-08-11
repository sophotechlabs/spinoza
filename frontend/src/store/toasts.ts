import { create } from 'zustand';

export const MAX_TOASTS = 4;

export type ToastTone = 'ok' | 'warn' | 'error';

export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
}

interface ToastsState {
  toasts: Toast[];
  push: (tone: ToastTone, message: string) => void;
  dismiss: (id: number) => void;
  clear: () => void;
}

let seq = 0;

function trim(toasts: Toast[]): Toast[] {
  if (toasts.length <= MAX_TOASTS) {
    return toasts;
  }
  return toasts.slice(toasts.length - MAX_TOASTS);
}

export const useToastsStore = create<ToastsState>((set) => ({
  toasts: [],
  push: (tone, message) => {
    seq += 1;
    const toast: Toast = { id: seq, tone, message };
    set((state) => ({ toasts: trim([...state.toasts, toast]) }));
  },
  dismiss: (id) => {
    set((state) => ({ toasts: state.toasts.filter((toast) => toast.id !== id) }));
  },
  clear: () => {
    set({ toasts: [] });
  },
}));

export function notifyOk(message: string): void {
  useToastsStore.getState().push('ok', message);
}

export function notifyWarn(message: string): void {
  useToastsStore.getState().push('warn', message);
}

export function notifyError(message: string): void {
  useToastsStore.getState().push('error', message);
}
