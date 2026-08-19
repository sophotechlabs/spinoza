import { useEffect, useState } from 'react';
import { CONTROL } from '../lib/controls';
import { fetchView, inDesktopWindow, moveToBrowser, moveToDesktop } from '../lib/view';
import { notifyError, notifyWarn } from '../store/toasts';

interface ViewSwitchProps {
  onLeft: () => void;
}

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the switch did not happen';
}

const NO_WINDOW =
  'Switching between window and browser is only available when Spinoza starts as the desktop app';

export default function ViewSwitch({ onLeft }: ViewSwitchProps) {
  const [window_, setWindow] = useState(false);
  const [asked, setAsked] = useState(false);
  const [busy, setBusy] = useState(false);
  const desktop = inDesktopWindow();

  useEffect(() => {
    let live = true;
    fetchView()
      .then((view) => {
        if (live) {
          setWindow(view.window);
          setAsked(true);
        }
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  async function toBrowser() {
    setBusy(true);
    try {
      const moved = await moveToBrowser();
      if (!moved.switched) {
        notifyWarn(moved.reason ?? 'the browser did not open');
      }
    } catch (err: unknown) {
      notifyError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  async function toDesktop() {
    setBusy(true);
    try {
      await moveToDesktop();
      onLeft();
    } catch (err: unknown) {
      notifyError(reason(err));
    } finally {
      setBusy(false);
    }
  }

  if (!asked) {
    return null;
  }

  if (!window_) {
    return (
      <span title={NO_WINDOW}>
        <button
          type="button"
          disabled
          className={`${CONTROL} border-edge text-fg-subtle`}
          aria-describedby="view-switch-why"
        >
          Desktop
        </button>
        <span id="view-switch-why" className="sr-only">
          {NO_WINDOW}
        </span>
      </span>
    );
  }

  if (desktop) {
    return (
      <button
        type="button"
        disabled={busy}
        title="Open in the browser, hide this window"
        onClick={() => void toBrowser()}
        className={`${CONTROL} border-edge-strong text-fg-soft hover:bg-surface-active disabled:text-fg-subtle`}
      >
        Browser
      </button>
    );
  }

  return (
    <button
      type="button"
      disabled={busy}
      title="Back to the desktop window"
      onClick={() => void toDesktop()}
      className={`${CONTROL} border-edge-strong text-fg-soft hover:bg-surface-active disabled:text-fg-subtle`}
    >
      Desktop
    </button>
  );
}
