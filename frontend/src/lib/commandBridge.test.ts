import { describe, expect, it, vi } from 'vitest';
import {
  CommandBridgeError,
  createCommandBridge,
  type CommandBridgeOwner,
  type CommandInvocation,
  type CommandResult,
  type CommandSource,
  type CommandSession,
} from './commandBridge';

const owner: CommandBridgeOwner = { userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };
const session: CommandSession = { id: owner.sessionId, generation: '1', owner };
let nextInvocationNumber = 1;
const invocationIDs = new Map<string, string>();
const testUUID7 = (label: string): string => {
  const existing = invocationIDs.get(label);
  if (existing) return existing;
  const value = `01900000-0000-7000-8000-${String(nextInvocationNumber).padStart(12, '0')}`;
  nextInvocationNumber += 1;
  invocationIDs.set(label, value);
  return value;
};
const invocation = (id = 'inv-a', ownership: 'local' | 'global' = 'local'): CommandInvocation => ({
  sessionId: session.id,
  invocationId: testUUID7(id),
  commandId: 'command.a',
  generation: '1',
  capabilityId: 'cap-a',
  ownership,
  source: 'ui.action',
});
const resultFor = (value: CommandInvocation, resultOwner = owner): CommandResult => ({
  ...value,
  owner: resultOwner,
  status: 'succeeded',
});

function fixture() {
  const dispatch = vi.fn(async (value: CommandInvocation) => ({ invocationId: value.invocationId, accepted: true }));
  const cancel = vi.fn(async () => undefined);
  const bridge = createCommandBridge({
    port: { dispatch, cancel },
    capabilities: [{ id: 'cap-a', commandId: 'command.a', generation: '1', owner }],
  });
  bridge.openSession(session);
  return { bridge, dispatch, cancel };
}

describe('command bridge', () => {
  it('valida capability/owner, faz ack e rejeita replay', async () => {
    const { bridge, dispatch } = fixture();
    await expect(bridge.invoke(invocation(), owner)).resolves.toMatchObject({ accepted: true, invocationId: testUUID7('inv-a') });
    await expect(bridge.invoke(invocation(), owner)).rejects.toMatchObject({ code: 'invocation-replay' });
    const wrongOwner = { ...owner, userId: 'user-b' };
    await expect(bridge.invoke(invocation('inv-b'), wrongOwner)).rejects.toMatchObject({ code: 'stale-generation' });
    expect(dispatch).toHaveBeenCalledTimes(1);
  });

  it('fecha Source e separa UUIDv7 de occurrence física opaca', async () => {
    const { bridge } = fixture();
    await expect(bridge.invoke({ ...invocation('bad-source'), source: 'untrusted.source' as CommandSource }, owner)).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(bridge.invoke({ ...invocation('bad-id'), invocationId: 'not-a-uuid' }, owner)).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(bridge.invoke({ ...invocation('event'), source: 'event', eventId: testUUID7('event-source') }, owner)).resolves.toMatchObject({ accepted: true });
    await expect(bridge.invoke({ ...invocation('missing-event'), source: 'event' }, owner)).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(bridge.invoke({ ...invocation('opaque-occurrence'), occurrenceId: 'deck-A/button-7/v2' }, owner)).resolves.toMatchObject({ accepted: true });
  });

  it('fecha resultado apenas com identidade exata e libera local/global', async () => {
    const { bridge } = fixture();
    await bridge.invoke(invocation(), owner);
    await expect(Promise.resolve().then(() => bridge.acceptResult(resultFor({ ...invocation(), generation: '2' })))).rejects.toMatchObject({ code: 'invalid-request' });
    expect(() => bridge.acceptResult(resultFor(invocation()))).not.toThrow();
    expect(() => bridge.acceptResult(resultFor(invocation()))).toThrowError(new CommandBridgeError('unknown-invocation'));
    await bridge.invoke(invocation('inv-global', 'global'), owner);
    await expect(bridge.invoke(invocation('inv-third'), owner)).resolves.toMatchObject({ accepted: true });
    const physical = { ...invocation('inv-physical'), occurrenceId: '8:keyboard6:Ctrl+N' };
    await bridge.invoke(physical, owner);
    await expect(bridge.invoke({ ...physical, invocationId: testUUID7('inv-physical-global'), ownership: 'global' }, owner)).rejects.toMatchObject({ code: 'ownership-conflict' });
  });

  it('mantém ownership até cancel confirmado e cancela na mudança de geração', async () => {
    const { bridge, cancel } = fixture();
    await bridge.invoke(invocation(), owner);
    cancel.mockRejectedValueOnce(new Error('transport unavailable'));
    await expect(bridge.cancel({ sessionId: session.id, invocationId: testUUID7('inv-a'), generation: '1', capabilityId: 'cap-a', owner })).rejects.toThrow('transport unavailable');
    await bridge.cancel({ sessionId: session.id, invocationId: testUUID7('inv-a'), generation: '1', capabilityId: 'cap-a', owner });
    await bridge.invoke(invocation('inv-b'), owner);
    await bridge.advanceGeneration(session.id, '2');
    expect(cancel).toHaveBeenCalledTimes(3);
    await expect(bridge.invoke({ ...invocation('inv-c'), generation: '1' }, owner)).rejects.toMatchObject({ code: 'stale-generation' });
  });

  it('normaliza repeat/release/blur e lifecycle lock fecha novas entradas', async () => {
    const { bridge } = fixture();
    const occurrenceId = '8:keyboard6:Ctrl+N';
    const first = { ...invocation(), occurrenceId };
    await expect(bridge.input({ sessionId: session.id, source: 'keyboard', key: 'Ctrl+N', generation: '1', kind: 'down', invocation: first, owner })).resolves.toMatchObject({ accepted: true });
    await expect(bridge.input({ sessionId: session.id, source: 'keyboard', key: 'Ctrl+N', generation: '1', kind: 'down', repeat: true, invocation: first, owner })).resolves.toMatchObject({ accepted: false });
    await expect(bridge.input({ sessionId: session.id, source: 'keyboard', key: 'Ctrl+N', generation: '1', kind: 'up', invocation: first, owner })).resolves.toMatchObject({ accepted: false });
    bridge.acceptResult(resultFor(first));
    await bridge.lifecycle({ kind: 'blur', sessionId: session.id, generation: '1' });
    const second = { ...invocation('inv-b'), occurrenceId };
    await bridge.input({ sessionId: session.id, source: 'keyboard', key: 'Ctrl+N', generation: '1', kind: 'down', invocation: second, owner });
    bridge.acceptResult(resultFor(second));
    await bridge.lifecycle({ kind: 'lock', sessionId: session.id, generation: '1' });
    await expect(bridge.invoke(invocation('inv-c'), owner)).rejects.toMatchObject({ code: 'session-unavailable' });
  });

  it('congela capability, sessão, owners e publica resultado aos listeners', async () => {
    const { bridge } = fixture();
    const seen: CommandResult[] = [];
    bridge.subscribeResult((value) => seen.push(value));
    const mutableOwner = { ...owner };
    const mutableCapability = { id: 'cap-a', commandId: 'command.a', generation: '1', owner: mutableOwner };
    const second = createCommandBridge({ port: { dispatch: async (value) => ({ invocationId: value.invocationId, accepted: true }), cancel: async () => undefined }, capabilities: [mutableCapability] });
    const mutableSession = { id: session.id, generation: '1', owner: mutableOwner };
    second.openSession(mutableSession);
    mutableOwner.userId = 'user-b';
    mutableSession.generation = '9';
    await expect(second.invoke(invocation('inv-copy'), owner)).resolves.toMatchObject({ accepted: true });
    await bridge.invoke(invocation(), owner);
    const value = resultFor(invocation());
    bridge.acceptResult(value);
    expect(seen[0]).toMatchObject({ invocationId: testUUID7('inv-a'), owner });
    expect(Object.isFrozen(seen[0])).toBe(true);
  });
});
