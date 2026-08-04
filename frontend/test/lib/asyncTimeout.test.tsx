import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { useEffect, useState } from 'react';

function Slow() {
  const [ready, setReady] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => {
      setReady(true);
    }, 1600);
    return () => {
      clearTimeout(timer);
    };
  }, []);
  if (!ready) {
    return <span>waiting</span>;
  }
  return <span>arrived</span>;
}

describe('an assertion that has to outwait a slow machine', () => {
  it('does not give up at the one second testing-library default', async () => {
    render(<Slow />);

    expect(await screen.findByText('arrived')).toBeInTheDocument();
  });
});
