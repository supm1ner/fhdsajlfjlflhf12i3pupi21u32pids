import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { Field } from './Field';

describe('Field', () => {
  it('associates the label with the input', () => {
    render(<Field label="Email" value="" onChange={() => {}} />);
    // getByLabelText resolves the <label htmlFor> ↔ input id association.
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
  });

  it('reports typed characters through onChange', async () => {
    const onChange = vi.fn();
    render(<Field label="Email" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Email'), 'a');
    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('is controlled by the value prop', async () => {
    function Harness(): JSX.Element {
      const [v, setV] = useState('');
      return <Field label="Name" value={v} onChange={setV} />;
    }
    render(<Harness />);
    const input = screen.getByLabelText('Name') as HTMLInputElement;
    await userEvent.type(input, 'cotton');
    expect(input.value).toBe('cotton');
  });

  it('renders a hint and passes through type/autoComplete', () => {
    render(
      <Field
        label="Password"
        type="password"
        autoComplete="current-password"
        hint="At least 8 characters"
        value=""
        onChange={() => {}}
      />,
    );
    const input = screen.getByLabelText('Password');
    expect(input).toHaveAttribute('type', 'password');
    expect(input).toHaveAttribute('autocomplete', 'current-password');
    expect(screen.getByText('At least 8 characters')).toBeInTheDocument();
  });
});
