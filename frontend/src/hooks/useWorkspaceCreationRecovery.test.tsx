import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';
import { useWorkspaceCreationRecovery } from './useWorkspaceCreationRecovery';
const events = vi.hoisted(() => ({ listener: null as ((event: unknown) => void) | null, off: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, listener: (event: unknown) => void) => {
  expect(name).toBe('workspace:creation-recovery'); events.listener = listener; return events.off;
} }));
afterEach(() => { cleanup(); vi.restoreAllMocks(); });
it('mostra aviso não bloqueante apenas ao proprietário atual e remove a assinatura', () => {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u', sessionId: 's', role: 'user' } });
  const toast = vi.spyOn(useUIStore.getState(), 'addToast');
  const hook = renderHook(useWorkspaceCreationRecovery);
  act(() => events.listener?.({ userId: 'u', sessionId: 's', path: 'C:/project/.assistente/workspace.yaml' }));
  expect(toast).toHaveBeenCalledWith(expect.any(String), 'warning', 15000);
  act(() => useAuthStore.setState({ user: { userId: 'u', sessionId: 'new', role: 'user' } }));
  act(() => events.listener?.({ userId: 'u', sessionId: 's', path: 'old/private/path' }));
  expect(toast).toHaveBeenCalledTimes(1);
  hook.unmount(); expect(events.off).toHaveBeenCalled();
});
