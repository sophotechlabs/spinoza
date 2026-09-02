import { useState } from 'react';
import type { RBACGrant, RBACIndex, RBACSubject } from '../lib/types';
import {
  NO_ASK,
  askable,
  fetchWho,
  grantLabel,
  grantNote,
  matches,
  ruleLabel,
  useRBAC,
  whoFailure,
  whereLabel,
} from '../lib/rbac';
import type { Ask } from '../lib/rbac';
import { CONTROL } from '../lib/controls';

const FIELD = 'rounded border border-edge-strong bg-surface px-2 py-1 text-fg';
import { notifyError } from '../store/toasts';
import LoadFailure from './LoadFailure';
import LoadWarning from './LoadWarning';
import Loading from './Loading';
import { useClusterEpoch } from '../store/cluster';

function Powers({ powers }: { powers: string[] }) {
  if (powers.length === 0) {
    return <span className="text-fg-faint">nothing worth naming</span>;
  }
  return (
    <span className="flex flex-wrap gap-1">
      {powers.map((one) => (
        <span
          key={one}
          className={`rounded border px-1 text-[10px] ${
            one === 'cluster-admin' ? 'border-error-line text-error' : 'border-edge text-warn'
          }`}
        >
          {one}
        </span>
      ))}
    </span>
  );
}

function Grant({ grant }: { grant: RBACGrant }) {
  const note = grantNote(grant);
  return (
    <li className="py-0.5">
      <span className="text-fg-soft">{grantLabel(grant)}</span>
      {note !== '' && <span className="ml-2 text-warn">{note}</span>}
      <ul className="pl-4">
        {(grant.rules ?? []).map((rule, at) => (
          <li key={`${grant.binding}-${String(at)}`} className="text-fg-muted">
            {ruleLabel(rule)}
          </li>
        ))}
      </ul>
    </li>
  );
}

function Row({ subject }: { subject: RBACSubject }) {
  const [open, setOpen] = useState(false);
  return (
    <li className="border-b border-edge px-2 py-1">
      <div className="flex items-baseline gap-3">
        <button
          type="button"
          aria-expanded={open}
          aria-label={`${open ? 'Hide' : 'Show'} what ${subject.label} is bound to`}
          onClick={() => {
            setOpen((was) => !was);
          }}
          className="w-6 shrink-0 text-left text-fg-muted"
        >
          {open ? '▾' : '▸'}
        </button>
        <span className="w-80 shrink-0 truncate text-fg-strong" title={subject.label}>
          {subject.label}
        </span>
        <span className="w-24 shrink-0 truncate text-fg-muted">{subject.kind}</span>
        <span className="min-w-0 flex-1">
          <Powers powers={subject.powers ?? []} />
        </span>
        <span className="w-56 shrink-0 truncate text-fg-muted" title={whereLabel(subject)}>
          {whereLabel(subject)}
        </span>
      </div>
      {open && (
        <ul className="pt-1 pl-9">
          {subject.grants.map((grant) => (
            <Grant
              key={`${grant.bindingKind}/${grant.namespace ?? ''}/${grant.binding}`}
              grant={grant}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

function Question({ onAnswer }: { onAnswer: (found: RBACIndex | null) => void }) {
  const [ask, setAsk] = useState<Ask>(NO_ASK);
  const [busy, setBusy] = useState(false);

  async function run() {
    setBusy(true);
    try {
      onAnswer(await fetchWho(ask));
    } catch (err: unknown) {
      notifyError(whoFailure(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-edge px-2 py-1.5">
      <span className="text-fg-soft">Who can</span>
      <input
        aria-label="Verb"
        placeholder="create"
        value={ask.verb}
        onChange={(event) => {
          setAsk((held) => ({ ...held, verb: event.target.value }));
        }}
        className={`${FIELD} w-24`}
      />
      <input
        aria-label="Resource"
        placeholder="pods/exec"
        value={ask.resource}
        onChange={(event) => {
          setAsk((held) => ({ ...held, resource: event.target.value }));
        }}
        className={`${FIELD} w-40`}
      />
      <input
        aria-label="API group"
        placeholder="core"
        value={ask.group}
        onChange={(event) => {
          setAsk((held) => ({ ...held, group: event.target.value }));
        }}
        className={`${FIELD} w-32`}
      />
      <input
        aria-label="Namespace"
        placeholder="anywhere"
        value={ask.namespace}
        onChange={(event) => {
          setAsk((held) => ({ ...held, namespace: event.target.value }));
        }}
        className={`${FIELD} w-32`}
      />
      <button
        type="button"
        disabled={busy || !askable(ask)}
        onClick={() => {
          void run();
        }}
        className={`${CONTROL} border-edge-strong text-fg hover:bg-surface-active disabled:opacity-50`}
      >
        Ask
      </button>
      <button
        type="button"
        onClick={() => {
          setAsk(NO_ASK);
          onAnswer(null);
        }}
        className={`${CONTROL} border-edge-strong text-fg-soft hover:bg-surface-active`}
      >
        Everyone
      </button>
    </div>
  );
}

export default function Rbac() {
  const epoch = useClusterEpoch();
  const { data, error } = useRBAC();
  const [answer, setAnswer] = useState<RBACIndex | null>(null);
  const [answeredOn, setAnsweredOn] = useState(epoch);
  const [query, setQuery] = useState('');

  if (data === null) {
    if (error !== null) {
      return <LoadFailure what="The permission index" message={error} />;
    }
    return <Loading what="who can do what" />;
  }

  let visibleAnswer = answer;
  if (answeredOn !== epoch) {
    visibleAnswer = null;
  }
  const shown = visibleAnswer ?? data;
  const subjects = shown.subjects.filter((one) => matches(one, query));
  return (
    <div className="flex h-full min-h-0 flex-col text-xs">
      {shown.error !== undefined && shown.error !== '' && <LoadWarning message={shown.error} />}
      <Question
        onAnswer={(found) => {
          setAnswer(found);
          setAnsweredOn(epoch);
        }}
      />
      <div className="flex shrink-0 items-center gap-3 border-b border-edge px-2 py-1.5 text-fg-muted">
        <input
          aria-label="Filter subjects"
          placeholder="filter"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
          }}
          className={`${FIELD} w-56`}
        />
        <span>
          {subjects.length} {visibleAnswer === null ? 'subjects' : 'can'}
        </span>
        {shown.dropped !== undefined && shown.dropped > 0 && (
          <span>{shown.dropped} more are not shown</span>
        )}
        {(shown.absent ?? []).length > 0 && (
          <span className="text-warn" title={(shown.absent ?? []).join('\n')}>
            {(shown.absent ?? []).length} bindings name a role that does not exist
          </span>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {subjects.length === 0 && <p className="p-3 text-fg-muted">Nobody.</p>}
        <ul>
          {subjects.map((one) => (
            <Row key={`${one.kind}/${one.namespace ?? ''}/${one.name}`} subject={one} />
          ))}
        </ul>
      </div>
    </div>
  );
}
