/** @vitest-environment jsdom */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { useUIStore } from '../store/uiStore';
import { registerAnnouncerSink, unregisterAnnouncerSink } from './voiceAccessibility/announcerBroker';
import { activateMCPAuthorizationNotices, getMCPAuthorizationNoticeOwner, presentMCPAuthorizationNotice } from './mcpAuthorizationNotice';

vi.mock('../store/authStore', async () => {
  const { create } = await import('zustand');
  return { useAuthStore: create(() => ({ isAuthenticated: true, user: { userId: 'u', sessionId: 's' } })) };
});

describe('MCP authorization notices', () => {
  beforeEach(() => { useUIStore.setState({ toasts: [] }); });
  afterEach(() => { unregisterAnnouncerSink(); vi.restoreAllMocks(); vi.useRealTimers(); });

  it('announces all deferred server results together through the real broker', async () => {
    let focused = false;
    vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    const session = { userId: 'u', sessionId: 's' };
    const deactivate = activateMCPAuthorizationNotices(session);
    const owner = getMCPAuthorizationNoticeOwner(session);
    presentMCPAuthorizationNotice(session, 'one', owner, () => 'Servidor um autorizado', 'success');
    presentMCPAuthorizationNotice(session, 'two', owner, () => 'Servidor dois falhou', 'error');
    expect(sink).not.toHaveBeenCalled();
    focused = true;
    window.dispatchEvent(new Event('focus'));
    await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.message)).toEqual(['Servidor um autorizado', 'Servidor dois falhou']));
    expect(sink).toHaveBeenCalledTimes(1);
    expect(sink).toHaveBeenCalledWith('Servidor um autorizado. Servidor dois falhou', 'assertive');
    deactivate();
  });
  it('combines results arriving in the same frame while focused', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    const session = { userId: 'u', sessionId: 's' };
    const deactivate = activateMCPAuthorizationNotices(session);
    const owner = getMCPAuthorizationNoticeOwner(session);
    presentMCPAuthorizationNotice(session, 'one', owner, () => 'Primeiro servidor', 'success');
    presentMCPAuthorizationNotice(session, 'two', owner, () => 'Segundo servidor', 'success');
    await waitFor(() => expect(sink).toHaveBeenCalledTimes(1));
    expect(sink).toHaveBeenCalledWith('Primeiro servidor. Segundo servidor', 'polite');
    deactivate();
  });

  it('waits for the first announcement before speaking a result from a later frame', () => {
    vi.useFakeTimers();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    const session = { userId: 'u', sessionId: 's' };
    const deactivate = activateMCPAuthorizationNotices(session);
    const owner = getMCPAuthorizationNoticeOwner(session);
    presentMCPAuthorizationNotice(session, 'one', owner, () => 'Primeiro servidor', 'success');
    vi.advanceTimersByTime(20);
    expect(sink).toHaveBeenCalledTimes(1);
    presentMCPAuthorizationNotice(session, 'two', owner, () => 'Segundo servidor', 'success');
    vi.advanceTimersByTime(100);
    expect(sink).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(3000);
    expect(sink).toHaveBeenCalledTimes(2);
    expect(sink).toHaveBeenLastCalledWith('Segundo servidor', 'polite');
    deactivate();
  });

});
