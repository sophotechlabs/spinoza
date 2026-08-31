import { isSaving, refresh, startSaving, stopSaving } from './persist';
import { useSettingsStore } from '../store/settings';
import { useThemeStore } from '../store/theme';

export async function catchUp(): Promise<void> {
  if (!(await refresh())) {
    return;
  }
  const was = isSaving();
  stopSaving();
  try {
    useThemeStore.getState().adoptStored();
    useSettingsStore.getState().adoptStored();
  } finally {
    if (was) {
      startSaving();
    }
  }
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
