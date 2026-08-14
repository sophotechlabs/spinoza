interface WordmarkProps {
  className?: string;
}

export default function Wordmark({ className = 'text-fg-strong' }: WordmarkProps) {
  return <span className={`font-display font-medium tracking-[0.18em] ${className}`}>SPINOZA</span>;
}
