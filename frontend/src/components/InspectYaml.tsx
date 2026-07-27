import { useEffect, useState } from 'react';
import type { ObjectDetail, ObjectRef } from '../lib/types';
import { applyObject, deleteObject } from '../lib/object';
import { fetchSchema, gvkOf, registerSchema, schemaPath } from '../lib/schema';
import YamlEditor from './YamlEditor';

interface InspectYamlProps {
  target: ObjectRef;
  detail: ObjectDetail;
  onApplied: () => void;
  onDeleted: () => void;
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

export default function InspectYaml({ target, detail, onApplied, onDeleted }: InspectYamlProps) {
  const yaml = detail.yaml;
  const path = schemaPath(gvkOf(detail));
  const [draft, setDraft] = useState(yaml);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    setDraft(yaml);
    setError(null);
    setNotice(null);
    setConfirming(false);
  }, [yaml]);

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const schema = await fetchSchema(gvkOf(detail));
        if (mounted) {
          registerSchema(path, schema);
        }
      } catch {
        return;
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [path, detail]);

  const dirty = draft !== yaml;

  async function handleApply() {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await applyObject(target, draft);
      setNotice('Applied.');
      onApplied();
    } catch (err: unknown) {
      setError(errorMessage(err, 'apply failed'));
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete() {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await deleteObject(target);
      onDeleted();
    } catch (err: unknown) {
      setError(errorMessage(err, 'delete failed'));
    } finally {
      setBusy(false);
      setConfirming(false);
    }
  }

  function handleRevert() {
    setDraft(yaml);
    setError(null);
    setNotice(null);
  }

  function askDelete() {
    setConfirming(true);
  }

  function cancelDelete() {
    setConfirming(false);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1">
        <YamlEditor value={draft} path={path} readOnly={busy} onChange={setDraft} />
      </div>
      {error !== null && (
        <p className="border-t border-neutral-800 bg-red-950/40 px-3 py-1.5 text-xs break-words text-red-300">
          {error}
        </p>
      )}
      {notice !== null && (
        <p className="border-t border-neutral-800 px-3 py-1.5 text-xs text-green-400">{notice}</p>
      )}
      <div className="flex items-center gap-2 border-t border-neutral-800 px-3 py-2 text-xs">
        <button
          type="button"
          onClick={() => void handleApply()}
          disabled={busy || !dirty}
          className="rounded border border-neutral-700 px-2 py-1 text-neutral-200 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-600"
        >
          Apply
        </button>
        <button
          type="button"
          onClick={handleRevert}
          disabled={busy || !dirty}
          className="rounded border border-neutral-800 px-2 py-1 text-neutral-400 hover:bg-neutral-800 disabled:cursor-not-allowed disabled:text-neutral-700"
        >
          Revert
        </button>
        {dirty && <span className="text-neutral-600">unsaved changes</span>}
        {!confirming && (
          <button
            type="button"
            onClick={askDelete}
            disabled={busy}
            className="ml-auto rounded border border-red-900 px-2 py-1 text-red-400 hover:bg-red-950 disabled:cursor-not-allowed disabled:text-neutral-700"
          >
            Delete
          </button>
        )}
        {confirming && (
          <div className="ml-auto flex items-center gap-2">
            <span className="text-neutral-400">Delete {target.name}?</span>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={busy}
              className="rounded border border-red-800 bg-red-950 px-2 py-1 text-red-300 hover:bg-red-900 disabled:cursor-not-allowed"
            >
              Confirm
            </button>
            <button
              type="button"
              onClick={cancelDelete}
              disabled={busy}
              className="rounded border border-neutral-800 px-2 py-1 text-neutral-400 hover:bg-neutral-800 disabled:cursor-not-allowed"
            >
              Cancel
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
