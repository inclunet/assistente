import { describe, it, expect } from 'vitest';
import { required, minLength, maxLength, pattern, slug, composeValidators } from './formValidation';

describe('formValidation', () => {
  it('required() rejects empty or whitespace', () => {
    const validator = required('Obrigatório');
    expect(validator('')).toBe('Obrigatório');
    expect(validator('   ')).toBe('Obrigatório');
    expect(validator('ok')).toBeNull();
  });

  it('minLength() validates trimmed length', () => {
    const validator = minLength(3);
    expect(validator('a')).toBe('Mínimo de 3 caracteres');
    expect(validator('abc')).toBeNull();
  });

  it('maxLength() validates trimmed length', () => {
    const validator = maxLength(5);
    expect(validator('123456')).toBe('Máximo de 5 caracteres');
    expect(validator('12345')).toBeNull();
  });

  it('pattern() validates when value is present', () => {
    const validator = pattern(/^[a-z]+$/i, 'Somente letras');
    expect(validator('123')).toBe('Somente letras');
    expect(validator('abc')).toBeNull();
    expect(validator('')).toBeNull();
  });

  it('slug() validates slug format', () => {
    const validator = slug();
    expect(validator('meu-slug')).toBeNull();
    expect(validator('MeuSlug')).toBe('Slug inválido');
  });

  it('composeValidators() returns first error', () => {
    const validator = composeValidators(required('Obrigatório'), minLength(3));
    expect(validator('')).toBe('Obrigatório');
    expect(validator('ab')).toBe('Mínimo de 3 caracteres');
    expect(validator('abc')).toBeNull();
  });
});
