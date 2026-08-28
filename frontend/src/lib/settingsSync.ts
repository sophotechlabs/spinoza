import { isSaving, refresh, startSaving, stopSaving } from './persist';
import { useThemeStore } from '../store/theme';

// The settings live on the server, so a second window holds its own copy from
// the moment it loaded. Coming back to a window is the moment to catch up.
//
// Only the theme is adopted. Everything else reads its slice once at mount and
// would need its own reconciliation.
export async function catchUp(): Promise<void> {
  if (!(await refresh())) {
    return;
  }
  // Painting a theme stores the palette it resolved to, so adopting one dirties
  // the settings and would send this window's whole copy back over whatever
  // else has moved. What was adopted is already saved.
  const was = isSaving();
  stopSaving();
  try {
    useThemeStore.getState().adoptStored();
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
