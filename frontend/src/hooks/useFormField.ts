import type { ChangeEvent } from 'react';
import { useCallback, useState } from 'react';
import type { Validator } from '../lib/formValidation';
import { validateValue } from '../lib/formValidation';

type FormChangeEvent = ChangeEvent<HTMLInputElement | HTMLTextAreaElement>;

type FormValue = string;

export interface UseFormFieldOptions {
  initialValue?: FormValue;
  validators?: Array<Validator<FormValue> | undefined>;
  validateOnChange?: boolean;
  validateOnBlur?: boolean;
  normalize?: (value: FormValue) => FormValue;
}

export interface UseFormFieldResult {
  value: FormValue;
  error: string | null;
  touched: boolean;
  setValue: (value: FormValue) => void;
  onChange: (eventOrValue: FormChangeEvent | FormValue) => void;
  onBlur: () => void;
  validate: (value?: FormValue) => boolean;
  reset: (nextValue?: FormValue) => void;
}

export const useFormField = (options: UseFormFieldOptions = {}): UseFormFieldResult => {
  const {
    initialValue = '',
    validators = [],
    validateOnChange = false,
    validateOnBlur = true,
    normalize,
  } = options;

  const [value, setValueState] = useState<FormValue>(initialValue);
  const [error, setError] = useState<string | null>(null);
  const [touched, setTouched] = useState(false);

  const runValidation = useCallback(
    (nextValue: FormValue) => {
      const normalizedValue = normalize ? normalize(nextValue) : nextValue;
      const validationError = validateValue(normalizedValue, validators) ?? null;
      setError(validationError);
      return !validationError;
    },
    [normalize, validators]
  );

  const setValue = useCallback(
    (nextValue: FormValue) => {
      const normalizedValue = normalize ? normalize(nextValue) : nextValue;
      setValueState(normalizedValue);
      if (validateOnChange) {
        runValidation(normalizedValue);
      }
    },
    [normalize, runValidation, validateOnChange]
  );

  const onChange = useCallback(
    (eventOrValue: FormChangeEvent | FormValue) => {
      if (typeof eventOrValue === 'string') {
        setValue(eventOrValue);
        return;
      }
      setValue(eventOrValue.target.value);
    },
    [setValue]
  );

  const onBlur = useCallback(() => {
    setTouched(true);
    if (validateOnBlur) {
      runValidation(value);
    }
  }, [runValidation, validateOnBlur, value]);

  const validate = useCallback(
    (nextValue?: FormValue) => {
      const valueToValidate = nextValue ?? value;
      return runValidation(valueToValidate);
    },
    [runValidation, value]
  );

  const reset = useCallback(
    (nextValue: FormValue = initialValue) => {
      const normalizedValue = normalize ? normalize(nextValue) : nextValue;
      setValueState(normalizedValue);
      setError(null);
      setTouched(false);
    },
    [initialValue, normalize]
  );

  return {
    value,
    error,
    touched,
    setValue,
    onChange,
    onBlur,
    validate,
    reset,
  };
};
