import { useState } from 'react';
import { request } from '../lib/http';
import { failure, reasonOf } from '../lib/object';
import { notifyError } from '../store/toasts';

async function download(): Promise<void> {
  const response = await request('/api/support');
  if (!response.ok) {
    throw await failure(response, `the support bundle failed with status ${response.status}`);
  }
  const body = await response.blob();
  const url = URL.createObjectURL(body);
  const link = document.createElement('a');
  link.href = url;
  link.download = 'spinoza-support.json';
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export default function SupportBundle() {
  const [busy, setBusy] = useState(false);

  return (
    <button
      type="button"
      disabled={busy}
      onClick={() => {
        setBusy(true);
        download()
          .catch((err: unknown) => {
            notifyError(reasonOf(err, 'that bundle was not written'));
          })
          .finally(() => {
            setBusy(false);
          });
      }}
      className="rounded border border-edge-strong px-2 py-0.5 text-fg hover:bg-surface-active disabled:opacity-50"
    >
      {busy ? 'Writing...' : 'Save a support bundle'}
    </button>
  );
}
