import { beforeEach, describe, expect, it, vi } from 'vitest';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  COMMAND_DECK_FEEDBACK_EVENT,
  COMMAND_DECK_STATE_EVENT,
  subscribeCommandDeckFeedback,
  type CommandDeckFeedbackContext,
  type CommandDeckFeedbackEvent,
} from './subscribeCommandDeckFeedback';

vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn() }));

const eventsOn = vi.mocked(EventsOn);
const now = 1_000_000;
const context: CommandDeckFeedbackContext = {
  authenticated: true,
  owner: { userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' },
  workspaceId: 'workspace-a',
  generation: 'generation-a',
  paletteDeadline: now + 5_000,
};

function event(overrides: Partial<CommandDeckFeedbackEvent> = {}): CommandDeckFeedbackEvent {
  return {
    invocationId: 'invocation-a',
    state: 'waiting',
    title: 'Open chat',
    userId: 'user-a',
    sessionId: 'session-a',
    workspaceId: 'workspace-a',
    generation: 'generation-a',
    expiresAt: now + 3_000,
    ...overrides,
  };
}

describe('subscribeCommandDeckFeedback', () => {
  let emit: (payload: unknown) => void;
  let disposeEvent: ReturnType<typeof vi.fn>;
  let emitState: (value: unknown) => void;
  let getContext: () => CommandDeckFeedbackContext;
  let announce: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    emit = () => undefined;
    disposeEvent = vi.fn();
    eventsOn.mockImplementation((name, listener) => {
      expect([COMMAND_DECK_FEEDBACK_EVENT, COMMAND_DECK_STATE_EVENT]).toContain(name);
      if (name === COMMAND_DECK_FEEDBACK_EVENT) emit = listener;
      else emitState = listener;
      return disposeEvent;
    });
    getContext = () => context;
    announce = vi.fn();
  });

  function subscribe(options: Partial<Parameters<typeof subscribeCommandDeckFeedback>[0]> = {}) {
    return subscribeCommandDeckFeedback({
      announce,
      getContext,
      translate: (state, title) => `${state}: ${title}`,
      now: () => now,
      ...options,
    });
  }

  it('anuncia estados válidos uma vez e ignora duplicatas e reordenação', () => {
    const dispose = subscribe();

    emit(event({ state: 'waiting' }));
    emit(event({ state: 'waiting' }));
    emit(event({ state: 'running' }));
    emit(event({ state: 'waiting' }));
    emit(event({ state: 'succeeded' }));
    emit(event({ state: 'failed' }));

    expect(announce.mock.calls).toEqual([
      ['waiting: Open chat'],
      ['running: Open chat'],
      ['succeeded: Open chat'],
    ]);
    dispose();
  });

  it('permite um estado inicial sem waiting, mas nunca mais de um terminal', () => {
    subscribe();

    emit(event({ state: 'running', invocationId: 'invocation-b' }));
    emit(event({ state: 'outcome_unknown', invocationId: 'invocation-b' }));
    emit(event({ state: 'running', invocationId: 'invocation-b' }));

    expect(announce.mock.calls).toEqual([
      ['running: Open chat'],
      ['outcome_unknown: Open chat'],
    ]);
  });

  it('rejeita frames malformados, estrangeiros, expirados e futuros demais', () => {
    subscribe();

    emit(null);
    emit(event({ userId: 'user-b' }));
    emit(event({ state: 'not-a-state' as CommandDeckFeedbackEvent['state'] }));
    emit(event({ title: '  ' }));
    emit(event({ title: 'Open\nchat' }));
    emit(event({ expiresAt: now - 1 }));
    emit(event({ expiresAt: now + 10_001 }));
    emit(event({ generation: 'stale-generation' }));

    expect(announce).not.toHaveBeenCalled();
  });

  it('aceita o título normalizado até 512 unidades UTF-16 e rejeita o seguinte', () => {
    subscribe();
    const maximumTitle = '😀'.repeat(256); // 256 code points, 512 UTF-16 units.
    emit(event({ title: maximumTitle }));
    emit(event({ invocationId: 'invocation-b', title: '😀'.repeat(257) }));

    expect(announce).toHaveBeenCalledOnce();
    expect(announce).toHaveBeenCalledWith(`waiting: ${maximumTitle}`);
  });

  it('aplica foco quando solicitado e bloqueia contexto não autenticado', () => {
    const hasFocus = vi.fn(() => false);
    subscribe({ requireFocus: true, hasFocus });
    emit(event());
    expect(announce).not.toHaveBeenCalled();
    expect(hasFocus).toHaveBeenCalledOnce();

    getContext = () => ({ ...context, authenticated: false });
    emit(event({ invocationId: 'invocation-b' }));
    expect(announce).not.toHaveBeenCalled();
  });

  it('descarta eventos depois do dispose e limpa o listener', () => {
    const dispose = subscribe();
    dispose();
    emit(event());

    expect(disposeEvent).toHaveBeenCalledTimes(2);
    expect(announce).not.toHaveBeenCalled();
  });

  it('limita o histórico a 64 invocações', () => {
    subscribe();
    for (let i = 0; i < 65; i += 1) {
      emit(event({ invocationId: `invocation-${i}` }));
    }
    emit(event({ invocationId: 'invocation-0', state: 'running' }));

    expect(announce).toHaveBeenCalledTimes(66);
  });

  it('separa estado persistente de resultado de execução e mantém os guards', () => {
    subscribe();
    emit(event({ state: 'on' }));
    expect(announce).not.toHaveBeenCalled();
    emitState({ ...event({ state: 'on' }), eventId: 'change-1' });
    emitState({ ...event({ state: 'on' }), eventId: 'change-1' });
    expect(announce).toHaveBeenCalledExactlyOnceWith('on: Open chat');
    emitState({ ...event({ state: 'off', sessionId: 'other' }), eventId: 'change-2' });
    emitState({ ...event({ state: 'succeeded' }), eventId: 'change-2' });
    emitState({ ...event({ state: 'off', expiresAt: now }), eventId: 'change-2' });
    expect(announce).toHaveBeenCalledTimes(1);
    emitState({ ...event({ state: 'off' }), eventId: 'change-3' });
    expect(announce).toHaveBeenLastCalledWith('off: Open chat');
  });
});
