/* Field.tsx — labelled input with icon, from design/src/ui.jsx */

import { useId, type ReactNode } from 'react';
import { Icon, type IconName } from './Icon';

interface FieldProps {
  label: string;
  type?: string;
  value: string;
  onChange?: (value: string) => void;
  icon?: IconName;
  hint?: string;
  error?: boolean;
  right?: ReactNode;
  autoFocus?: boolean;
  name?: string;
  autoComplete?: string;
  required?: boolean;
  disabled?: boolean;
}

export function Field({
  label,
  type = 'text',
  value,
  onChange,
  icon,
  hint,
  error,
  right,
  autoFocus,
  name,
  autoComplete,
  required,
  disabled,
}: FieldProps): JSX.Element {
  const inputId = useId();
  const hintId = useId();

  return (
    <div className="field" style={{ minWidth: 0 }}>
      {label && (
        <label htmlFor={inputId} className="label">
          {label}
        </label>
      )}
      <div className="input-wrap">
        {icon && (
          <span className="input-icon">
            <Icon name={icon} size={18} />
          </span>
        )}
        <input
          id={inputId}
          name={name}
          type={type}
          value={value}
          autoFocus={autoFocus}
          autoComplete={autoComplete}
          required={required}
          disabled={disabled}
          aria-describedby={hint ? hintId : undefined}
          onChange={(e) => onChange?.(e.target.value)}
          className={['input', icon ? 'has-icon' : '', right ? 'has-suffix' : '', error ? 'input-err' : '']
            .filter(Boolean)
            .join(' ')}
        />
        {right && <span className="input-suffix">{right}</span>}
      </div>
      {(hint || error) && (
        <span id={hintId} className={'hint' + (error ? ' err' : '')}>
          {error || hint}
        </span>
      )}
    </div>
  );
}
