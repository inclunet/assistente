/** @vitest-environment jsdom */
import { describe, it, expect } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useFormField } from './useFormField';
import { required } from '../lib/formValidation';

describe('useFormField', () => {
  it('tracks value changes', () => {
    const { result } = renderHook(() => useFormField({ initialValue: '' }));

    act(() => {
      result.current.onChange('novo valor');
    });

    expect(result.current.value).toBe('novo valor');
  });

  it('validates on blur by default', () => {
    const { result } = renderHook(() => useFormField({
      initialValue: '',
      validators: [required('Obrigatório')]
    }));

    act(() => {
      result.current.onBlur();
    });

    expect(result.current.error).toBe('Obrigatório');
  });

  it('validate() returns validation result', () => {
    const { result } = renderHook(() => useFormField({
      initialValue: 'ok',
      validators: [required('Obrigatório')]
    }));

    act(() => {
      result.current.onChange('');
    });

    let validationResult = false;
    act(() => {
      validationResult = result.current.validate();
    });

    expect(validationResult).toBe(false);
    expect(result.current.error).toBe('Obrigatório');
  });
});
