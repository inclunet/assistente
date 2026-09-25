import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createCommandContextualBackendExecution,
  WORKSPACE_CHAT_OPEN_COMMAND_ID,
  WORKSPACE_CREATE_COMMAND_ID,
  WORKSPACE_MUTATION_COMMAND_IDS,
  WORKSPACE_TAB_CREATE_COMMAND_IDS,
  WORKSPACE_TAB_MUTATION_COMMAND_IDS,
  isWorkspaceMutationCommand,
  isWorkspaceTabMutationCommand,
} from './commandContextualBackendExecution';
import type { CommandExecutionResult } from './commandUIExecution';

const stores = vi.hoisted(() => ({
  auth: { isAuthenticated: true, user: { userId: 'user', sessionId: 'session' } },
  workspace: { workspace: { id: 'workspace', activeTabId: 'tab', profile: '', tabs: [] } },
}));
vi.mock('../store/authStore', () => ({ useAuthStore: {
  getState: () => stores.auth, subscribe: () => () => undefined,
} }));
vi.mock('../store/workspaceStore', () => ({ useWorkspaceStore: {
  getState: () => stores.workspace, subscribe: () => () => undefined,
} }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}
const cleanups: Array<() => void> = [];
function fixture(commandID: string) {
  const reservation = { ticket: 'ticket', invocationId: 'invocation', commandId: commandID };
  const handoff = { ...reservation, handoffId: 'handoff' };
  const port = {
    beginUICommand: vi.fn(async () => reservation),
    takeUICommand: vi.fn(async () => handoff),
    completeUICommand: vi.fn(async () => undefined),
    commitBackendCommand: vi.fn(async (): Promise<void> => undefined),
    cancelUICommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async (): Promise<CommandExecutionResult> => ({ invocationId: 'invocation', status: 'succeeded' })),
  };
  const target = { isCurrent: vi.fn(() => true), prepare: vi.fn(() => true), dispose: vi.fn() };
  const prepareTarget = vi.fn(() => target);
  const execution = createCommandContextualBackendExecution(port, { prepareTarget });
  cleanups.push(execution.dispose);
  return { port, target, prepareTarget, execution, reservation, handoff };
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  const button = document.createElement('button');
  document.body.append(button);
  button.focus();
});
afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

it('isola tickets e resultados de tipos concorrentes sem cruzar submissões', async () => {
  const [chat, editor] = WORKSPACE_TAB_MUTATION_COMMAND_IDS;
  const firstCommit = deferred<void>();
  const secondCommit = deferred<void>();
  const port = {
    beginUICommand: vi.fn(async (commandId: string) => ({ ticket: commandId, invocationId: commandId, commandId })),
    takeUICommand: vi.fn(async (ticket: string) => ({ ticket, invocationId: ticket, commandId: ticket, handoffId: ticket + ':handoff' })),
    completeUICommand: vi.fn(async () => undefined),
    commitBackendCommand: vi.fn((ticket: string) => ticket === chat ? firstCommit.promise : secondCommit.promise),
    cancelUICommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async (ticket: string): Promise<CommandExecutionResult> => ({ invocationId: ticket, status: 'succeeded' })),
  };
  const execution = createCommandContextualBackendExecution(port, { prepareTarget: () => ({ isCurrent: () => true, dispose: vi.fn() }) });
  cleanups.push(execution.dispose);
  const one = execution.execute(chat);
  const two = execution.execute(editor);
  await vi.waitFor(() => expect(port.commitBackendCommand).toHaveBeenCalledTimes(2));
  expect(port.commitBackendCommand).toHaveBeenCalledWith(chat, chat + ':handoff');
  expect(port.commitBackendCommand).toHaveBeenCalledWith(editor, editor + ':handoff');
  secondCommit.resolve();
  await expect(two).resolves.toMatchObject({ invocationId: editor, status: 'succeeded' });
  expect(port.getUICommandResult).not.toHaveBeenCalledWith(chat);
  firstCommit.resolve();
  await expect(one).resolves.toMatchObject({ invocationId: chat, status: 'succeeded' });
  expect(port.completeUICommand).not.toHaveBeenCalled();
});

it('recusa navegação local no adapter contextual antes de reservar no backend', async () => {
  const f = fixture('workspace.tab.next');
  await expect(f.execution.execute('workspace.tab.next')).resolves.toMatchObject({ status: 'cancelled', errorCode: 'ui-handler-missing' });
  expect(f.port.beginUICommand).not.toHaveBeenCalled();
  expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
});

it.each(['reject', 'throw', 'stale', 'async'] as const)('cancela preparação %s sem submeter escrita nem afirmar outcome desconhecido', async reason => {
  const f = fixture('editor.mode.view');
  f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' });
  f.target.prepare.mockImplementation(() => {
    if (reason === 'throw') throw new Error('flush failed');
    if (reason === 'async') return Promise.resolve(true) as unknown as boolean;
    if (reason === 'stale') f.target.isCurrent.mockReturnValue(false);
    return reason !== 'reject';
  });
  await expect(f.execution.execute('editor.mode.view')).resolves.toMatchObject({ status: 'cancelled' });
  expect(f.target.prepare).toHaveBeenCalledOnce();
  expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
  expect(f.port.completeUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled');
  expect(f.port.cancelUICommand).not.toHaveBeenCalled();
});

it('não reserva outra navegação local nem aceita terceiro argumento', async () => {
  const f = fixture('workspace.tab.next');
  await expect(f.execution.execute('workspace.tab.previous')).resolves.toMatchObject({ status: 'cancelled', errorCode: 'ui-handler-missing' });
  expect(f.port.beginUICommand).not.toHaveBeenCalled();
  expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
});

it('adiciona workspace.create à família genérica sem alterar a família de abas', async () => {
  expect(isWorkspaceTabMutationCommand(WORKSPACE_CREATE_COMMAND_ID)).toBe(false);
  expect(isWorkspaceMutationCommand(WORKSPACE_CREATE_COMMAND_ID)).toBe(true);
  expect(WORKSPACE_MUTATION_COMMAND_IDS).toContain(WORKSPACE_CREATE_COMMAND_ID);
  expect(isWorkspaceMutationCommand('navigation.settings.open')).toBe(false);

  const f = fixture(WORKSPACE_CREATE_COMMAND_ID);
  await expect(f.execution.execute(WORKSPACE_CREATE_COMMAND_ID)).resolves.toMatchObject({ status: 'succeeded' });
  expect(f.port.beginUICommand).toHaveBeenCalledWith(WORKSPACE_CREATE_COMMAND_ID);
  expect(f.port.commitBackendCommand).toHaveBeenCalledWith('ticket', 'handoff');
});

it('reconhece workspace.chat.open como mutação contextual sem promovê-lo a mutação de aba', async () => {
  expect(isWorkspaceTabMutationCommand(WORKSPACE_CHAT_OPEN_COMMAND_ID)).toBe(false);
  expect(isWorkspaceMutationCommand(WORKSPACE_CHAT_OPEN_COMMAND_ID)).toBe(true);
  expect(WORKSPACE_MUTATION_COMMAND_IDS).toContain(WORKSPACE_CHAT_OPEN_COMMAND_ID);

  const f = fixture(WORKSPACE_CHAT_OPEN_COMMAND_ID);
  await expect(f.execution.execute(WORKSPACE_CHAT_OPEN_COMMAND_ID)).resolves.toMatchObject({ status: 'succeeded' });
  expect(f.port.beginUICommand).toHaveBeenCalledWith(WORKSPACE_CHAT_OPEN_COMMAND_ID);
  expect(f.port.commitBackendCommand).toHaveBeenCalledWith('ticket', 'handoff');
});

it.each(WORKSPACE_TAB_CREATE_COMMAND_IDS)('reconhece %s como criação contextual', async (commandID) => {
  const f = fixture(commandID);
  await f.execution.execute(commandID);
  expect(f.port.beginUICommand).toHaveBeenCalledWith(commandID);
  expect(f.port.commitBackendCommand).toHaveBeenCalledWith('ticket', 'handoff');
  expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(1);
});

describe.each(WORKSPACE_MUTATION_COMMAND_IDS)('submissão de escrita contextual %s', (commandID) => {
  it('aguarda persistência sem usar CompleteUICommand succeeded nem repetir a submissão', async () => {
    const f = fixture(commandID);
    const commit = deferred<void>();
    f.port.commitBackendCommand.mockReturnValue(commit.promise);
    const pending = f.execution.execute(commandID);
    expect(f.execution.execute(commandID)).toBe(pending);
    await vi.waitFor(() => expect(f.port.commitBackendCommand).toHaveBeenCalledWith('ticket', 'handoff'));
    expect(f.port.completeUICommand).not.toHaveBeenCalled();
    expect(f.port.getUICommandResult).not.toHaveBeenCalled();
    commit.resolve();
    await expect(pending).resolves.toMatchObject({ status: 'succeeded' });
    expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(1);
    expect(f.prepareTarget).toHaveBeenCalledTimes(1);
    expect(f.target.dispose).toHaveBeenCalledTimes(1);
  });
  it('recusa contexto inicial inválido antes de Begin', async () => {
    const f = fixture(commandID);
    f.target.isCurrent.mockReturnValue(false);
    await expect(f.execution.execute(commandID)).resolves.toMatchObject({ errorCode: 'ui-context-stale' });
    expect(f.port.beginUICommand).not.toHaveBeenCalled();
    expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
  });
  it('não submete quando o alvo muda enquanto aguarda Take', async () => {
    const f = fixture(commandID);
    const take = deferred<typeof f.handoff>();
    f.port.takeUICommand.mockReturnValue(take.promise);
    f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' });
    const pending = f.execution.execute(commandID);
    await vi.waitFor(() => expect(f.port.takeUICommand).toHaveBeenCalled());
    f.target.isCurrent.mockReturnValue(false);
    take.resolve(f.handoff);
    await expect(pending).resolves.toMatchObject({ status: 'cancelled' });
    expect(f.port.completeUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled');
    expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
  });
  it('rejeita handoff de outro comando sem executar escrita', async () => {
    const f = fixture(commandID);
    f.port.takeUICommand.mockResolvedValue({ ...f.handoff, commandId: 'workspace.list' });
    f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' });
    await f.execution.execute(commandID);
    expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
    expect(f.port.cancelUICommand).toHaveBeenCalledWith('ticket');
  });
  it('não aceita substituir o tipo por outro comando de criação autorizado', async () => {
    const f = fixture(commandID);
    const otherID = WORKSPACE_TAB_MUTATION_COMMAND_IDS.find((id) => id !== commandID)!;
    f.port.takeUICommand.mockResolvedValue({ ...f.handoff, commandId: otherID });
    f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'cancelled' });
    await f.execution.execute(commandID);
    expect(f.port.commitBackendCommand).not.toHaveBeenCalled();
    expect(f.port.cancelUICommand).toHaveBeenCalledWith('ticket');
  });
  it('não transforma desmontagem após submissão em cancelamento da escrita', async () => {
    const f = fixture(commandID);
    const commit = deferred<void>();
    f.port.commitBackendCommand.mockReturnValue(commit.promise);
    const pending = f.execution.execute(commandID);
    await vi.waitFor(() => expect(f.port.commitBackendCommand).toHaveBeenCalled());
    f.execution.dispose();
    await f.execution.cancel();
    expect(f.port.cancelUICommand).not.toHaveBeenCalled();
    commit.resolve();
    await expect(pending).resolves.toMatchObject({ status: 'succeeded' });
    expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(1);
  });
  it.each(['failed', 'outcome_unknown', 'succeeded'] as const)('lê %s autoritativo após perda de confirmação, sem retry', async (status) => {
    const f = fixture(commandID);
    f.port.commitBackendCommand.mockRejectedValue(new Error('transport lost'));
    f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status });
    await expect(f.execution.execute(commandID)).resolves.toMatchObject({ status });
    expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(1);
    expect(f.port.completeUICommand).not.toHaveBeenCalled();
  });
  it('não aceita comandos fora deste adapter', async () => {
    const f = fixture(commandID);
    await expect(f.execution.execute('navigation.settings.open')).resolves.toMatchObject({ errorCode: 'ui-handler-missing' });
    expect(f.port.beginUICommand).not.toHaveBeenCalled();
  });
  it('não declara sucesso quando a porta de submissão lança erro síncrono', async () => {
    const f = fixture(commandID);
    f.port.commitBackendCommand.mockImplementation(() => { throw new Error('bridge unavailable'); });
    f.port.getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'outcome_unknown' });
    await expect(f.execution.execute(commandID)).resolves.toMatchObject({ status: 'outcome_unknown' });
    expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(1);
    expect(f.port.completeUICommand).not.toHaveBeenCalled();
    expect(f.target.dispose).toHaveBeenCalledTimes(1);
  });
  it('uma nova tentativa captura novamente o alvo após a anterior terminar', async () => {
    const f = fixture(commandID);
    await f.execution.execute(commandID);
    await f.execution.execute(commandID);
    expect(f.port.beginUICommand).toHaveBeenCalledTimes(2);
    expect(f.port.commitBackendCommand).toHaveBeenCalledTimes(2);
    expect(f.prepareTarget).toHaveBeenCalledTimes(2);
    expect(f.target.dispose).toHaveBeenCalledTimes(2);
  });
});
