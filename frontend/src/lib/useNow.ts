import { useEffect, useState } from 'react';

const AGE_TICK_MS = 30000;

export function useNow(everyMs: number = AGE_TICK_MS): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = setInterval(() => {
      setNow(Date.now());
    }, everyMs);
    return () => {
      clearInterval(timer);
    };
  }, [everyMs]);

  return now;
}
