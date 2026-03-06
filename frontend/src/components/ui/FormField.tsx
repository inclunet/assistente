import { ReactNode, useId } from 'react';
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

  return (
    <div className="form-field-group">
      {label && (
        <label className="form-field-group__label" htmlFor={fieldId}>
          {label}
          {required && <span className="form-field-group__required">*</span>}
        </label>
      )}
      <div className="form-field-group__control" id={fieldId}>
        {children}
      </div>
      {description && !error && (
        <p className="form-field-group__description">{description}</p>
      )}
      {error && <p className="form-field-group__error">{error}</p>}
    </div>
  );
};
