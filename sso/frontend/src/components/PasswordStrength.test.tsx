import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PasswordStrength } from './PasswordStrength';
import { I18N } from '../i18n/dictionary';

const t = I18N.en;

describe('PasswordStrength meter', () => {
  it('renders nothing for an empty password', () => {
    const { container } = render(<PasswordStrength password="" t={t} />);
    expect(container.firstChild).toBeNull();
  });

  it('shows the weak label for a trivial password', () => {
    render(<PasswordStrength password="abc" t={t} />);
    expect(screen.getByTestId('password-strength')).toBeInTheDocument();
    expect(screen.getByText(t.pw_weak)).toBeInTheDocument();
  });

  it('shows the strong label for a strong password', () => {
    render(<PasswordStrength password="Abcdefg1!" t={t} />);
    expect(screen.getByText(t.pw_strong)).toBeInTheDocument();
  });

  it('renders three bars', () => {
    const { container } = render(<PasswordStrength password="Abcdefg1" t={t} />);
    // The three meter segments live in the first row of the testid wrapper.
    const meter = screen.getByTestId('password-strength');
    const bars = meter.querySelectorAll(':scope > div > div');
    expect(bars).toHaveLength(3);
    void container;
  });
});
