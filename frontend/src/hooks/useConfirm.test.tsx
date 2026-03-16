/** @vitest-environment jsdom */
import { describe, it, expect, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useConfirm } from './useConfirm';
import type { ConfirmOptions } from '../store/confirmStore';

const requestConfirmMock = vi.fn();

vi.mock('../store/confirmStore', () => ({
  requestConfirm: (...args: unknown[]) => requestConfirmMock(...args),
}));

describe('useConfirm', () => {
  it('chama requestConfirm com as opcoes fornecidas', () => {
    const { result } = renderHook(() => useConfirm());

    const options: ConfirmOptions = {
      title: 'Confirmar acao',
      message: 'Tem certeza?',
      confirmText: 'OK',
      cancelText: 'Cancelar',
    };

    act(() => {
      result.current(options);
    });

    expect(requestConfirmMock).toHaveBeenCalledWith(options);
  });
});
