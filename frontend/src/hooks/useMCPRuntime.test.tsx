/** @vitest-environment jsdom */
import { Suspense, startTransition } from 'react';
import { act, render, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { mcp } from '@wailsjs/go/models';
import { useMCPRuntime } from './useMCPRuntime';
import { useMCPStore } from '../store/mcpStore';
import { useUIStore } from '../store/uiStore';
import { useAuthStore } from '../store/authStore';
import { reauthorizeMCPServer } from '../services/mcpReauthorization';

const mocks = vi.hoisted(() => ({
  list: vi.fn(), reauthorize: vi.fn(), announce: vi.fn(),
  handlers: new Map<string, (data?: unknown) => void>(),
  t: (key: string) => key,
}));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: mocks.t }) }));
vi.mock('../lib/i18n', () => ({ default: { t: mocks.t } }));
vi.mock('../services/voiceAccessibility/announcerBroker', () => ({ announceWithOrigin: mocks.announce, estimateAnnouncementReadingMs: () => 3000 }));
vi.mock('../store/authStore', async () => {
  const { create } = await import('zustand');
  return { useAuthStore: create(() => ({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session', role: 'admin' } })) };
});
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, handler: (data?: unknown) => void) => {
  mocks.handlers.set(name, handler);
  return () => { mocks.handlers.delete(name); };
} }));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({
 ListMCPServers: mocks.list, ReauthorizeMCPServer: mocks.reauthorize,
 ConnectMCPServer: vi.fn(), DisconnectMCPServer: vi.fn(), ReconnectMCPServer: vi.fn(),
 SaveMCPServer: vi.fn(), SaveMCPServerWithOAuthSecret: vi.fn(), SaveMCPServerWithCredential: vi.fn(),
 DeleteMCPServer: vi.fn(), GetMCPServerTools: vi.fn(), GetMCPServerConfig: vi.fn(),
}));
const server = (overrides = {}) => new mcp.ServerInfo({ slug: 'db', name: 'Databricks', enabled: true, authType: 'oauth2_pkce', needsReauth: true, ...overrides });

beforeEach(() => {
 vi.spyOn(document, 'hasFocus').mockReturnValue(true);
 mocks.t = (key: string) => key;
 mocks.list.mockReset().mockResolvedValue([server()]);
 mocks.reauthorize.mockReset().mockResolvedValue(undefined);
 mocks.announce.mockClear();
 mocks.handlers.clear();
 useUIStore.setState({ toasts: [] });
 useMCPStore.getState().reset();
 useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session', role: 'admin' } });
});

describe('global MCP authorization presentation', () => {
 it('keeps background reauthorization state without opening consent or persistent notices', async () => {
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  expect(mocks.reauthorize).not.toHaveBeenCalled();
  expect(useUIStore.getState().toasts).toHaveLength(0);
  act(() => { mocks.handlers.get('mcp:oauth_authorize')?.({ slug: 'db' }); });
  expect(mocks.announce).toHaveBeenCalledTimes(1);
  expect(mocks.announce).toHaveBeenCalledWith(expect.objectContaining({ message: 'mcp.announce.reauthorizing', announcePriority: 'assertive' }));
  act(() => { mocks.handlers.get('mcp:oauth_device_verify')?.({ slug: 'db' }); });
  expect(mocks.announce).toHaveBeenCalledTimes(2);
  unmount();
  expect(mocks.handlers.size).toBe(0);
 });
 it('shows temporary feedback for interactive authorization in the active session', async () => {
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: true }); });
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'other-server', userId: 'user', sessionId: 'session', success: false }); });
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'other', sessionId: 'session', success: true }); });
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'old-session', success: true }); });
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => [toast.message, toast.type])).toEqual([
   ['mcp.toast.serverReauthorized', 'success'], ['mcp.toast.interactiveAuthorizationFailed', 'error'],
  ]));
  unmount();
 });
 it('waits until the application regains focus to announce interactive results', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  mocks.announce.mockClear();
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: true }); });
  expect(useUIStore.getState().toasts).toHaveLength(0);
  expect(mocks.announce).not.toHaveBeenCalled();
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.type)).toEqual(['success']));
  expect(mocks.announce).toHaveBeenCalledTimes(1);
  act(() => window.dispatchEvent(new Event('focus')));
  expect(useUIStore.getState().toasts).toHaveLength(1);
  unmount();
 });
 it('waits for visibility even if focused and delivers exactly once on visibilitychange', async () => {
  let visible = false;
  vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visible ? 'visible' : 'hidden');
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  mocks.announce.mockClear();
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: false }); });
  act(() => window.dispatchEvent(new Event('focus')));
  expect(useUIStore.getState().toasts).toHaveLength(0);
  expect(mocks.announce).not.toHaveBeenCalled();
  visible = true;
  act(() => document.dispatchEvent(new Event('visibilitychange')));
  await waitFor(() => expect(useUIStore.getState().toasts).toHaveLength(1));
  expect(mocks.announce).toHaveBeenCalledTimes(1);
  act(() => document.dispatchEvent(new Event('visibilitychange')));
  expect(useUIStore.getState().toasts).toHaveLength(1);
  unmount();
  vi.restoreAllMocks();
 });
 it('discards a deferred failure when the session changes', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: false }); });
  act(() => useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'new', role: 'admin' } }));
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  expect(useUIStore.getState().toasts).toHaveLength(0);
  unmount();
 });
 it('cancels deferred results when the runtime unmounts', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  act(() => { mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: true }); });
  unmount();
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  expect(useUIStore.getState().toasts).toHaveLength(0);
 });
 it('announces only the latest deferred result for each server', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  act(() => {
   mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: true });
   mocks.handlers.get('mcp:oauth_interactive_result')?.({ slug: 'db', userId: 'user', sessionId: 'session', success: false });
  });
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.type)).toEqual(['error']));
  unmount();
 });
 it('waits for focus before presenting an explicit authorization failure', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  mocks.reauthorize.mockRejectedValue(new Error('oauth_consent_declined'));
  const { unmount } = renderHook(useMCPRuntime);
  await act(async () => { await reauthorizeMCPServer({ slug: 'db', name: 'Databricks' }); });
  expect(useUIStore.getState().toasts).toHaveLength(0);
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.message)).toEqual(['mcp.error.consentDeclined']));
  unmount();
 });
 it('cancels deferred explicit results when the runtime unmounts', async () => {
  let focused = false;
  vi.spyOn(document, 'hasFocus').mockImplementation(() => focused);
  mocks.reauthorize.mockRejectedValue(new Error('oauth_consent_declined'));
  const { unmount } = renderHook(useMCPRuntime);
  await act(async () => { await reauthorizeMCPServer({ slug: 'db', name: 'Databricks' }); });
  unmount();
  focused = true;
  act(() => window.dispatchEvent(new Event('focus')));
  expect(useUIStore.getState().toasts).toHaveLength(0);
 });
 it('ignores explicit authorization completing after runtime unmount', async () => {
  let finish!: () => void;
  mocks.reauthorize.mockImplementation(() => new Promise<void>((resolve) => { finish = resolve; }));
  const { unmount } = renderHook(useMCPRuntime);
  const operation = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  unmount();
  await act(async () => { finish(); await operation; });
  expect(useUIStore.getState().toasts).toHaveLength(0);
 });
 it('ignores an old operation after the same session runtime remounts', async () => {
  let finish!: () => void;
  mocks.reauthorize.mockImplementation(() => new Promise<void>((resolve) => { finish = resolve; }));
  const first = renderHook(useMCPRuntime);
  const operation = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  first.unmount();
  const second = renderHook(useMCPRuntime);
  await act(async () => { finish(); await operation; });
  expect(useUIStore.getState().toasts).toHaveLength(0);
  second.unmount();
 });
 it('notifies the new runtime when it joins an authorization already running', async () => {
  let finish!: () => void;
  mocks.reauthorize.mockImplementation(() => new Promise<void>((resolve) => { finish = resolve; }));
  const first = renderHook(useMCPRuntime);
  const firstOperation = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  first.unmount();
  const second = renderHook(useMCPRuntime);
  const secondOperation = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  expect(mocks.reauthorize).toHaveBeenCalledTimes(1);
  await act(async () => { finish(); await Promise.all([firstOperation, secondOperation]); });
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.type)).toEqual(['success']));
  second.unmount();
 });
 it('deduplicates repeated explicit reauthorization actions', async () => {
  let finish!: () => void;
  mocks.reauthorize.mockImplementation(() => new Promise<void>((resolve) => { finish = resolve; }));
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  const first = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  const second = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  expect(mocks.reauthorize).toHaveBeenCalledTimes(1);
  mocks.list.mockResolvedValue([server({ needsReauth: false })]);
  await act(async () => { finish(); await Promise.all([first, second]); });
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.type)).toEqual(['success']));
  unmount();
 });
 it('uses the existing translated error on consent failure', async () => {
  mocks.reauthorize.mockRejectedValue(new Error('oauth_consent_declined'));
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  await act(async () => { await reauthorizeMCPServer({ slug: 'db', name: 'Databricks' }); });
  await waitFor(() => expect(useUIStore.getState().toasts.map((toast) => toast.message)).toEqual(['mcp.error.consentDeclined']));
  unmount();
 });
 it('ignores late results after logout', async () => {
  let finish!: () => void;
  mocks.reauthorize.mockImplementation(() => new Promise<void>((resolve) => { finish = resolve; }));
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  const operation = reauthorizeMCPServer({ slug: 'db', name: 'Databricks' });
  act(() => { useAuthStore.setState({ isAuthenticated: false, user: null }); });
  await act(async () => { finish(); await operation; });
  expect(useUIStore.getState().toasts).toHaveLength(0);
  expect(useMCPStore.getState().servers).toHaveLength(0);
  expect(mocks.handlers.size).toBe(0);
  unmount();
 });
 it('does not publish a translator from discarded concurrent render work', async () => {
  const pending = new Promise<void>(() => {});
  let suspended = false;
  function Runtime({ suspend }: { suspend: boolean }) {
   useMCPRuntime();
   if (suspend) { suspended = true; throw pending; }
   return null;
  }
  const view = render(<Suspense fallback={null}><Runtime suspend={false} /></Suspense>);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  mocks.t = (key: string) => `uncommitted:${key}`;
  act(() => { startTransition(() => { view.rerender(<Suspense fallback={null}><Runtime suspend /></Suspense>); }); });
  expect(suspended).toBe(true);
  act(() => { mocks.handlers.get('mcp:oauth_authorize')?.({ slug: 'db' }); });
  expect(mocks.announce).toHaveBeenLastCalledWith(expect.objectContaining({ message: 'mcp.announce.reauthorizing' }));
  view.unmount();
 });
 it('does not reset readiness or subscriptions when the language changes', async () => {
  const { rerender, unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  const revision = useMCPStore.getState().loadRevision;
  const calls = mocks.list.mock.calls.length;
  mocks.t = (key: string) => `translated:${key}`;
  rerender();
  expect(mocks.list).toHaveBeenCalledTimes(calls);
  expect(useMCPStore.getState().loadRevision).toBe(revision);
  act(() => { mocks.handlers.get('mcp:oauth_authorize')?.({ slug: 'db' }); });
  expect(mocks.announce).toHaveBeenCalledWith(expect.objectContaining({ message: 'translated:mcp.announce.reauthorizing' }));
  unmount();
 });
});
