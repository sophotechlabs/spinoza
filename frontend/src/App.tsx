import { useState } from 'react';
import type { PodRow } from './lib/types';
import { usePodRows } from './store/pods';
import { usePodsFeed } from './lib/feed';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import PodTable from './components/PodTable';
import DetailsDrawer from './components/DetailsDrawer';
import BottomDock from './components/BottomDock';

export default function App() {
  const feed = usePodsFeed();
  const rows = usePodRows();
  const [selected, setSelected] = useState<PodRow | null>(null);

  function handleSelect(pod: PodRow) {
    setSelected(pod);
  }

  function handleClose() {
    setSelected(null);
  }

  let selectedUid: string | null = null;
  if (selected) {
    selectedUid = selected.uid;
  }

  return (
    <div className="flex h-screen flex-col bg-neutral-950 font-mono text-sm text-neutral-200">
      <TopBar status={feed.status} />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1 overflow-auto">
            <PodTable rows={rows} selectedUid={selectedUid} onSelect={handleSelect} />
          </div>
          <BottomDock />
        </div>
        <DetailsDrawer pod={selected} onClose={handleClose} />
      </div>
    </div>
  );
}
