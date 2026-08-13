import App from './App';
import ErrorBoundary from './components/ErrorBoundary';

export default function Root() {
  return (
    <ErrorBoundary label="Spinoza">
      <App />
    </ErrorBoundary>
  );
}
