import { useEffect, useState } from 'react';

export function useElementWidth(element: HTMLElement | null): number {
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (element !== null) {
      const update = () => {
        setWidth(element.clientWidth);
      };
      update();
      if (typeof ResizeObserver !== 'undefined') {
        const observer = new ResizeObserver(update);
        observer.observe(element);
        return () => {
          observer.disconnect();
        };
      }
    }
  }, [element]);
  return width;
}
