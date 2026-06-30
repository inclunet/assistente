import { forwardRef, TextareaHTMLAttributes, useId } from 'react';
import './Textarea.css';

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
  hint?: string;
  fullWidth?: boolean;
  resize?: 'none' | 'vertical' | 'horizontal' | 'both';
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ label, error, hint, fullWidth, resize = 'vertical', className = '', id: externalId, ...props }, ref) => {
    const autoId = useId();
    const textareaId = externalId || autoId;
    const errorId = error ? `${textareaId}-error` : undefined;
    const hintId = hint ? `${textareaId}-hint` : undefined;

    const describedBy = [
      props['aria-describedby'],
      errorId,
      hintId,
    ].filter(Boolean).join(' ') || undefined;

    return (
      <div className={`textarea-wrapper ${fullWidth ? 'textarea-wrapper--full' : ''}`}>
        {label && (
          <label htmlFor={textareaId} className="textarea-label">
            {label}
            {props.required && <span aria-hidden="true"> *</span>}
          </label>
        )}
        <textarea
          ref={ref}
          id={textareaId}
          className={`textarea ${error ? 'textarea--error' : ''} textarea--resize-${resize} ${className}`}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          {...props}
        />
        {hint && <span id={hintId} className="textarea-hint">{hint}</span>}
        {error && <span id={errorId} className="textarea-error">{error}</span>}
      </div>
    );
  }
);

Textarea.displayName = 'Textarea';
