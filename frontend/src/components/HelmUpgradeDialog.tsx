import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import type { HelmChartVersions, HelmRelease } from '../lib/types';
import { fetchHelmVersions, upgradeRelease } from '../lib/helm';
import { notifyError, notifyOk } from '../store/toasts';
import { useProtectedCluster } from '../store/contexts';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';
import Loading from './Loading';
import ClusterBadge from './ClusterBadge';

const YamlEditor = lazy(() => import('./YamlEditor'));
const ManifestDiff = lazy(() => import('./ManifestDiff'));

interface HelmUpgradeDialogProps {
  release: HelmRelease;
  currentValues: string;
  currentManifest: string;
  onClose: () => void;
  onUpgraded: () => void;
}

interface Choice {
  repo: string;
  version: string;
}

function choiceFrom(versions: HelmChartVersions | null, value: string): Choice | null {
  if (versions === null) {
    return null;
  }
  const split = value.indexOf(':');
  const repo = versions.repos.at(Number(value.slice(0, split)));
  if (split < 0 || repo === undefined) {
    return null;
  }
  return { repo: repo.url, version: value.slice(split + 1) };
}

function repoLabel(repo: { name?: string; url: string }): string {
  if (repo.name !== undefined && repo.name !== '') {
    return repo.name;
  }
  return repo.url;
}

const buttonClass =
  'rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';
const primaryClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:border-edge disabled:text-fg-faint';

export default function HelmUpgradeDialog({
  release,
  currentValues,
  currentManifest,
  onClose,
  onUpgraded,
}: HelmUpgradeDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [versions, setVersions] = useState<HelmChartVersions | null>(null);
  const [versionsError, setVersionsError] = useState<string | null>(null);
  const [picked, setPicked] = useState('');
  const [values, setValues] = useState(currentValues);
  const [step, setStep] = useState<'edit' | 'diff'>('edit');
  const [proposed, setProposed] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [typing, setTyping] = useState(false);
  const protectedCluster = useProtectedCluster();

  useEffect(() => {
    const dialog = ref.current;
    if (dialog?.open === false) {
      dialog.showModal();
    }
  }, []);

  useEffect(() => {
    let live = true;
    fetchHelmVersions(release.chart)
      .then((found) => {
        if (live) {
          setVersions(found);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setVersionsError(err instanceof Error ? err.message : 'the version lookup failed');
        }
      });
    return () => {
      live = false;
    };
  }, [release.chart]);

  const choice = choiceFrom(versions, picked);

  async function preview(chosen: Choice) {
    setBusy(true);
    setError(null);
    try {
      const result = await upgradeRelease(argsFor(chosen), true);
      setProposed(result.manifest ?? '');
      setStep('diff');
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'the upgrade request failed';
      setError(message);
      notifyError(message);
    } finally {
      setBusy(false);
    }
  }

  async function run(chosen: Choice, confirm?: string) {
    setBusy(true);
    setError(null);
    setTyping(false);
    try {
      const result = await upgradeRelease(argsFor(chosen), false, confirm);
      notifyOk(result.message);
      onUpgraded();
      onClose();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'the upgrade request failed';
      setError(message);
      notifyError(message);
    } finally {
      setBusy(false);
    }
  }

  function argsFor(chosen: Choice) {
    return {
      namespace: release.namespace,
      name: release.name,
      chart: release.chart,
      repo: chosen.repo,
      version: chosen.version,
      values,
    };
  }

  function askUpgrade(chosen: Choice) {
    if (protectedCluster) {
      setTyping(true);
      return;
    }
    void run(chosen);
  }

  return (
    <dialog
      ref={ref}
      aria-label={`Upgrade ${release.name}`}
      onClose={onClose}
      className="backdrop:bg-black/50 m-auto w-[44rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      {typing && (
        <ConfirmByName
          open
          name={release.name}
          what={`Upgrading ${release.name} to ${release.chart} ${choice?.version ?? ''}.`}
          onConfirm={() => {
            if (choice !== null) {
              void run(choice, release.name);
            }
          }}
          onCancel={() => {
            setTyping(false);
          }}
        />
      )}
      <div className="flex items-center justify-between border-b border-edge px-3 py-2">
        <h2 className="flex items-center gap-2 text-xs font-semibold tracking-wide text-fg-strong uppercase">
          Upgrade {release.name}
          <ClusterBadge />
        </h2>
        <button
          type="button"
          aria-label="Close the upgrade dialog"
          onClick={onClose}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          ✕
        </button>
      </div>
      <div className="p-3 text-xs">
        {versions === null && versionsError === null && <Loading what="versions" />}
        {versionsError !== null && (
          <p role="alert" className="text-error">
            {versionsError}
          </p>
        )}
        {versions !== null && versions.repos.length === 0 && (
          <p role="status" className="text-warn">
            {emptyNote(versions)}
          </p>
        )}
        {versions !== null && versions.repos.length > 0 && (
          <>
            {versions.error !== undefined && (
              <p role="status" className="mb-2 text-warn">
                {versions.error}
              </p>
            )}
            <div className="flex items-center gap-2">
              <label htmlFor="upgrade-version" className="text-fg">
                Version
              </label>
              <select
                id="upgrade-version"
                aria-label="Chart version"
                value={picked}
                disabled={busy || step === 'diff'}
                onChange={(event) => {
                  setPicked(event.target.value);
                }}
                className="rounded border border-edge-strong bg-surface-raised px-1.5 py-0.5 text-fg disabled:text-fg-subtle"
              >
                <option value="" disabled>
                  pick a version
                </option>
                {versions.repos.map((repo, index) => (
                  <optgroup key={repo.url} label={repoLabel(repo)}>
                    {repo.versions.map((version) => (
                      <option key={`${repo.url}:${version}`} value={`${String(index)}:${version}`}>
                        {version}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <span className="text-fg-muted">from {release.chartVersion}</span>
            </div>
            {step === 'edit' && (
              <div className="mt-2 h-64 overflow-hidden rounded border border-edge">
                <Suspense fallback={<Loading what="editor" />}>
                  <YamlEditor
                    value={values}
                    path={`values/${release.namespace}/${release.name}.yaml`}
                    readOnly={busy}
                    onChange={setValues}
                  />
                </Suspense>
              </div>
            )}
            {step === 'diff' && (
              <div className="mt-2 h-80 overflow-hidden rounded border border-edge">
                <Suspense fallback={<Loading what="diff" />}>
                  <ManifestDiff original={currentManifest} modified={proposed} />
                </Suspense>
              </div>
            )}
            <Announce message={error} urgent className="mt-2 text-error" />
            <div className="mt-3 flex items-center justify-end gap-2">
              {step === 'edit' && (
                <>
                  <button type="button" onClick={onClose} className={buttonClass}>
                    Cancel
                  </button>
                  <button
                    type="button"
                    disabled={busy || choice === null}
                    onClick={() => {
                      if (choice !== null) {
                        void preview(choice);
                      }
                    }}
                    className={primaryClass}
                  >
                    Preview
                  </button>
                </>
              )}
              {step === 'diff' && (
                <>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setStep('edit');
                    }}
                    className={buttonClass}
                  >
                    Back
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      if (choice !== null) {
                        askUpgrade(choice);
                      }
                    }}
                    className={primaryClass}
                  >
                    Upgrade to {choice?.version ?? ''}
                  </button>
                </>
              )}
            </div>
          </>
        )}
      </div>
    </dialog>
  );
}

function emptyNote(versions: HelmChartVersions): string {
  if (versions.error !== undefined && versions.error !== '') {
    return versions.error;
  }
  return 'no configured chart repository offers this chart';
}
