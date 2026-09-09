import { useEffect, useState } from 'react';

export const CANVAS_FLOOR = 1280;

export function tooNarrow(width: number): boolean {
  return width < CANVAS_FLOOR;
}

export function useNarrowWindow(): boolean {
  const [narrow, setNarrow] = useState(() => tooNarrow(window.innerWidth));

  useEffect(() => {
    function measure() {
      setNarrow(tooNarrow(window.innerWidth));
    }
    measure();
    window.addEventListener('resize', measure);
    return () => {
      window.removeEventListener('resize', measure);
    };
  }, []);

  return narrow;
}
