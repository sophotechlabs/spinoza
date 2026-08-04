import { Component } from 'react';
import type { ReactNode } from 'react';

interface ErrorBoundaryProps {
  label: string;
  children: ReactNode;
}

interface ErrorBoundaryState {
  message: string | null;
}

function messageOf(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the cause was not an Error';
}

export default class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { message: null };

  static getDerivedStateFromError(err: unknown): ErrorBoundaryState {
    return { message: messageOf(err) };
  }

  retry = () => {
    this.setState({ message: null });
  };

  render() {
    if (this.state.message === null) {
      return this.props.children;
    }
    return (
      <div role="alert" className="flex min-h-0 flex-1 items-start justify-center p-6 text-xs">
        <div className="max-w-2xl rounded border border-error-line bg-error-tint/40 px-3 py-2">
          <div className="font-semibold text-error">{this.props.label} stopped rendering</div>
          <div className="mt-1 break-words text-error-strong">{this.state.message}</div>
          <button
            type="button"
            onClick={this.retry}
            className="mt-1.5 rounded border border-error-line-strong px-1.5 py-0.5 text-error-contrast hover:bg-error-tint-strong"
          >
            Try again
          </button>
        </div>
      </div>
    );
  }
}
