import { forwardRef, SelectHTMLAttributes, useId } from 'react';
import './Select.css';

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  hint?: string;
  fullWidth?: boolean;
  options: SelectOption[];
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ label, error, hint, fullWidth, options, className = '', id: externalId, ...props }, ref) => {
    const autoId = useId();
    const selectId = externalId || autoId;
    const errorId = error ? `${selectId}-error` : undefined;
    const hintId = hint ? `${selectId}-hint` : undefined;

    const describedBy = [
      props['aria-describedby'],
      errorId,
      hintId,
    ].filter(Boolean).join(' ') || undefined;

    return (
      <div className={`select-wrapper ${fullWidth ? 'select-wrapper--full' : ''}`}>
        {label && (
          <label htmlFor={selectId} className="select-label">
            {label}
            {props.required && <span aria-hidden="true"> *</span>}
          </label>
        )}
        <select
          ref={ref}
          id={selectId}
          className={`select ${error ? 'select--error' : ''} ${className}`}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          {...props}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value} disabled={option.disabled}>
              {option.label}
            </option>
          ))}
        </select>
        {hint && <span id={hintId} className="select-hint">{hint}</span>}
        {error && <span id={errorId} className="select-error" role="alert">{error}</span>}
      </div>
    );
  }
);

Select.displayName = 'Select';
