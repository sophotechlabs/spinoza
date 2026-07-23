import { useEffect, useState } from 'react';
import type { PodRow, ServerMsg } from './lib/types';

function wsURL(): string {
  let proto = 'ws';
  if (location.protocol === 'https:') {
    proto = 'wss';
  }
  return `${proto}://${location.host}/ws`;
}

export default function App() {
  const [rows, setRows] = useState<Map<string, PodRow>>(new Map());
  const [status, setStatus] = useState('connecting');

  useEffect(() => {
    const ws = new WebSocket(wsURL());
    ws.onopen = () => setStatus('connected');
    ws.onclose = () => setStatus('disconnected');
    ws.onerror = () => setStatus('error');
    ws.onmessage = (e) => {
      const m: ServerMsg = JSON.parse(e.data);
      setRows((prev) => {
        const next = new Map(prev);
        if (m.type === 'snapshot') {
          next.clear();
          for (const it of m.items) {
            next.set(it.uid, it);
          }
        } else if (m.type === 'added' || m.type === 'modified') {
          next.set(m.item.uid, m.item);
        } else if (m.type === 'deleted') {
          next.delete(m.uid);
        }
        return next;
      });
    };
    return () => ws.close();
  }, []);

  const list = [...rows.values()].sort((a, b) => a.name.localeCompare(b.name));

  return (
    <div className="min-h-screen bg-neutral-950 text-neutral-200 p-6 font-mono text-sm">
      <h1 className="text-lg mb-1">Spinoza — Phase 0 skeleton</h1>
      <p className="text-neutral-500 mb-4">
        ws: {status} · pods: {list.length}
      </p>
      <table className="w-full text-left">
        <thead className="text-neutral-500 border-b border-neutral-800">
          <tr>
            <th className="py-1 pr-4">Name</th>
            <th className="pr-4">Namespace</th>
            <th className="pr-4">Status</th>
            <th className="pr-4">Ready</th>
            <th>Node</th>
          </tr>
        </thead>
        <tbody>
          {list.map((p) => (
            <tr key={p.uid} className="border-b border-neutral-900">
              <td className="py-1 pr-4">{p.name}</td>
              <td className="pr-4 text-neutral-400">{p.namespace}</td>
              <td className="pr-4">{p.phase}</td>
              <td className="pr-4">{p.ready}</td>
              <td className="text-neutral-400">{p.node}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
