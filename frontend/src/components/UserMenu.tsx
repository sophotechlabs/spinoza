import { useRef } from 'react';
import { CONTROL } from '../lib/controls';
import { SIGN_OUT_PATH, scopeSummary } from '../lib/identity';
import { useSession } from '../store/identity';
import { useDismissMenu } from '../lib/useDismissMenu';

export default function UserMenu() {
  const session = useSession();
  const ref = useRef<HTMLDetailsElement | null>(null);

  useDismissMenu(ref);

  if (!session.cluster) {
    return null;
  }
  if (!session.authenticated) {
    return null;
  }
  if (session.mode === 'none') {
    return null;
  }

  const name = session.user ?? 'signed in';

  return (
    <details ref={ref} className="relative">
      <summary
        aria-label="Account"
        title={`Signed in as ${name}`}
        className={`${CONTROL} max-w-56 cursor-pointer list-none border-edge-strong text-fg-soft hover:bg-surface-active [&::-webkit-details-marker]:hidden`}
      >
        <span className="truncate">{name}</span>
        <span className="text-fg-muted">{session.role}</span>
      </summary>
      <div className="absolute right-0 z-30 mt-1 w-72 rounded border border-edge-strong bg-surface-raised p-3 text-xs shadow">
        <p className="font-semibold text-fg-strong">{name}</p>
        <p className="mt-1 text-fg-soft">
          Role {session.role}, reading {scopeSummary(session.scope)}.
        </p>
        {session.groups !== undefined && session.groups.length > 0 && (
          <p className="mt-1 break-words text-fg-muted">{session.groups.join(', ')}</p>
        )}
        <a
          href={SIGN_OUT_PATH}
          data-testid="sign-out"
          className="mt-3 inline-flex items-center rounded border border-edge-strong px-2 py-1 text-fg hover:bg-surface-active"
        >
          Sign out
        </a>
      </div>
    </details>
  );
}
