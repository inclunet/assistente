import { describe, expect, it, vi } from 'vitest';
import {
  acceptCommandBridgeResult,
  cancelCommandBridgeInvocation,
  dispatchCommandBridgeInput,
  dispatchCommandBridgeInvocation,
  dispatchCommandBridgeLifecycle,
} from './commandBridgeWails';
import type { CommandBridgeOwner, CommandInvocation, CommandResult } from './commandBridge';

vi.mock('./waitForWailsBridge', () => ({
  waitForWailsBridge: vi.fn(async () => undefined),
}));

const owner: CommandBridgeOwner = { userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };
const invocation: CommandInvocation = {
  sessionId: owner.sessionId,
  invocationId: '01900000-0000-7000-8000-000000000001',
  commandId: 'command.a',
  generation: '1',
  capabilityId: 'cap-a',
  ownership: 'local',
  source: 'ui.action',
};
const result: CommandResult = { ...invocation, owner, status: 'succeeded' };

function target() {
  const app = {
    CommandBridgeInvoke: vi.fn(async (value: CommandInvocation) => ({ invocationId: value.invocationId, accepted: true })),
    CommandBridgeInput: vi.fn(async (value: { invocation: CommandInvocation }) => ({ invocationId: value.invocation.invocationId, accepted: true })),
    CommandBridgeAcceptResult: vi.fn(async (value: CommandResult) => ({ invocationId: value.invocationId, accepted: true })),
    CommandBridgeCancel: vi.fn(async (value: { invocationId: string }) => ({ invocationId: value.invocationId, accepted: true })),
    CommandBridgeLifecycle: vi.fn(async () => undefined),
  };
  const windowTarget = {
    go: {
      app: {
        App: app,
      },
    },
  } as unknown as Window;
  return { app, windowTarget };
}

describe('commandBridgeWails', () => {
  it('encaminha dispatch, input, resultado, cancelamento e lifecycle para o App', async () => {
    const { app, windowTarget } = target();
    await expect(dispatchCommandBridgeInvocation(invocation, owner, { target: windowTarget })).resolves.toMatchObject({ accepted: true, invocationId: invocation.invocationId });
    await expect(dispatchCommandBridgeInput({ sessionId: owner.sessionId, source: 'keyboard', key: 'Ctrl+N', generation: '1', kind: 'down', invocation, owner }, { target: windowTarget })).resolves.toMatchObject({ accepted: true });
    await expect(acceptCommandBridgeResult(result, { target: windowTarget })).resolves.toMatchObject({ accepted: true });
    await expect(cancelCommandBridgeInvocation({ sessionId: owner.sessionId, invocationId: invocation.invocationId, generation: '1', capabilityId: invocation.capabilityId, owner }, { target: windowTarget })).resolves.toMatchObject({ accepted: true });
    await expect(dispatchCommandBridgeLifecycle({ kind: 'blur', sessionId: owner.sessionId, generation: '1' }, { target: windowTarget })).resolves.toBeUndefined();

    expect(app.CommandBridgeInvoke).toHaveBeenCalledWith(invocation, owner);
    expect(app.CommandBridgeInput).toHaveBeenCalledTimes(1);
    expect(app.CommandBridgeAcceptResult).toHaveBeenCalledWith(result);
    expect(app.CommandBridgeCancel).toHaveBeenCalledTimes(1);
    expect(app.CommandBridgeLifecycle).toHaveBeenCalledWith({ kind: 'blur', sessionId: owner.sessionId, generation: '1' });
  });

  it('falha fechado quando a API Wails ainda não publicou a ponte', async () => {
    await expect(dispatchCommandBridgeInvocation(invocation, owner, { target: { go: { app: { App: {} } } } as unknown as Window })).rejects.toThrow('Command bridge Wails API is not available');
  });
});
