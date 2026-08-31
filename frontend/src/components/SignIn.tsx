import type { Session } from '../lib/types';
import { signInHref } from '../lib/identity';
import Wordmark from './Wordmark';

interface SignInProps {
  session: Session;
}

function reason(session: Session): string {
  if (session.error !== undefined && session.error !== '') {
    return session.error;
  }
  if (session.signIn) {
    return 'Sign in with your identity provider to reach this cluster.';
  }
  return 'This spinoza expects your proxy to say who you are, and no identity reached it.';
}

export default function SignIn({ session }: SignInProps) {
  return (
    <div className="flex h-screen flex-col items-center justify-center gap-6 bg-surface font-mono text-sm text-fg">
      <Wordmark className="text-2xl text-fg-strong" />
      <div className="w-96 rounded border border-edge-strong bg-surface-raised p-5">
        <p className="font-semibold text-fg-strong">Sign in</p>
        <p className="mt-2 text-xs text-fg-soft">{reason(session)}</p>
        {session.signIn && (
          <a
            href={signInHref()}
            data-testid="sign-in"
            className="mt-4 inline-flex items-center rounded border border-edge-strong px-3 py-1.5 text-fg hover:bg-surface-active"
          >
            Sign in
          </a>
        )}
      </div>
    </div>
  );
}
