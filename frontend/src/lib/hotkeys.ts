import { useEffect, useRef } from 'react';
import { isMac } from './platform';

export const FILTER_INPUT_ID = 'resource-filter';

export interface Hotkey {
  keys: string;
  description: string;
}

export function modLabel(): string {
  if (isMac()) {
    return '⌘';
  }
  return 'Ctrl';
}

export function paletteChordLabel(): string {
  if (isMac()) {
    return '⌘K';
  }
  return 'Ctrl K';
}

export function shortcuts(): Hotkey[] {
  return [
    { keys: paletteChordLabel(), description: 'Open the command palette' },
    { keys: '/', description: 'Jump to the resource filter' },
    { keys: '?', description: 'Show this list' },
    { keys: 'Esc', description: 'Close the palette or dialog, then the inspector' },
    { keys: 's', description: 'Sync or reconcile the selected gitops object' },
    { keys: 'r', description: 'Refresh the selected Argo application' },
    { keys: 'Shift R', description: 'Hard refresh it, or reconcile a Flux object with its source' },
    { keys: 't', description: 'Terminate the operation running on it' },
  ];
}

export function ignorable(event: KeyboardEvent): boolean {
  if (held(event)) {
    return true;
  }
  return typing(event.target);
}

export interface HotkeyActions {
  palette: () => void;
  filter: () => void;
  help: () => void;
  close: () => void;
}

export function focusFilter(): void {
  const input = document.getElementById(FILTER_INPUT_ID);
  if (!(input instanceof HTMLInputElement)) {
    return;
  }
  input.focus();
  input.select();
}

function typing(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target.isContentEditable) {
    return true;
  }
  if (target.tagName === 'INPUT') {
    return true;
  }
  if (target.tagName === 'TEXTAREA') {
    return true;
  }
  return target.tagName === 'SELECT';
}

function held(event: KeyboardEvent): boolean {
  if (event.metaKey) {
    return true;
  }
  if (event.ctrlKey) {
    return true;
  }
  return event.altKey;
}

function paletteChord(event: KeyboardEvent): boolean {
  if (event.key.toLowerCase() !== 'k') {
    return false;
  }
  if (event.metaKey) {
    return true;
  }
  return event.ctrlKey;
}

export function useHotkeys(actions: HotkeyActions): void {
  const latest = useRef(actions);
  latest.current = actions;

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        latest.current.close();
        return;
      }
      if (paletteChord(event)) {
        event.preventDefault();
        latest.current.palette();
        return;
      }
      if (held(event)) {
        return;
      }
      if (typing(event.target)) {
        return;
      }
      if (event.key === '/') {
        event.preventDefault();
        latest.current.filter();
        return;
      }
      if (event.key === '?') {
        event.preventDefault();
        latest.current.help();
      }
    }
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
    };
  }, []);
}
