import type { WorkspaceData } from '../../store/workspaceStore';
import { canStartSTT, requestSTTStart, resetSTTGateForTests } from './sttGate';
import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
  } as WorkspaceData,
  subscribe: vi.fn<(listener: unknown) => () => void>(),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({ workspace: hoisted.workspace }),
    subscribe: (listener: unknown) => hoisted.subscribe(listener),
  },
}));

describe('useVoiceAccessibilityWorkspaceResolver', () => {
  beforeEach(() => {
    hoisted.subscribe.mockReset();
    hoisted.subscribe.mockReturnValue(vi.fn());

    hoisted.workspace.activeTabId = 'tab-1';
  });

  afterEach(resetSTTGateForTests);

  it('registra resolver global baseado na aba ativa do workspace', () => {
    renderHook(() => useVoiceAccessibilityWorkspaceResolver(true));

    expect(canStartSTT({ tabId: 'tab-1' })).toBe(true);
    expect(canStartSTT({ tabId: 'tab-2' })).toBe(false);
    expect(canStartSTT({ conversationId: 'conv-1' })).toBe(true);
    expect(canStartSTT({ conversationId: 'conv-2' })).toBe(false);
  });

  it.each([
    { terminalSessionId: 's' },
    { surfaceType: 'terminal' as const, tabId: 'tab-1' },
  ])('trata o terminal selecionado como inativo enquanto o painel está fora da tela', origin => {
    const original = hoisted.workspace;
    hoisted.workspace = { ...original, tabs: [{ ...original.tabs[0], type: 'terminal', state: { sessionId: 's' } }] };
    try {
      const { rerender } = renderHook(({ visible }) => useVoiceAccessibilityWorkspaceResolver(visible), { initialProps: { visible: true } });
      const cancel = vi.fn();
      expect(requestSTTStart({ origin, cancel })).toBe(true);
      rerender({ visible: false });
      expect(cancel).toHaveBeenCalledTimes(1);
      expect(requestSTTStart({ origin, cancel })).toBe(false);
      rerender({ visible: true });
      expect(canStartSTT(origin)).toBe(true);
      expect(cancel).toHaveBeenCalledTimes(1);
      expect(hoisted.subscribe).toHaveBeenCalledTimes(1);
    } finally { hoisted.workspace = original; }
  });

  it('cancela STT inativo quando a aba ativa muda', () => {
    renderHook(() => useVoiceAccessibilityWorkspaceResolver(true));

    const listener = hoisted.subscribe.mock.calls[0][0] as (
      state: { workspace: typeof hoisted.workspace },
      previousState: { workspace: typeof hoisted.workspace },
    ) => void;

    const cancel = vi.fn();
    expect(requestSTTStart({ origin: { tabId: 'tab-1' }, cancel })).toBe(true);
    const previous = { ...hoisted.workspace };
    hoisted.workspace.activeTabId = 'tab-2';
    listener({ workspace: hoisted.workspace }, { workspace: previous });
    expect(cancel).toHaveBeenCalledTimes(1);
  });
});
