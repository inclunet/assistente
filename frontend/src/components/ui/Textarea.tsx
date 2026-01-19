import { forwardRef, TextareaHTMLAttributes } from 'react';
import './Textarea.css';

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
  fullWidth?: boolean;
  resize?: 'none' | 'vertical' | 'horizontal' | 'both';
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ label, error, fullWidth, resize = 'vertical', className = '', ...props }, ref) => {
    return (
      <div className={`textarea-wrapper ${fullWidth ? 'textarea-wrapper--full' : ''}`}>
        {label && <label className="textarea-label">{label}</label>}
        <textarea
          ref={ref}
          className={`textarea ${error ? 'textarea--error' : ''} textarea--resize-${resize} ${className}`}
          {...props}
        />
        {error && <span className="textarea-error">{error}</span>}
      </div>
    );
  }
);

Textarea.displayName = 'Textarea';
