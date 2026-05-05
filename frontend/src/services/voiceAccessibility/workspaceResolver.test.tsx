import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useVoiceAccessibilityWorkspaceResolver } from './workspaceResolver';

const hoisted = vi.hoisted(() => ({
  workspace: {
    id: 'workspace-1',
    name: 'Workspace',
    activeTabId: 'tab-1',
    tabs: [
      { id: 'tab-1', type: 'chat', conversationId: 'conv-1', title: 'Chat 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversationId: 'conv-2', title: 'Chat 2', position: 1 },
    ],
  },
  subscribe: vi.fn<(listener: unknown) => () => void>(),
  registerResolver: vi.fn<(resolver: unknown) => () => void>(() => vi.fn()),
  cancelInactiveSTTSession: vi.fn(),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({ workspace: hoisted.workspace }),
    subscribe: (listener: unknown) => hoisted.subscribe(listener),
  },
}));

vi.mock('./announcerBroker', () => ({
  registerVoiceAccessibilityActiveResolver: (resolver: unknown) => hoisted.registerResolver(resolver),
}));

vi.mock('./sttGate', () => ({
  cancelInactiveSTTSession: () => hoisted.cancelInactiveSTTSession(),
}));

describe('useVoiceAccessibilityWorkspaceResolver', () => {
  beforeEach(() => {
    hoisted.subscribe.mockReset();
    hoisted.subscribe.mockReturnValue(vi.fn());
    hoisted.registerResolver.mockClear();
    hoisted.cancelInactiveSTTSession.mockClear();
    hoisted.workspace.activeTabId = 'tab-1';
  });

  it('registra resolver global baseado na aba ativa do workspace', () => {
    renderHook(() => useVoiceAccessibilityWorkspaceResolver());

    const resolver = hoisted.registerResolver.mock.calls[0][0] as (origin?: { tabId?: string; conversationId?: string }) => boolean;

    expect(resolver({ tabId: 'tab-1' })).toBe(true);
    expect(resolver({ tabId: 'tab-2' })).toBe(false);
    expect(resolver({ conversationId: 'conv-1' })).toBe(true);
    expect(resolver({ conversationId: 'conv-2' })).toBe(false);
  });

  it('cancela STT inativo quando a aba ativa muda', () => {
    renderHook(() => useVoiceAccessibilityWorkspaceResolver());

    const listener = hoisted.subscribe.mock.calls[0][0] as (
      state: { workspace: typeof hoisted.workspace },
      previousState: { workspace: typeof hoisted.workspace },
    ) => void;

    listener(
      { workspace: { ...hoisted.workspace, activeTabId: 'tab-2' } },
      { workspace: { ...hoisted.workspace, activeTabId: 'tab-1' } },
    );

    expect(hoisted.cancelInactiveSTTSession).toHaveBeenCalled();
  });
});
