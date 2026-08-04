import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import ContainerSquares from '../../src/components/ContainerSquares';
import { makeRow } from '../helpers';

describe('what a container square tells assistive technology', () => {
  it('carries the same detail the tooltip does, as an image label', () => {
    render(
      <ContainerSquares
        row={makeRow({
          containers: [
            {
              name: 'app',
              state: 'waiting',
              reason: 'CrashLoopBackOff',
              ready: false,
              restarts: 3,
              init: false,
            },
          ],
        })}
        fallback="—"
      />,
    );

    expect(
      screen.getByRole('img', { name: 'app: waiting (CrashLoopBackOff) · 3 restarts' }),
    ).toBeInTheDocument();
  });
});
