import { ReactNode, useId, cloneElement, isValidElement } from 'react';
import './FormField.css';

export interface FormFieldProps {
  label?: string;
  description?: string;
  error?: string | null;
  required?: boolean;
  id?: string;
  children: ReactNode;
}

export const FormField = ({
  label,
  description,
  error,
  required = false,
  id,
  children,
}: FormFieldProps) => {
  const generatedId = useId();
  const fieldId = id ?? generatedId;

  // Propaga fieldId para o input filho se for um elemento React válido
  const childrenWithId = isValidElement(children)
    ? cloneElement(children as React.ReactElement<{ id?: string }>, { id: fieldId })
    : children;

  return (
    <div className="form-field-group">
      {label && (
        <label className="form-field-group__label" htmlFor={fieldId}>
          {label}
          {required && <span className="form-field-group__required">*</span>}
        </label>
      )}
      <div className="form-field-group__control">
        {childrenWithId}
      </div>
      {description && !error && (
        <p className="form-field-group__description">{description}</p>
      )}
      {error && <p className="form-field-group__error">{error}</p>}
    </div>
  );
};
