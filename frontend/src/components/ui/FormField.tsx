import { ReactNode, useId, cloneElement, isValidElement } from 'react';
import './FormField.css';

export interface FormFieldProps {
  label?: string;
  description?: string;
  error?: string | null;
  required?: boolean;
  id?: string;
  visuallyHidden?: boolean;
  children: ReactNode;
}

export const FormField = ({
  label,
  description,
  error,
  required = false,
  id,
  visuallyHidden = false,
  children,
}: FormFieldProps) => {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const descId = description ? `${fieldId}-desc` : undefined;
  const errorId = error ? `${fieldId}-error` : undefined;

  const childrenWithId = isValidElement(children)
    ? cloneElement(children as React.ReactElement<{ id?: string; 'aria-describedby'?: string }>, {
        id: fieldId,
        'aria-describedby': [descId, errorId].filter(Boolean).join(' ') || undefined,
      })
    : children;

  return (
    <div className="form-field-group">
      {label && (
        <label
          className={`form-field-group__label${visuallyHidden ? ' sr-only' : ''}`}
          htmlFor={fieldId}
        >
          {label}
          {required && <span className="form-field-group__required" aria-hidden="true">*</span>}
          {required && <span className="sr-only"> ({'\u00A0'}obrigatório)</span>}
        </label>
      )}
      <div className="form-field-group__control">
        {childrenWithId}
      </div>
      {description && !error && (
        <p id={descId} className="form-field-group__description">{description}</p>
      )}
      {error && <p id={errorId} className="form-field-group__error">{error}</p>}
    </div>
  );
};
