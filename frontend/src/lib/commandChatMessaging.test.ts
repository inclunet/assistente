import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createStore } from 'zustand/vanilla';
import { captureChatMessagingTarget, registerChatMessagingSurface, executeChatMessaging, type ChatMessagingCommandID } from './commandChatMessaging';
import { getModalRegistrySnapshot, registerOpenModal, unregisterOpenModal, updateOpenModalScope } from './modalRegistry';

const cleanups: Array<() => void> = [];
function setup(id = 'a') {
  const root = document.createElement('div'); document.body.append(root);
  const state = createStore(() => ({ owner: 'owner', session: 'session', conversation: 'conversation', route: '/', draft: 'draft', active: true }));
  const execute = vi.fn(async () => {});
  const disposed = vi.fn();
  const off = registerChatMessagingSurface({ root, instanceId: id,
    isCurrent: () => { const s = state.getState(); return s.owner === 'owner' && s.session === 'session' && s.conversation === 'conversation' && s.active; },
    canStart: () => true,
    subscribe: changed => state.subscribe(changed),
    prepare: () => {
      const draft = state.getState().draft;
      return { isCurrent: () => state.getState().draft === draft, canCommit: () => true, execute, dispose: disposed };
    },
  });
  cleanups.push(() => root.remove(), off);
  const capture = (command: ChatMessagingCommandID = 'chat.message.send', expected?: string) => {
    const target = captureChatMessagingTarget(() => state.getState().route, command, expected);
    if (target) cleanups.push(() => target.dispose());
    return target;
  };
  return { root, state, capture, execute, disposed, off };
}
beforeEach(() => { vi.spyOn(document, 'hasFocus').mockReturnValue(true); });
afterEach(() => { cleanups.splice(0).reverse().forEach(off => off()); vi.useRealTimers(); vi.restoreAllMocks(); });
describe('chat messaging registry real', () => {
  it.each(['closed', 'replaced', 'disposed', 'timeout'] as const)('exclusão aguarda fechamento da decisão sem aceitar contexto inválido: %s', async outcome => {
    vi.useFakeTimers();
    const root = document.createElement('div'); document.body.append(root);
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    const execute = vi.fn(async () => {});
    const off = registerChatMessagingSurface({ root, instanceId: 'delayed-delete', isCurrent: () => true, canStart: () => true,
      subscribe: () => () => {}, prepare: () => ({
        isCurrent: () => true, canCommit: () => getModalRegistrySnapshot().ids.length === 0,
        prepareAdmission: async () => {
          document.body.append(overlay);
          registerOpenModal('delayed-dialog', { dialogId: 'decision', kind: 'decision', generation: '1', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] });
          root.setAttribute('inert', '');
        }, execute, dispose: () => {},
      }),
    });
    cleanups.push(() => root.remove(), off, () => { unregisterOpenModal('delayed-dialog'); overlay.remove(); });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.delete')!;
    const reservation = { commandId: target.commandId, ticket: 't', invocationId: 'i' };
    const port = { beginUICommand: vi.fn(async () => reservation),
      takeUICommand: vi.fn(async () => ({ ...reservation, handoffId: 'h' })),
      completeUICommand: vi.fn(async () => {}), cancelUICommand: vi.fn(async () => {}),
      getUICommandResult: vi.fn(async () => ({ invocationId: 'i', status: 'succeeded' as const })), commitBackendCommand: vi.fn(async () => {}),
    };
    const running = executeChatMessaging(port, target);
    await vi.advanceTimersByTimeAsync(300);
    expect(execute).not.toHaveBeenCalled();
    if (outcome === 'replaced') updateOpenModalScope('delayed-dialog', { dialogId: 'another-decision', kind: 'decision', generation: '2', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] });
    if (outcome === 'disposed') target.dispose();
    if (outcome === 'timeout') await vi.advanceTimersByTimeAsync(30_000);
    unregisterOpenModal('delayed-dialog'); overlay.remove(); root.removeAttribute('inert');
    expect(await running).toBe(outcome === 'closed' ? 'succeeded' : outcome === 'timeout' ? 'failed' : 'cancelled');
    expect(execute).toHaveBeenCalledTimes(outcome === 'closed' ? 1 : 0);
    expect(vi.getTimerCount()).toBe(0);
  });
  it.each([true, false])('exclusão tolera apenas decisão backend durante admissão: decision=%s', async decision => {
    const root = document.createElement('div'); document.body.append(root);
    const overlay = document.createElement('div'); overlay.className = 'modal-overlay';
    const execute = vi.fn(async () => {});
    const off = registerChatMessagingSurface({ root, instanceId: 'delete', isCurrent: () => true, canStart: () => true,
      subscribe: () => () => {}, prepare: () => ({
        isCurrent: () => true, canCommit: () => getModalRegistrySnapshot().ids.length === 0,
        prepareAdmission: async () => {
          document.body.append(overlay);
          registerOpenModal('dialog', decision ? { dialogId: 'dialog', kind: 'decision', generation: '1', allowedCommandIds: ['decision.respond'], allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] } : undefined);
          root.setAttribute('inert', '');
        }, execute, dispose: () => {},
      }),
    });
    cleanups.push(() => root.remove(), off, () => { unregisterOpenModal('dialog'); overlay.remove(); });
    const target = captureChatMessagingTarget(() => '/', 'chat.message.delete')!;
    const reservation = { commandId: target.commandId, ticket: 't', invocationId: 'i' };
    const port = { beginUICommand: vi.fn(async () => reservation),
      takeUICommand: vi.fn(async () => { unregisterOpenModal('dialog'); overlay.remove(); root.removeAttribute('inert'); return { ...reservation, handoffId: 'h' }; }),
      completeUICommand: vi.fn(async () => {}), cancelUICommand: vi.fn(async () => {}),
      getUICommandResult: vi.fn(async () => ({ invocationId: 'i', status: 'succeeded' as const })), commitBackendCommand: vi.fn(async () => {}),
    };
    expect(await executeChatMessaging(port, target)).toBe(decision ? 'succeeded' : 'cancelled');
    expect(execute).toHaveBeenCalledTimes(decision ? 1 : 0);
    expect(port.takeUICommand).toHaveBeenCalledTimes(decision ? 1 : 0);
  });
  it('resolve superfície única antes de comparar instanceId', () => {
    const a = setup(); const b = setup('b');
    expect(a.capture('chat.message.send', 'a')).toBeUndefined();
    b.state.setState({ active: false });
    expect(a.capture('chat.message.send', 'wrong')).toBeUndefined();
    expect(a.capture('chat.message.send', 'a')).toBeDefined();
  });
  it.each(['owner', 'session', 'conversation', 'route', 'draft'] as const)('subscription invalida ABA de %s', field => {
    const source = setup(); const target = source.capture()!;
    const original = source.state.getState()[field];
    source.state.setState({ [field]: 'replacement' }); source.state.setState({ [field]: original });
    expect(target.isCurrent()).toBe(false);
  });
  it('modal superior aberto e fechado durante Take não recupera lease', async () => {
    const source = setup(); const target = source.capture()!;
    const reservation = { commandId: target.commandId, ticket: 't', invocationId: 'i' };
    const port = { beginUICommand: vi.fn(async () => reservation),
      takeUICommand: vi.fn(async () => { registerOpenModal('decision'); unregisterOpenModal('decision'); return { ...reservation, handoffId: 'h' }; }),
      completeUICommand: vi.fn(async () => {}), cancelUICommand: vi.fn(async () => {}),
      getUICommandResult: vi.fn(async () => ({ invocationId: 'i', status: 'succeeded' as const })), commitBackendCommand: vi.fn(async () => {}),
    };
    expect(await executeChatMessaging(port, target)).toBe('cancelled');
    expect(source.execute).not.toHaveBeenCalled();
    expect(port.completeUICommand).toHaveBeenCalledExactlyOnceWith('t', 'h', 'cancelled');
  });
  it('unregister dispõe todas leases imediatamente e idempotentemente', () => {
    const source = setup(); const a = source.capture()!; const b = source.capture('chat.message.retry')!;
    source.off(); source.off(); a.dispose(); b.dispose();
    expect(source.disposed).toHaveBeenCalledTimes(2);
    expect(a.isCurrent()).toBe(false); expect(b.isCurrent()).toBe(false);
  });
  it('invalida invisibilidade e desconexão sem retarget', () => {
    const source = setup(); const target = source.capture()!;
    source.root.hidden = true; expect(target.isCurrent()).toBe(false);
    source.root.hidden = false; expect(target.isCurrent()).toBe(false);
    const next = source.capture()!; source.root.remove(); expect(next.isCurrent()).toBe(false);
  });
});
