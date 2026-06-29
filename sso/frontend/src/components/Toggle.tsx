/* Toggle.tsx — switch toggle (new design from tokens.css .switch) */

interface ToggleProps {
  on: boolean;
  onChange: (on: boolean) => void;
  label?: string;
}

export function Toggle({ on, onChange, label }: ToggleProps): JSX.Element {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      onClick={() => onChange(!on)}
      className="switch"
    />
  );
}
