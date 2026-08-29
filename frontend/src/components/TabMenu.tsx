import { useState } from 'react';
import { colorNames, colorVar } from '../lib/clusterColor';
import { closeCluster, clusterFailure, openCluster, recolorCluster } from '../lib/clusters';
import { renameCluster, reopenCluster } from '../lib/clusters';
import type { Tab } from '../store/clusters';
import { nameOf } from '../store/clusters';
import { forgetTab } from '../lib/tabs';
import { useReachable } from '../store/clusterHealth';
import { notifyError } from '../store/toasts';
import { CONTROL } from '../lib/controls';

interface TabMenuProps {
  tab: Tab;
  onDone: () => void;
}

const FIELD = 'w-full rounded border border-edge-strong bg-surface px-2 py-1 text-fg';

export default function TabMenu({ tab, onDone }: TabMenuProps) {
  const answering = useReachable(tab.id);
  const [label, setLabel] = useState(tab.label);
  const [grouping, setGrouping] = useState(tab.grouping);
  const [busy, setBusy] = useState(false);

  async function ask(what: string, run: () => Promise<unknown>) {
    setBusy(true);
    try {
      await run();
    } catch (err: unknown) {
      notifyError(`${what} ${nameOf(tab)}: ${clusterFailure(err, 'the request failed')}`);
    } finally {
      setBusy(false);
    }
  }

  async function reconnect() {
    onDone();
    await ask('Reconnecting to', async () => {
      await closeCluster(tab.id);
      forgetTab(tab.id);
      await openCluster(tab.kubeconfig, tab.context);
    });
  }

  return (
    <div
      role="group"
      aria-label={`Settings for ${nameOf(tab)}`}
      onPointerDown={(event) => {
        event.stopPropagation();
      }}
      className="absolute top-full left-0 z-30 mt-1 flex w-64 flex-col gap-2 rounded border border-edge-strong bg-surface-raised p-2 shadow"
    >
      <div className="flex gap-1">
        {colorNames().map((color) => (
          <button
            key={color}
            type="button"
            aria-label={`Colour ${String(color)}`}
            aria-current={color === tab.color}
            disabled={busy}
            onClick={() => void ask('Recolouring', () => recolorCluster(tab.id, color))}
            style={{ backgroundColor: colorVar(color) }}
            className="h-4 w-4 rounded-sm border border-edge-strong aria-[current=true]:ring-1 aria-[current=true]:ring-fg"
          />
        ))}
      </div>

      <label htmlFor="tab-label" className="text-fg-soft">
        Name
      </label>
      <input
        id="tab-label"
        type="text"
        value={label}
        placeholder={tab.context}
        spellCheck={false}
        onChange={(event) => {
          setLabel(event.target.value);
        }}
        className={FIELD}
      />

      <label htmlFor="tab-group" className="text-fg-soft">
        Group
      </label>
      <input
        id="tab-group"
        type="text"
        value={grouping}
        placeholder="none"
        spellCheck={false}
        onChange={(event) => {
          setGrouping(event.target.value);
        }}
        className={FIELD}
      />

      <label className="flex items-center gap-2 text-fg-soft">
        <input
          type="checkbox"
          checked={tab.reopen}
          disabled={busy}
          onChange={(event) =>
            void ask('Remembering', () => reopenCluster(tab.id, event.target.checked))
          }
        />
        Open this cluster again next time
      </label>

      <div className="flex items-center gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={() =>
            void ask('Renaming', async () => {
              await renameCluster(tab.id, label, grouping);
              onDone();
            })
          }
          className={`${CONTROL} border-edge-strong text-fg hover:bg-surface-active disabled:opacity-50`}
        >
          Save
        </button>
        {!answering && (
          <button
            type="button"
            disabled={busy}
            onClick={() => void reconnect()}
            className={`${CONTROL} ml-auto border-warn-line-strong text-warn-strong hover:bg-warn-tint disabled:opacity-50`}
          >
            Reconnect
          </button>
        )}
      </div>
    </div>
  );
}
