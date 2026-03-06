export type Validator<T = string> = (value: T) => string | null;

export const required = (message = 'Campo obrigatório'): Validator<string> => {
  return (value) => {
    if (typeof value !== 'string') {
      return message;
    }

    return value.trim().length === 0 ? message : null;
  };
};

export const minLength = (min: number, message?: string): Validator<string> => {
  const fallback = `Mínimo de ${min} caracteres`;
  return (value) => {
    if (typeof value !== 'string') {
      return message ?? fallback;
    }

    return value.trim().length < min ? message ?? fallback : null;
  };
};

export const maxLength = (max: number, message?: string): Validator<string> => {
  const fallback = `Máximo de ${max} caracteres`;
  return (value) => {
    if (typeof value !== 'string') {
      return message ?? fallback;
    }

    return value.trim().length > max ? message ?? fallback : null;
  };
};

export const pattern = (regex: RegExp, message = 'Formato inválido'): Validator<string> => {
  return (value) => {
    if (typeof value !== 'string') {
      return message;
    }

    if (value.trim().length === 0) {
      return null;
    }

    return regex.test(value) ? null : message;
  };
};

export const slug = (message = 'Slug inválido'): Validator<string> => {
  return pattern(/^[a-z0-9]+(?:-[a-z0-9]+)*$/, message);
};

export const composeValidators = <T>(...validators: Array<Validator<T> | undefined>) => {
  return (value: T) => {
    for (const validator of validators) {
      if (!validator) continue;
      const error = validator(value);
      if (error) return error;
    }
    return null;
  };
};

export const validateValue = <T>(value: T, validators: Array<Validator<T> | undefined> = []) => {
  return composeValidators(...validators)(value);
};
