import { useEffect } from 'react';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import Loading from './components/Loading';
import SignIn from './components/SignIn';
import { OWN_WINDOW, fetchSession, signedOut } from './lib/identity';
import { adoptSession, useSession, useSessionKnown } from './store/identity';

export default function Root() {
  const session = useSession();
  const known = useSessionKnown();

  useEffect(() => {
    let live = true;
    fetchSession()
      .then((found) => {
        if (live) {
          adoptSession(found);
        }
      })
      .catch(() => {
        if (live) {
          adoptSession(OWN_WINDOW);
        }
      });
    return () => {
      live = false;
    };
  }, []);

  if (!known) {
    return <Loading what="spinoza" />;
  }
  if (signedOut(session)) {
    return <SignIn session={session} />;
  }
  return (
    <ErrorBoundary label="Spinoza">
      <App />
    </ErrorBoundary>
  );
}
