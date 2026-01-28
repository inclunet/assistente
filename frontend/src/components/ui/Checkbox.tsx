import { forwardRef, InputHTMLAttributes } from 'react';
import './Checkbox.css';

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: string;
}

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, className = '', ...props }, ref) => {
    return (
      <label className={`checkbox-wrapper ${className}`}>
        <input ref={ref} type="checkbox" className="checkbox-input" {...props} />
        <span className="checkbox-box">
          <svg
            className="checkbox-check"
            viewBox="0 0 12 10"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              d="M1 5L4.5 8.5L11 1.5"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </span>
        {label && <span className="checkbox-label">{label}</span>}
      </label>
    );
  }
);

Checkbox.displayName = 'Checkbox';
