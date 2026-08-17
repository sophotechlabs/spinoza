import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import MovedToDesktop from '../../src/components/MovedToDesktop';

describe('MovedToDesktop', () => {
  it('says nothing until the app has moved', () => {
    const view = render(<MovedToDesktop open={false} onStay={vi.fn()} />);

    expect(view.container).toBeEmptyDOMElement();
  });

  it('tells the tab it can be closed', () => {
    render(<MovedToDesktop open onStay={vi.fn()} />);

    expect(screen.getByText('Spinoza is back in its window')).toBeInTheDocument();
    expect(screen.getByText(/close this tab/)).toBeInTheDocument();
  });

  it('gets out of the way for someone who wants to stay', async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    render(<MovedToDesktop open onStay={onStay} />);

    await user.click(screen.getByRole('button', { name: 'Keep using this tab' }));

    expect(onStay).toHaveBeenCalledTimes(1);
  });
});
