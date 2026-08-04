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

function refKey(ref: ObjectRef): string {
  return `${ref.group}/${ref.version}/${ref.resource}/${ref.namespace}/${ref.name}`;
}

function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}

export default function InspectYaml({ target, detail, onApplied, onDeleted }: InspectYamlProps) {
  const yaml = detail.yaml;
  const apiVersion = detail.apiVersion;
  const kind = detail.kind;
  const path = schemaPath(gvkOf(detail));
  const [draft, setDraft] = useState(yaml);
  const [base, setBase] = useState(yaml);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const targetKey = refKey(target);
  const [lastTarget, setLastTarget] = useState(targetKey);
  if (targetKey !== lastTarget) {
    setLastTarget(targetKey);
    setBase(yaml);
    setDraft(yaml);
    setError(null);
    setNotice(null);
    setConfirming(false);
  } else if (yaml !== base && draft === base) {
    setBase(yaml);
    setDraft(yaml);
  }

  useEffect(() => {
    let mounted = true;
    const load = async () => {
      try {
        const schema = await fetchSchema(gvkOf({ apiVersion, kind }));
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
  }, [path, apiVersion, kind]);

  const dirty = draft !== base;
  const stale = yaml !== base;

  async function handleApply() {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await applyObject(target, draft);
      setBase(draft);
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
    setBase(yaml);
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
        <p className="border-t border-edge bg-error-tint/40 px-3 py-1.5 text-xs break-words text-error-strong">
          {error}
        </p>
      )}
      {notice !== null && (
        <p className="border-t border-edge px-3 py-1.5 text-xs text-ok">{notice}</p>
      )}
      <div className="flex items-center gap-2 border-t border-edge px-3 py-2 text-xs">
        <button
          type="button"
          onClick={() => void handleApply()}
          disabled={busy || !dirty}
          className="rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Apply
        </button>
        <button
          type="button"
          onClick={handleRevert}
          disabled={busy || !dirty}
          className="rounded border border-edge px-2 py-1 text-fg-muted hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint"
        >
          Revert
        </button>
        {dirty && !stale && <span className="text-fg-muted">unsaved changes</span>}
        {stale && (
          <span className="text-warn">changed on the server — Revert to load the new version</span>
        )}
        {!confirming && (
          <button
            type="button"
            onClick={askDelete}
            disabled={busy}
            className="ml-auto rounded border border-error-line px-2 py-1 text-error hover:bg-error-tint disabled:cursor-not-allowed disabled:text-fg-faint"
          >
            Delete
          </button>
        )}
        {confirming && (
          <div className="ml-auto flex items-center gap-2">
            <span className="text-fg-muted">Delete {target.name}?</span>
            <button
              type="button"
              onClick={() => void handleDelete()}
              disabled={busy}
              className="rounded border border-error-line-strong bg-error-tint px-2 py-1 text-error-strong hover:bg-error-tint-strong disabled:cursor-not-allowed"
            >
              Confirm
            </button>
            <button
              type="button"
              onClick={cancelDelete}
              disabled={busy}
              className="rounded border border-edge px-2 py-1 text-fg-muted hover:bg-surface-active disabled:cursor-not-allowed"
            >
              Cancel
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
