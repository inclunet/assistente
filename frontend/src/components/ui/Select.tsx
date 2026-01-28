import { forwardRef, SelectHTMLAttributes } from 'react';
import './Select.css';

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  fullWidth?: boolean;
  options: SelectOption[];
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ label, error, fullWidth, options, className = '', ...props }, ref) => {
    return (
      <div className={`select-wrapper ${fullWidth ? 'select-wrapper--full' : ''}`}>
        {label && <label className="select-label">{label}</label>}
        <select
          ref={ref}
          className={`select ${error ? 'select--error' : ''} ${className}`}
          {...props}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value} disabled={option.disabled}>
              {option.label}
            </option>
          ))}
        </select>
        {error && <span className="select-error">{error}</span>}
      </div>
    );
  }
);

Select.displayName = 'Select';
