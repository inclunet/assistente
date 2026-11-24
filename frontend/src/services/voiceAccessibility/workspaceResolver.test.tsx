import type { WorkspaceData } from '../../store/workspaceStore';
import { canStartSTT, requestSTTStart, resetSTTGateForTests } from './sttGate';
import { announceWithOrigin, registerAnnouncerSink, unregisterAnnouncerSink, estimateAnnouncementReadingMs } from './announcerBroker';
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

  it.each(['Atual', ''])('usa o título vigente na entrega do aviso adiado: %s', title => {
    const original = hoisted.workspace;
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    vi.useFakeTimers();
    try {
      hoisted.workspace = { ...original, tabs: [original.tabs[0], { ...original.tabs[1], type: 'terminal', title: 'Antes' }] };
      renderHook(() => useVoiceAccessibilityWorkspaceResolver(true));
      announceWithOrigin({ message: 'Leitura', protectsReading: true });
      announceWithOrigin({ message: 'privado', eventType: 'completion', origin: { surfaceType: 'terminal', tabId: 'tab-2', title: 'Antes' } });
      hoisted.workspace = { ...original, tabs: title ? [original.tabs[0], { ...original.tabs[1], type: 'terminal', title }] : [original.tabs[0]] };
      vi.advanceTimersByTime(estimateAnnouncementReadingMs('Leitura'));
      expect(sink.mock.calls.map(([message]) => message)).toEqual(['Leitura', title
        ? 'Atividade no terminal Atual em segundo plano' : 'Atividade no terminal em segundo plano']);
    } finally { unregisterAnnouncerSink(); vi.useRealTimers(); hoisted.workspace = original; }
  });

  it.each([
    { sessionId: 's', tabId: 'tab-2', expected: 'Atividade no terminal Build em segundo plano' },
    { sessionId: 's', tabId: undefined, expected: 'Atividade no terminal Build em segundo plano' },
    { sessionId: 'outra', tabId: 'tab-2', expected: 'Atividade no terminal em segundo plano' },
    { sessionId: 'outra', tabId: undefined, expected: 'Atividade no terminal em segundo plano' },
  ])('identifica atividade apenas da sessão vinculada à aba ($sessionId, $tabId)', ({ sessionId, tabId, expected }) => {
    const original = hoisted.workspace;
    hoisted.workspace = { ...original, tabs: [original.tabs[0], {
      ...original.tabs[1], type: 'terminal', title: 'Build', state: { sessionId },
    }] };
    const sink = vi.fn();
    registerAnnouncerSink(sink);
    try {
      renderHook(() => useVoiceAccessibilityWorkspaceResolver(true));
      announceWithOrigin({ message: 'detalhe', eventType: 'completion', origin: { terminalSessionId: 's', tabId } });
      expect(sink).toHaveBeenCalledWith(expected, 'polite');
    } finally { unregisterAnnouncerSink(); hoisted.workspace = original; }
  });

  it('cancela a captura quando a aba ativa passa a exibir outra sessão', () => {
    const original = hoisted.workspace;
    try {
      const tab = { ...original.tabs[0], type: 'terminal' as const, state: { sessionId: 'a' } };
      const previous = { ...original, tabs: [tab] };
      hoisted.workspace = previous;
      renderHook(() => useVoiceAccessibilityWorkspaceResolver(true));
      const cancel = vi.fn();
      expect(requestSTTStart({ origin: { terminalSessionId: 'a' }, cancel })).toBe(true);
      hoisted.workspace = { ...previous, tabs: [{ ...tab, state: { sessionId: 'b' } }] };
      const listener = hoisted.subscribe.mock.calls[0][0] as (next: { workspace: WorkspaceData }, prev: { workspace: WorkspaceData }) => void;
      listener({ workspace: hoisted.workspace }, { workspace: previous });
      expect(cancel).toHaveBeenCalledTimes(1);
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
