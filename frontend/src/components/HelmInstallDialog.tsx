import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import type { HelmChartHit, HelmChartVersions } from '../lib/types';
import { fetchChartValues, fetchHelmVersions, installRelease, searchCharts } from '../lib/helm';
import { notifyError, notifyOk } from '../store/toasts';
import { useProtectedCluster } from '../store/contexts';
import { useHelmAccess } from '../lib/useHelmAccess';
import { useHelmRefusal } from '../store/helmAccess';
import { useNamespaceStore } from '../store/namespace';
import Announce from './Announce';
import ConfirmByName from './ConfirmByName';
import Loading from './Loading';

const YamlEditor = lazy(() => import('./YamlEditor'));
const ManifestDiff = lazy(() => import('./ManifestDiff'));

interface HelmInstallDialogProps {
  namespace: string;
  onClose: () => void;
  onInstalled: () => void;
}

interface Choice {
  repo: string;
  version: string;
}

const SEARCH_DELAY_MS = 300;

const buttonClass =
  'rounded border border-edge-strong px-2 py-1 text-fg-soft hover:bg-surface-active disabled:cursor-not-allowed disabled:text-fg-faint';
const primaryClass =
  'rounded border border-warn-line px-2 py-1 text-warn hover:bg-warn-tint disabled:cursor-not-allowed disabled:border-edge disabled:text-fg-faint';
const fieldClass =
  'rounded border border-edge-strong bg-surface-raised px-1.5 py-0.5 text-fg placeholder:text-fg-muted disabled:text-fg-subtle';

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

function hitLabel(hit: HelmChartHit): string {
  if (hit.repo === undefined || hit.repo === '') {
    return hit.url;
  }
  return hit.repo;
}

function searchNote(hits: HelmChartHit[], query: string, searching: boolean): string {
  if (searching) {
    return 'searching every configured repository';
  }
  if (query.trim() === '') {
    return 'type part of a chart name';
  }
  if (hits.length === 0) {
    return 'no configured repository offers a chart by that name';
  }
  return `${String(hits.length)} charts`;
}

export default function HelmInstallDialog({
  namespace,
  onClose,
  onInstalled,
}: HelmInstallDialogProps) {
  const ref = useRef<HTMLDialogElement | null>(null);
  const [query, setQuery] = useState('');
  const [hits, setHits] = useState<HelmChartHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [chart, setChart] = useState('');
  const [versions, setVersions] = useState<HelmChartVersions | null>(null);
  const [picked, setPicked] = useState('');
  const [name, setName] = useState('');
  const [target, setTarget] = useState(namespace);
  const [createNamespace, setCreateNamespace] = useState(false);
  const [values, setValues] = useState('');
  const [step, setStep] = useState<'pick' | 'edit' | 'preview'>('pick');
  const [rendered, setRendered] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [typing, setTyping] = useState(false);
  const protectedCluster = useProtectedCluster();
  const known = useNamespaceStore((state) => state.names);
  useHelmAccess(target, '');

  useEffect(() => {
    const dialog = ref.current;
    if (dialog?.open === false) {
      dialog.showModal();
    }
  }, []);

  useEffect(() => {
    if (query.trim() === '') {
      setHits([]);
      setTruncated(false);
      return;
    }
    let live = true;
    setSearching(true);
    const timer = setTimeout(() => {
      searchCharts(query)
        .then((found) => {
          if (!live) {
            return;
          }
          setHits(found.hits);
          setTruncated(found.truncated === true);
          setSearchError(found.error ?? null);
        })
        .catch((err: unknown) => {
          if (live) {
            setSearchError(messageOf(err, 'the chart search failed'));
          }
        })
        .finally(() => {
          if (live) {
            setSearching(false);
          }
        });
    }, SEARCH_DELAY_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [query]);

  useEffect(() => {
    if (chart === '') {
      return;
    }
    let live = true;
    setVersions(null);
    fetchHelmVersions(chart)
      .then((found) => {
        if (!live) {
          return;
        }
        setVersions(found);
        setPicked(firstVersion(found));
      })
      .catch((err: unknown) => {
        if (live) {
          setError(messageOf(err, 'the version lookup failed'));
        }
      });
    return () => {
      live = false;
    };
  }, [chart]);

  const choice = choiceFrom(versions, picked);

  function pickChart(hit: HelmChartHit) {
    setChart(hit.chart);
    setName(hit.chart);
    setStep('edit');
    setError(null);
  }

  async function loadValues(chosen: Choice) {
    setBusy(true);
    setError(null);
    try {
      const found = await fetchChartValues(chart, chosen.repo, chosen.version);
      setValues(found.values);
    } catch (err: unknown) {
      setError(messageOf(err, 'the chart values could not be read'));
    } finally {
      setBusy(false);
    }
  }

  async function preview(chosen: Choice) {
    setBusy(true);
    setError(null);
    try {
      const result = await installRelease(argsFor(chosen), true);
      setRendered(result.manifest ?? '');
      setStep('preview');
    } catch (err: unknown) {
      const message = messageOf(err, 'the install preview failed');
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
      const result = await installRelease(argsFor(chosen), false, confirm);
      notifyOk(result.message);
      onInstalled();
      onClose();
    } catch (err: unknown) {
      const message = messageOf(err, 'the install failed');
      setError(message);
      notifyError(message);
    } finally {
      setBusy(false);
    }
  }

  function argsFor(chosen: Choice) {
    return {
      namespace: target,
      name,
      chart,
      repo: chosen.repo,
      version: chosen.version,
      values,
      createNamespace,
    };
  }

  function askInstall(chosen: Choice) {
    if (protectedCluster) {
      setTyping(true);
      return;
    }
    void run(chosen);
  }

  const ready = choice !== null && name !== '' && target !== '';
  // The preview is a dry run, which writes no release object at all, so it is
  // never the thing a refusal stands in the way of.
  const noInstall = useHelmRefusal(target, '', 'install');

  return (
    <dialog
      ref={ref}
      aria-label="Install a chart"
      onClose={onClose}
      className="backdrop:bg-black/50 m-auto w-[46rem] rounded border border-edge-strong bg-surface p-0 text-fg"
    >
      {typing && (
        <ConfirmByName
          open
          name={name}
          what={`Installing ${chart} ${choice?.version ?? ''} as ${name} in ${target}.`}
          onConfirm={() => {
            if (choice !== null) {
              void run(choice, name);
            }
          }}
          onCancel={() => {
            setTyping(false);
          }}
        />
      )}
      <div className="flex items-center justify-between border-b border-edge px-3 py-2">
        <h2 className="text-xs font-semibold tracking-wide text-fg-strong uppercase">
          Install a chart
        </h2>
        <button
          type="button"
          aria-label="Close the install dialog"
          onClick={onClose}
          className="rounded border border-edge-strong px-1.5 py-0.5 text-fg-soft hover:bg-surface-active"
        >
          ✕
        </button>
      </div>
      <div className="p-3 text-xs">
        {step === 'pick' && (
          <>
            <div className="flex items-center gap-2">
              <label htmlFor="chart-search" className="text-fg">
                Chart
              </label>
              <input
                id="chart-search"
                type="search"
                aria-label="Search charts"
                placeholder="podinfo"
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                }}
                className={`w-64 ${fieldClass}`}
              />
              <span className="text-fg-muted">{searchNote(hits, query, searching)}</span>
            </div>
            {searchError !== null && (
              <p role="status" className="mt-2 text-warn">
                {searchError}
              </p>
            )}
            {truncated && (
              <p role="status" className="mt-2 text-fg-muted">
                Only the first matches are shown; narrow the search to see the rest.
              </p>
            )}
            {hits.length > 0 && (
              <ul className="mt-2 max-h-72 overflow-auto rounded border border-edge">
                {hits.map((hit) => (
                  <li
                    key={`${hit.url}/${hit.chart}`}
                    className="border-b border-edge last:border-b-0"
                  >
                    <button
                      type="button"
                      aria-label={`${hit.chart} ${hit.version} from ${hitLabel(hit)}`}
                      onClick={() => {
                        pickChart(hit);
                      }}
                      className="flex w-full flex-col gap-0.5 px-2 py-1.5 text-left hover:bg-surface-active"
                    >
                      <span className="flex items-baseline gap-2">
                        <span className="text-fg-strong">{hit.chart}</span>
                        <span className="text-fg-muted">{hit.version}</span>
                        <span className="text-fg-subtle">{hitLabel(hit)}</span>
                      </span>
                      {hit.description !== undefined && hit.description !== '' && (
                        <span className="truncate text-fg-muted">{hit.description}</span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
        {step !== 'pick' && (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-fg-strong">{chart}</span>
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setStep('pick');
                }}
                className={buttonClass}
              >
                Change chart
              </button>
              {versions === null && <Loading what="versions" />}
              {versions !== null && (
                <>
                  <label htmlFor="install-version" className="text-fg">
                    Version
                  </label>
                  <select
                    id="install-version"
                    aria-label="Chart version"
                    value={picked}
                    disabled={busy || step === 'preview'}
                    onChange={(event) => {
                      setPicked(event.target.value);
                    }}
                    className={fieldClass}
                  >
                    <option value="" disabled>
                      pick a version
                    </option>
                    {versions.repos.map((repo, index) => (
                      <optgroup key={repo.url} label={repoLabel(repo)}>
                        {repo.versions.map((version) => (
                          <option
                            key={`${repo.url}:${version}`}
                            value={`${String(index)}:${version}`}
                          >
                            {version}
                          </option>
                        ))}
                      </optgroup>
                    ))}
                  </select>
                </>
              )}
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              <label htmlFor="install-name" className="text-fg">
                Release
              </label>
              <input
                id="install-name"
                aria-label="Release name"
                value={name}
                disabled={busy || step === 'preview'}
                onChange={(event) => {
                  setName(event.target.value);
                }}
                className={`w-40 ${fieldClass}`}
              />
              <label htmlFor="install-namespace" className="text-fg">
                Namespace
              </label>
              <input
                id="install-namespace"
                aria-label="Namespace"
                list="install-namespaces"
                value={target}
                disabled={busy || step === 'preview'}
                onChange={(event) => {
                  setTarget(event.target.value);
                }}
                className={`w-40 ${fieldClass}`}
              />
              <datalist id="install-namespaces">
                {known.map((entry) => (
                  <option key={entry} value={entry} />
                ))}
              </datalist>
              <label className="flex items-center gap-1 text-fg">
                <input
                  type="checkbox"
                  aria-label="Create the namespace"
                  checked={createNamespace}
                  disabled={busy || step === 'preview'}
                  onChange={(event) => {
                    setCreateNamespace(event.target.checked);
                  }}
                />
                create it
              </label>
            </div>
            {step === 'edit' && (
              <>
                <div className="mt-2 flex items-center gap-2">
                  <span className="text-fg-muted">Values</span>
                  <button
                    type="button"
                    disabled={busy || choice === null}
                    onClick={() => {
                      if (choice !== null) {
                        void loadValues(choice);
                      }
                    }}
                    className={buttonClass}
                  >
                    Load the chart defaults
                  </button>
                </div>
                <div className="mt-1 h-64 overflow-hidden rounded border border-edge">
                  <Suspense fallback={<Loading what="editor" />}>
                    <YamlEditor
                      value={values}
                      path={`values/${target}/${name}.yaml`}
                      readOnly={busy}
                      onChange={setValues}
                    />
                  </Suspense>
                </div>
              </>
            )}
            {step === 'preview' && (
              <div className="mt-2 h-80 overflow-hidden rounded border border-edge">
                <Suspense fallback={<Loading what="diff" />}>
                  <ManifestDiff original="" modified={rendered} />
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
                    disabled={busy || !ready}
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
              {step === 'preview' && (
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
                    disabled={busy || !ready || noInstall !== null}
                    title={noInstall ?? `Install ${name} into ${target}`}
                    onClick={() => {
                      if (choice !== null) {
                        askInstall(choice);
                      }
                    }}
                    className={primaryClass}
                  >
                    Install {name}
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

function firstVersion(versions: HelmChartVersions): string {
  for (const [index, repo] of versions.repos.entries()) {
    const newest = repo.versions.at(0);
    if (newest !== undefined) {
      return `${String(index)}:${newest}`;
    }
  }
  return '';
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}
