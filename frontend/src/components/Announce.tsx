interface AnnounceProps {
  message: string | null;
  urgent?: boolean;
  className?: string;
}

function roleFor(urgent: boolean): 'alert' | 'status' {
  if (urgent) {
    return 'alert';
  }
  return 'status';
}

function classFor(message: string | null, className: string): string {
  if (message === null) {
    return '';
  }
  return className;
}

export default function Announce({ message, urgent = false, className = '' }: AnnounceProps) {
  return (
    <p role={roleFor(urgent)} className={classFor(message, className)}>
      {message}
    </p>
  );
}
