import { InputHTMLAttributes, forwardRef, useId } from 'react';
import './Input.css';

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
  fullWidth?: boolean;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ label, error, hint, fullWidth = false, className = '', id: externalId, ...props }, ref) => {
    const autoId = useId();
    const inputId = externalId || autoId;
    const errorId = error ? `${inputId}-error` : undefined;
    const hintId = hint ? `${inputId}-hint` : undefined;

    const describedBy = [
      props['aria-describedby'],
      errorId,
      hintId,
    ].filter(Boolean).join(' ') || undefined;

    return (
      <div className={`input-wrapper ${fullWidth ? 'input-wrapper--full-width' : ''}`}>
        {label && (
          <label htmlFor={inputId} className="input__label">
            {label}
            {props.required && <span aria-hidden="true"> *</span>}
          </label>
        )}
        <input
          ref={ref}
          id={inputId}
          className={`input ${error ? 'input--error' : ''} ${className}`}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          {...props}
        />
        {hint && <span id={hintId} className="input__hint">{hint}</span>}
        {error && <span id={errorId} className="input__error">{error}</span>}
      </div>
    );
  }
);

Input.displayName = 'Input';
