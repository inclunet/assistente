/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useSubAgentRunEvents } from './useSubAgentRunEvents';
import {
  SUBAGENT_RUN_FINISHED_EVENT,
  SUBAGENT_RUN_STARTED_EVENT,
  type SubAgentRunEvent,
} from '../types/subagentRuns';

const handlers = new Map<string, (data: unknown) => void>();
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: (data: unknown) => void) => {
    handlers.set(event, handler);
    return vi.fn();
  },
}));

const announceRequestSpy = vi.fn();
vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: announceRequestSpy }),
}));

const fetchRunsSpy = vi.fn();
vi.mock('../store/subAgentRunsStore', () => ({
  useSubAgentRunsStore: { getState: () => ({ fetchRuns: fetchRunsSpy }) },
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { title?: string; defaultValue?: string }) =>
      options?.title ? `${key}:${options.title}` : key,
  }),
}));

function runEvent(overrides: Partial<SubAgentRunEvent> = {}): SubAgentRunEvent {
  return {
    runId: 'run-1',
    conversationId: 'conv-1',
    parentConversationId: 'parent-1',
    title: 'Revisar PR',
    status: 'running',
    background: true,
    ...overrides,
  };
}

describe('useSubAgentRunEvents', () => {
  beforeEach(() => {
    handlers.clear();
    announceRequestSpy.mockClear();
    fetchRunsSpy.mockClear();
  });

  it('anuncia o início de um run em segundo plano com a origem externa', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_STARTED_EVENT)?.(runEvent()));

    expect(announceRequestSpy).toHaveBeenCalledTimes(1);
    const request = announceRequestSpy.mock.calls[0][0];
    expect(request.message).toBe('subAgentRuns.announce.started:Revisar PR');
    expect(request.eventType).toBe('progress');
    // Um run em segundo plano não tem aba dona: marcá-lo como externo impede que
    // a política de aba inativa silencie o aviso.
    expect(request.origin).toMatchObject({ isExternal: true, conversationId: 'conv-1' });
  });

  it('anuncia a conclusão como evento de completion, para esperar a leitura em curso', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_FINISHED_EVENT)?.(runEvent({ status: 'succeeded' })));

    const request = announceRequestSpy.mock.calls[0][0];
    expect(request.message).toBe('subAgentRuns.announce.finished.succeeded:Revisar PR');
    expect(request.eventType).toBe('completion');
  });

  it('anuncia falha e expiração sem interromper a leitura (também completion)', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_FINISHED_EVENT)?.(runEvent({ status: 'failed' })));
    act(() => handlers.get(SUBAGENT_RUN_FINISHED_EVENT)?.(runEvent({ status: 'timed_out' })));

    expect(announceRequestSpy.mock.calls[0][0].message).toBe('subAgentRuns.announce.finished.failed:Revisar PR');
    expect(announceRequestSpy.mock.calls[1][0].message).toBe('subAgentRuns.announce.finished.timed_out:Revisar PR');
    expect(announceRequestSpy.mock.calls.every((call) => call[0].eventType === 'completion')).toBe(true);
  });

  it('não anuncia runs síncronos: o resultado já aparece no turno do pai', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_STARTED_EVENT)?.(runEvent({ background: false })));
    act(() => handlers.get(SUBAGENT_RUN_FINISHED_EVENT)?.(runEvent({ background: false, status: 'succeeded' })));

    expect(announceRequestSpy).not.toHaveBeenCalled();
    // Mas a lista continua sendo atualizada — o run existe e aparece no painel.
    expect(fetchRunsSpy).toHaveBeenCalledTimes(2);
  });

  it('atualiza a lista de runs a cada evento', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_STARTED_EVENT)?.(runEvent()));
    act(() => handlers.get(SUBAGENT_RUN_FINISHED_EVENT)?.(runEvent({ status: 'succeeded' })));

    expect(fetchRunsSpy).toHaveBeenCalledTimes(2);
  });

  it('ignora payload sem runId', () => {
    renderHook(() => useSubAgentRunEvents());
    act(() => handlers.get(SUBAGENT_RUN_STARTED_EVENT)?.({}));

    expect(fetchRunsSpy).not.toHaveBeenCalled();
    expect(announceRequestSpy).not.toHaveBeenCalled();
  });
});
