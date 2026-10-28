/** @vitest-environment jsdom */
import { act, renderHook, waitFor } from '@testing-library/react';
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
vi.mock('../services/voiceAccessibility/announcerBroker', () => ({ announceWithOrigin: mocks.announce }));
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
  expect(useUIStore.getState().toasts.map((toast) => toast.type)).toEqual(['success']);
  unmount();
 });
 it('uses the existing translated error on consent failure', async () => {
  mocks.reauthorize.mockRejectedValue(new Error('oauth_consent_declined'));
  const { unmount } = renderHook(useMCPRuntime);
  await waitFor(() => expect(useMCPStore.getState().servers).toHaveLength(1));
  await act(async () => { await reauthorizeMCPServer({ slug: 'db', name: 'Databricks' }); });
  expect(useUIStore.getState().toasts.map((toast) => toast.message)).toEqual(['mcp.error.consentDeclined']);
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
