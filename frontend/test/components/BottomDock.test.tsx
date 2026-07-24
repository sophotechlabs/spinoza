import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BottomDock from '../../src/components/BottomDock';

describe('BottomDock', () => {
  it('renders collapsed with no panel body', () => {
    render(<BottomDock />);
    expect(screen.getByRole('button', { name: /Panel/ })).toHaveTextContent('▸');
    expect(screen.queryByText('No output.')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Logs' })).not.toBeInTheDocument();
  });

  it('opens and closes the panel on toggle', async () => {
    const user = userEvent.setup();
    render(<BottomDock />);
    const toggle = screen.getByRole('button', { name: /Panel/ });
    await user.click(toggle);
    expect(toggle).toHaveTextContent('▾');
    expect(screen.getByText('No output.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Logs' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeDisabled();
    await user.click(toggle);
    expect(toggle).toHaveTextContent('▸');
    expect(screen.queryByText('No output.')).not.toBeInTheDocument();
  });
});
