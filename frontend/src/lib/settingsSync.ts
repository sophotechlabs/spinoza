import { isSaving, refresh, startSaving, stopSaving, withoutTrackingChanges } from './persist';
import { useSettingsStore } from '../store/settings';
import { useThemeStore } from '../store/theme';

let activeCatchUp: Promise<void> | null = null;

async function refreshSettings(): Promise<void> {
  if (!(await refresh())) {
    return;
  }
  const was = isSaving();
  stopSaving();
  try {
    withoutTrackingChanges(() => {
      useThemeStore.getState().adoptStored();
      useSettingsStore.getState().adoptStored();
    });
  } finally {
    if (was) {
      startSaving();
    }
  }
}

export function catchUp(): Promise<void> {
  if (activeCatchUp !== null) {
    return activeCatchUp;
  }
  const started = refreshSettings();
  const settled = started.finally(() => {
    if (activeCatchUp === settled) {
      activeCatchUp = null;
    }
  });
  activeCatchUp = settled;
  return settled;
}

export function watchSettings(): () => void {
  const onFocus = (): void => {
    void catchUp();
  };
  const onVisible = (): void => {
    if (document.visibilityState !== 'visible') {
      return;
    }
    void catchUp();
  };
  window.addEventListener('focus', onFocus);
  document.addEventListener('visibilitychange', onVisible);
  return () => {
    window.removeEventListener('focus', onFocus);
    document.removeEventListener('visibilitychange', onVisible);
  };
}
