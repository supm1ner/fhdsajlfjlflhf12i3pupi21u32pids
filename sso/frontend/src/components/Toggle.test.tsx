import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Toggle } from './Toggle';

describe('Toggle', () => {
  it('exposes switch semantics with the correct checked state', () => {
    render(<Toggle on={true} onChange={() => {}} label="Remember me" />);
    const sw = screen.getByRole('switch', { name: 'Remember me' });
    expect(sw).toHaveAttribute('aria-checked', 'true');
  });

  it('toggles the value on click', async () => {
    const onChange = vi.fn();
    render(<Toggle on={false} onChange={onChange} label="Remember me" />);
    await userEvent.click(screen.getByRole('switch', { name: 'Remember me' }));
    expect(onChange).toHaveBeenCalledWith(true);
  });
});
