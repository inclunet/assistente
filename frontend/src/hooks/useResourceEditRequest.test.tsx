import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useResourceEditRequest } from './useResourceEditRequest';
import { useNavigationStore } from '../store/navigationStore';

describe('useResourceEditRequest', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-04T12:00:00Z'));
    useNavigationStore.getState().clearPendingEdit();
  });
  afterEach(() => { vi.useRealTimers(); });

  it('preserva o pedido recebido durante uma carga maior que cinco segundos', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'first');
    const { rerender } = renderHook(({ ready }) => useResourceEditRequest('mcp', { onEdit, ready }), { initialProps: { ready: false } });
    act(() => { vi.advanceTimersByTime(6000); });
    expect(onEdit).not.toHaveBeenCalled();
    rerender({ ready: true });
    expect(onEdit).toHaveBeenCalledOnce();
    expect(onEdit).toHaveBeenCalledWith('first', expect.objectContaining({ id: 'first' }));
    expect(useNavigationStore.getState().pendingEdit).toBeNull();
  });

  it('recusa pedido que já chegou expirado', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'expired');
    act(() => { vi.advanceTimersByTime(6000); });
    const { rerender } = renderHook(({ ready }) => useResourceEditRequest('mcp', { onEdit, ready }), { initialProps: { ready: false } });
    rerender({ ready: true });
    expect(onEdit).not.toHaveBeenCalled();
    expect(useNavigationStore.getState().pendingEdit).toBeNull();
  });

  it('respeita cancelamento enquanto aguarda dados', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'cancelled');
    const { rerender } = renderHook(({ ready }) => useResourceEditRequest('mcp', { onEdit, ready }), { initialProps: { ready: false } });
    act(() => { useNavigationStore.getState().clearPendingEdit(); vi.advanceTimersByTime(6000); });
    rerender({ ready: true });
    expect(onEdit).not.toHaveBeenCalled();
  });

  it('executa somente o pedido substituto recebido durante a espera', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'old');
    const { rerender } = renderHook(({ ready }) => useResourceEditRequest('mcp', { onEdit, ready }), { initialProps: { ready: false } });
    act(() => { vi.advanceTimersByTime(1000); useNavigationStore.getState().requestResourceEdit('mcp', 'new'); });
    act(() => { vi.advanceTimersByTime(6000); });
    rerender({ ready: true });
    expect(onEdit).toHaveBeenCalledOnce();
    expect(onEdit).toHaveBeenCalledWith('new', expect.objectContaining({ id: 'new' }));
  });

  it('não consome pedido substituto destinado a outro recurso', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'old');
    const { rerender } = renderHook(({ ready }) => useResourceEditRequest('mcp', { onEdit, ready }), { initialProps: { ready: false } });
    act(() => { useNavigationStore.getState().requestResourceEdit('providers', 'other'); });
    rerender({ ready: true });
    expect(onEdit).not.toHaveBeenCalled();
    expect(useNavigationStore.getState().pendingEdit?.resource).toBe('providers');
  });

  it('não transfere recepção para outra montagem da página', () => {
    const onEdit = vi.fn();
    useNavigationStore.getState().requestResourceEdit('mcp', 'old');
    const { unmount } = renderHook(() => useResourceEditRequest('mcp', { onEdit, ready: false }));
    unmount();
    act(() => { vi.advanceTimersByTime(6000); });
    renderHook(() => useResourceEditRequest('mcp', { onEdit }));
    expect(onEdit).not.toHaveBeenCalled();
  });
});
