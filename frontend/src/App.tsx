import { useEffect, useState } from 'react';
import type { ResourceDescriptor, Row } from './lib/types';
import { useResourceFeed } from './lib/feed';
import Sidebar from './components/Sidebar';
import TopBar from './components/TopBar';
import ResourceTable from './components/ResourceTable';
import DetailsDrawer from './components/DetailsDrawer';
import BottomDock from './components/BottomDock';

const MAIN_SUB_ID = 'main';

export default function App() {
  const feed = useResourceFeed();
  const [active, setActive] = useState<ResourceDescriptor | null>(null);
  const [selected, setSelected] = useState<Row | null>(null);

  const { subscribe, unsubscribe } = feed;

  useEffect(() => {
    if (active === null) {
      return;
    }
    subscribe(MAIN_SUB_ID, active, '');
    return () => {
      unsubscribe(MAIN_SUB_ID);
    };
  }, [active, subscribe, unsubscribe]);

  function handleSelectResource(descriptor: ResourceDescriptor) {
    setActive(descriptor);
    setSelected(null);
  }

  function handleSelectRow(row: Row) {
    setSelected(row);
  }

  function handleClose() {
    setSelected(null);
  }

  return (
    <div className="flex h-screen flex-col bg-neutral-950 font-mono text-sm text-neutral-200">
      <TopBar status={feed.status} onReconnect={feed.reconnect} />
      <div className="flex min-h-0 flex-1">
        <Sidebar activeResource={active} onSelect={handleSelectResource} />
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1">
            <ResourceTable
              active={active}
              subId={MAIN_SUB_ID}
              selected={selected}
              onSelect={handleSelectRow}
            />
          </div>
          <BottomDock />
        </div>
        <DetailsDrawer row={selected} onClose={handleClose} />
      </div>
    </div>
  );
}
