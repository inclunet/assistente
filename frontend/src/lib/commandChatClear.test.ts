import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createStore } from 'zustand/vanilla';
import { CHAT_CLEAR_COMMAND, CHAT_CLEAR_EVENT, captureChatClearTarget, executeChatClear, registerChatClearSurface, requestChatClear } from './commandChatClear';
import { getModalRegistrySnapshot, registerOpenModal, unregisterOpenModal } from './modalRegistry';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandTakeResponse } from './commandUIExecution';
import { acquireCommandFocusTracking } from './commandFocusContext';

const cleanup: Array<() => void> = [];
function modal(id: string) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  document.body.append(overlay);
  registerOpenModal(id);
  const close = () => { unregisterOpenModal(id); overlay.remove(); };
  cleanup.push(close);
  return close;
}
function surface(instanceId = 'chat-a', modalId?: string) {
  const root = document.createElement('div');
  document.body.append(root);
  const context = createStore(() => ({ owner: 'alice', session: 'session-a', route: '/', conversation: 42, active: true, busy: false }));
  const subscribers = new Set<() => void>();
  const succeeded = vi.fn();
  const unregister = registerChatClearSurface({
    root, instanceId, modalId,
    isCurrent: () => {
      const state = context.getState();
      return state.owner === 'alice' && state.session === 'session-a' && state.conversation === 42 && state.active;
    },
    canStart: () => !context.getState().busy && getModalRegistrySnapshot().topID === (modalId ?? null),
    subscribe: changed => {
      subscribers.add(changed);
      const off = context.subscribe(changed);
      return () => { subscribers.delete(changed); off(); };
    },
    succeeded,
  });
  cleanup.push(unregister, () => root.remove());
  const capture = (expected?: string) => {
    const target = captureChatClearTarget(() => context.getState().route, expected);
    if (target) cleanup.push(() => target.dispose());
    return target;
  };
  return { root, context, succeeded, unregister, subscribers, capture };
}
const reservation = { ticket: 'ticket-a', invocationId: 'invocation-a', commandId: CHAT_CLEAR_COMMAND };
const receipt = { ...reservation, handoffId: 'handoff-a' };
function port() {
  return {
    beginUICommand: vi.fn<CommandContextualBackendPort['beginUICommand']>().mockResolvedValue(reservation),
    takeUICommand: vi.fn<CommandContextualBackendPort['takeUICommand']>().mockResolvedValue(receipt),
    commitBackendCommand: vi.fn<CommandContextualBackendPort['commitBackendCommand']>().mockResolvedValue(undefined),
    getUICommandResult: vi.fn<CommandContextualBackendPort['getUICommandResult']>().mockResolvedValue({ invocationId: reservation.invocationId, status: 'succeeded' }),
    completeUICommand: vi.fn<CommandContextualBackendPort['completeUICommand']>().mockResolvedValue(undefined),
    cancelUICommand: vi.fn<CommandContextualBackendPort['cancelUICommand']>().mockResolvedValue(undefined),
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}
beforeEach(() => { vi.spyOn(document, 'hasFocus').mockReturnValue(true); });
afterEach(() => { cleanup.splice(0).reverse().forEach(fn => fn()); vi.useRealTimers(); vi.restoreAllMocks(); });

describe('chat clear registry real', () => {
  it('composição real bloqueia captura e commit de lease anterior', async () => {
    const source = surface();
    cleanup.push(acquireCommandFocusTracking(document));
    const input = document.createElement('textarea');
    document.body.append(input);
    cleanup.push(() => input.remove());
    input.focus();
    const target = source.capture()!;
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(source.capture()).toBeUndefined();
    expect(target.canCommit()).toBe(false);
    const backend = port();
    expect(await executeChatClear(backend, target)).toBe('cancelled');
    expect(backend.commitBackendCommand).not.toHaveBeenCalled();
  });
  it.each(['ticket', 'invocationId'] as const)('recusa reserva com %s vazio antes de Take', async field => {
    const source = surface(); const backend = port();
    backend.beginUICommand.mockResolvedValue({ ...reservation, [field]: '' });
    expect(await executeChatClear(backend, source.capture()!)).toBe('cancelled');
    expect(backend.takeUICommand).not.toHaveBeenCalled();
    expect(backend.commitBackendCommand).not.toHaveBeenCalled();
  });
  it('falha da apresentação não converte sucesso confirmado em outcome_unknown', async () => {
    const source = surface(); const backend = port();
    source.succeeded.mockImplementation(() => { throw new Error('announcer failed'); });
    expect(await executeChatClear(backend, source.capture()!)).toBe('succeeded');
    expect(backend.commitBackendCommand).toHaveBeenCalledOnce();
    expect(backend.completeUICommand).not.toHaveBeenCalled();
  });
  it('evento preserva instanceId e captura somente a instância esperada', () => {
    const source = surface();
    const listener = vi.fn((event: Event) => {
      expect((event as CustomEvent).detail).toEqual({ instanceId: 'chat-a' });
      expect(source.capture((event as CustomEvent<{ instanceId: string }>).detail.instanceId)).toBeDefined();
    });
    window.addEventListener(CHAT_CLEAR_EVENT, listener);
    try { requestChatClear('chat-a'); expect(listener).toHaveBeenCalledTimes(1); }
    finally { window.removeEventListener(CHAT_CLEAR_EVENT, listener); }
    expect(source.capture('other-instance')).toBeUndefined();
  });
  it('recusa zero ou múltiplas superfícies elegíveis e aceita apenas a ativa', () => {
    expect(captureChatClearTarget(() => '/')).toBeUndefined();
    const a = surface(); const b = surface('chat-b');
    expect(a.capture()).toBeUndefined();
    b.context.setState({ active: false });
    expect(a.capture()).toBeDefined();
  });
  it.each(['owner', 'session', 'route', 'conversation', 'active'] as const)('invalida ABA de %s pelas subscriptions reais', field => {
    const s = surface(); const target = s.capture()!;
    const original = s.context.getState();
    const changed = { ...original, [field]: field === 'conversation' ? 99 : field === 'active' ? false : 'other' };
    s.context.setState(changed); s.context.setState(original);
    expect(target.isCurrent()).toBe(false);
    expect(target.canCommit()).toBe(false);
    target.succeeded(); expect(s.succeeded).not.toHaveBeenCalled();
  });
  it('dispose é imediato e idempotente e remove subscription', () => {
    const s = surface(); const target = s.capture()!;
    expect(s.subscribers.size).toBe(1);
    target.dispose(); target.dispose();
    expect(s.subscribers.size).toBe(0);
    expect(target.isCurrent()).toBe(false);
  });
  it('unregister torna a lease inválida', () => {
    const s = surface(); const target = s.capture()!;
    s.unregister(); expect(target.isCurrent()).toBe(false);
  });
  it('chat modal mantém a origem sob decisão e volta a permitir commit depois do fechamento', () => {
    modal('chat-modal'); const s = surface('chat-a', 'chat-modal'); const target = s.capture()!;
    const closeDecision = modal('decision');
    expect(target.isCurrent()).toBe(true); expect(target.canCommit()).toBe(false);
    closeDecision();
    expect(target.isCurrent()).toBe(true); expect(target.canCommit()).toBe(true);
  });
  it('fechar modal de origem invalida mesmo com decisão ainda aberta', () => {
    const closeChat = modal('chat-modal'); const s = surface('chat-a', 'chat-modal'); const target = s.capture()!;
    modal('decision'); closeChat();
    expect(target.isCurrent()).toBe(false);
  });
});

describe('executeChatClear com registry real e transporte controlado', () => {
  it('sucesso confirmado aplica uma vez e replay da lease não inicia outra operação', async () => {
    const s = surface(); const target = s.capture()!; const p = port();
    expect(await executeChatClear(p, target)).toBe('succeeded');
    expect(p.beginUICommand).toHaveBeenCalledExactlyOnceWith(CHAT_CLEAR_COMMAND);
    expect(p.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a');
    expect(s.succeeded).toHaveBeenCalledTimes(1);
    expect(await executeChatClear(p, target)).toBe('cancelled');
    expect(p.beginUICommand).toHaveBeenCalledTimes(1);
    expect(p.completeUICommand).not.toHaveBeenCalled();
    expect(s.subscribers.size).toBe(0);
  });
  it('canCommit falso cancela handoff sem commit', async () => {
    vi.useFakeTimers(); const s = surface(); const target = s.capture()!; const p = port();
    s.context.setState({ busy: true });
    const result = executeChatClear(p, target);
    await vi.runAllTimersAsync();
    expect(await result).toBe('cancelled');
    expect(p.commitBackendCommand).not.toHaveBeenCalled();
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a', 'cancelled');
  });
  it('reserva de outro comando cancela antes de Take', async () => {
    const s = surface(); const p = port(); p.beginUICommand.mockResolvedValue({ ...reservation, commandId: 'chat.other' });
    expect(await executeChatClear(p, s.capture()!)).toBe('cancelled');
    expect(p.takeUICommand).not.toHaveBeenCalled(); expect(p.commitBackendCommand).not.toHaveBeenCalled();
    expect(p.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-a');
  });
  it.each([
    { commandId: 'chat.other' }, { invocationId: 'other-invocation' }, { ticket: 'other-ticket' }, { handoffId: '' },
  ])('recusa receipt incompatível %j', async mismatch => {
    const s = surface(); const p = port(); p.takeUICommand.mockResolvedValue({ ...receipt, ...mismatch });
    expect(await executeChatClear(p, s.capture()!)).toBe('cancelled');
    expect(p.commitBackendCommand).not.toHaveBeenCalled(); expect(s.succeeded).not.toHaveBeenCalled();
  });
  it('dispose durante Take cancela antes do commit', async () => {
    const s = surface(); const target = s.capture()!; const p = port(); const pending = deferred<UICommandTakeResponse>();
    p.takeUICommand.mockReturnValue(pending.promise);
    const result = executeChatClear(p, target); await Promise.resolve();
    expect(p.takeUICommand).toHaveBeenCalledTimes(1);
    target.dispose(); pending.resolve(receipt);
    expect(await result).toBe('cancelled'); expect(p.commitBackendCommand).not.toHaveBeenCalled();
    expect(p.completeUICommand).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a', 'cancelled');
  });
  it('recusa cancelamento backend em Take sem Commit', async () => {
    const s = surface(); const p = port(); p.takeUICommand.mockRejectedValue(new Error('backend-decision-cancelled'));
    expect(await executeChatClear(p, s.capture()!)).toBe('cancelled');
    expect(p.commitBackendCommand).not.toHaveBeenCalled(); expect(p.cancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket-a');
  });
  it('commit com erro de transporte retorna outcome_unknown sem retry ou complete', async () => {
    const s = surface(); const p = port(); p.commitBackendCommand.mockRejectedValue(new Error('transport lost after write'));
    expect(await executeChatClear(p, s.capture()!)).toBe('outcome_unknown');
    expect(p.commitBackendCommand).toHaveBeenCalledTimes(1); expect(p.getUICommandResult).not.toHaveBeenCalled();
    expect(p.completeUICommand).not.toHaveBeenCalled(); expect(p.cancelUICommand).not.toHaveBeenCalled();
    expect(s.succeeded).not.toHaveBeenCalled();
  });
  it.each(['failed', 'cancelled', 'denied'] as const)('resultado backend %s não aplica sucesso', async status => {
    const s = surface(); const p = port(); p.getUICommandResult.mockResolvedValue({ invocationId: reservation.invocationId, status });
    expect(await executeChatClear(p, s.capture()!)).toBe(status); expect(s.succeeded).not.toHaveBeenCalled();
  });
  it('resultado de outra invocação é incerto e não aplica sucesso', async () => {
    const s = surface(); const p = port(); p.getUICommandResult.mockResolvedValue({ invocationId: 'other', status: 'succeeded' });
    expect(await executeChatClear(p, s.capture()!)).toBe('outcome_unknown'); expect(s.succeeded).not.toHaveBeenCalled();
  });
  it('espera fechamento da decisão sobre chat modal antes do commit', async () => {
    vi.useFakeTimers(); modal('chat-modal'); const s = surface('chat-a', 'chat-modal'); const target = s.capture()!; const p = port();
    p.takeUICommand.mockImplementation(async () => {
      const close = modal('decision'); window.setTimeout(close, 40); return receipt;
    });
    const result = executeChatClear(p, target);
    await vi.advanceTimersByTimeAsync(20);
    expect(target.isCurrent()).toBe(true); expect(p.commitBackendCommand).not.toHaveBeenCalled();
    await vi.runAllTimersAsync();
    expect(await result).toBe('succeeded'); expect(s.succeeded).toHaveBeenCalledTimes(1);
    expect(getModalRegistrySnapshot().topID).toBe('chat-modal');
  });
});
