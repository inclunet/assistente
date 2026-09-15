import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createCommandBridgeDialogAdapter,
  questionnaireCommandDialogUI,
  type CommandDialogRequest,
  type CommandDialogUI,
} from './commandBridgeDialogAdapter';
import type { CommandBridgeOwner, CommandLifecycleEvent } from './commandBridge';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';

const owner: CommandBridgeOwner = {
  userId: 'user-a',
  sessionId: 'session-a',
  workspaceId: 'workspace-a',
};

function payload(id: string): QuestionnairePayload {
  return {
    id,
    kind: 'decision',
    title: 'Decisão de comando',
    description: 'Confirmar operação',
    questions: [],
    actions: [{ id: 'allow', label: 'Permitir' }],
  };
}

function request(id = 'dialog-a', generation = '1'): CommandDialogRequest {
  return { owner, sessionId: owner.sessionId, generation, payload: payload(id) };
}

function lifecycle(kind: CommandLifecycleEvent['kind'], generation?: string): CommandLifecycleEvent {
  return { kind, sessionId: owner.sessionId, generation };
}

function controlledUI(): {
  ui: CommandDialogUI;
  results: Map<string, (result: { answers: Record<string, unknown>; cancelled: boolean }) => void>;
} {
  const results = new Map<string, (result: { answers: Record<string, unknown>; cancelled: boolean }) => void>();
  const ui: CommandDialogUI = {
    request: vi.fn((data) => new Promise<{ answers: Record<string, unknown>; cancelled: boolean }>((resolve) => results.set(data.id, resolve))),
    cancelById: vi.fn(() => true),
  };
  return { ui, results };
}

describe('createCommandBridgeDialogAdapter', () => {
  beforeEach(() => {
    useQuestionnaireUIStore.setState({ active: null, queue: [], _activeResolve: null });
  });

  it('usa a stack de questionários existente e cancela apenas o diálogo ativo do próprio id', async () => {
    const lifecycleHost = { lifecycle: vi.fn(async () => undefined), shutdown: vi.fn(async () => undefined) };
    const adapter = createCommandBridgeDialogAdapter(lifecycleHost);

    const resultPromise = adapter.present(request());
    expect(useQuestionnaireUIStore.getState().active?.id).toBe('dialog-a');

    await adapter.lifecycle(lifecycle('lock', '1'));

    await expect(resultPromise).resolves.toEqual({ answers: {}, cancelled: true });
    expect(lifecycleHost.lifecycle).toHaveBeenCalledWith(lifecycle('lock', '1'));
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
  });

  it('não cancela modal de outro dono quando a stack está ocupada', async () => {
    const foreignResolve = vi.fn();
    useQuestionnaireUIStore.setState({
      active: payload('foreign-dialog'),
      _activeResolve: foreignResolve,
    });
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    });

    const resultPromise = adapter.present(request());
    await adapter.lifecycle(lifecycle('lock', '1'));

    await expect(resultPromise).resolves.toEqual({ answers: {}, cancelled: true });
    expect(useQuestionnaireUIStore.getState().active?.id).toBe('foreign-dialog');
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(0);
    expect(foreignResolve).not.toHaveBeenCalled();
    useQuestionnaireUIStore.getState().cancel();
  });

  it('recusa colisão de id antes de enfileirar e não cancela o terceiro', async () => {
    const foreignResolve = vi.fn();
    useQuestionnaireUIStore.setState({
      active: payload('shared-dialog'),
      _activeResolve: foreignResolve,
    });
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    });

    await expect(adapter.present(request('shared-dialog'))).rejects.toThrow('already pending');
    expect(useQuestionnaireUIStore.getState().active?.id).toBe('shared-dialog');
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(0);
    expect(foreignResolve).not.toHaveBeenCalled();
    useQuestionnaireUIStore.getState().cancel();
  });

  it('cancela geração antiga e preserva blur/release/repeat', async () => {
    const { ui, results } = controlledUI();
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    }, ui);
    const oldPromise = adapter.present(request('old', '1'));
    const currentPromise = adapter.present(request('current', '2'));

    await adapter.lifecycle(lifecycle('blur', '2'));
    await adapter.lifecycle(lifecycle('release', '2'));
    await adapter.lifecycle(lifecycle('repeat', '2'));
    expect(ui.cancelById).not.toHaveBeenCalled();

    await adapter.lifecycle(lifecycle('generation', '2'));
    expect(ui.cancelById).toHaveBeenCalledWith('old');
    await expect(oldPromise).resolves.toEqual({ answers: {}, cancelled: true });

    results.get('current')?.({ answers: { actionId: 'allow' }, cancelled: false });
    await expect(currentPromise).resolves.toEqual({
      answers: { actionId: 'allow' },
      cancelled: false,
    });
  });

  it('rejeita contexto implícito ou payload que não é decisão', async () => {
    const { ui } = controlledUI();
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    }, ui);

    await expect(adapter.present({
      ...request(),
      owner: { ...owner, sessionId: 'other-session' },
    })).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(adapter.present({
      ...request(),
      payload: { ...payload('form'), kind: 'form' },
    })).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(adapter.present({
      ...request(),
      payload: { ...payload('bad-action'), actions: [null as never] },
    })).rejects.toMatchObject({ code: 'invalid-request' });
    await expect(adapter.present({
      ...request(),
      payload: {
        ...payload('duplicate-actions'),
        actions: [{ id: 'same', label: 'A' }, { id: 'same', label: 'B' }],
      },
    })).rejects.toMatchObject({ code: 'invalid-request' });
    expect(ui.request).not.toHaveBeenCalled();
  });

  it('cancela a UI antes de um lifecycle bloqueado e continua após erro de cancelamento', async () => {
    const { ui } = controlledUI();
    let releaseLifecycle!: () => void;
    const lifecycleDone = new Promise<void>((resolve) => { releaseLifecycle = resolve; });
    const bridgeError = new Error('lifecycle rejected');
    const cancelById = vi.fn((id: string) => {
      if (id === 'first') throw new Error('cancel failed');
      return true;
    });
    ui.cancelById = cancelById;
    const lifecycleHost = {
      lifecycle: vi.fn(async () => {
        await lifecycleDone;
        throw bridgeError;
      }),
      shutdown: vi.fn(async () => undefined),
    };
    const adapter = createCommandBridgeDialogAdapter(lifecycleHost, ui);
    const first = adapter.present(request('first'));
    const second = adapter.present(request('second'));

    const lifecyclePromise = adapter.lifecycle(lifecycle('logout', '1'));
    await vi.waitFor(() => expect(cancelById).toHaveBeenCalledTimes(2));
    expect(cancelById).toHaveBeenNthCalledWith(1, 'first');
    expect(cancelById).toHaveBeenNthCalledWith(2, 'second');
    releaseLifecycle();
    await expect(lifecyclePromise).rejects.toBe(bridgeError);
    await expect(first).resolves.toEqual({ answers: {}, cancelled: true });
    await expect(second).resolves.toEqual({ answers: {}, cancelled: true });
  });

  it('congela cópia profunda do request e aceita workspace vazio no escopo global', async () => {
    const { ui, results } = controlledUI();
    const captured: QuestionnairePayload[] = [];
    vi.mocked(ui.request).mockImplementation((data) => {
      captured.push(data);
      return new Promise((resolve) => results.set(data.id, resolve));
    });
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    }, ui);
    const mutableOwner = { ...owner, workspaceId: '' };
    const mutablePayload = payload('frozen');
    mutablePayload.actions![0].label = { key: 'command.allow', params: { label: 'A' } };
    const mutableRequest: CommandDialogRequest = {
      owner: mutableOwner,
      sessionId: owner.sessionId,
      generation: '1',
      payload: mutablePayload,
    };
    const resultPromise = adapter.present(mutableRequest);
    mutableOwner.userId = 'user-mutated';
    Object.assign(mutableRequest, { generation: '9' });
    mutablePayload.actions![0].id = 'mutated';
    mutablePayload.actions!.push({ id: 'late', label: 'Late' });

    expect(captured).toHaveLength(1);
    expect(captured[0].actions).toHaveLength(1);
    expect(captured[0].actions?.[0].id).toBe('allow');
    expect(Object.isFrozen(captured[0])).toBe(true);
    expect(Object.isFrozen(captured[0].actions)).toBe(true);
    expect(Object.isFrozen(captured[0].actions?.[0])).toBe(true);

    await adapter.lifecycle(lifecycle('generation', '2'));
    await expect(resultPromise).resolves.toEqual({ answers: {}, cancelled: true });
  });

  it('clona __proto__ como propriedade própria sem poluir o protótipo', async () => {
    const { ui, results } = controlledUI();
    const captured: QuestionnairePayload[] = [];
    vi.mocked(ui.request).mockImplementation((data) => {
      captured.push(data);
      return new Promise((resolve) => results.set(data.id, resolve));
    });
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown: vi.fn(async () => undefined),
    }, ui);
    const maliciousPayload = JSON.parse('{"id":"json-proto","kind":"decision","questions":[],"actions":[{"id":"allow","label":"Permitir"}],"__proto__":{"polluted":"yes"}}') as QuestionnairePayload;

    const resultPromise = adapter.present({ ...request('json-proto'), payload: maliciousPayload });
    const cloned = captured[0] as unknown as Record<string, unknown>;
    expect(Object.getPrototypeOf(cloned)).toBeNull();
    expect(Object.prototype.hasOwnProperty.call(cloned, '__proto__')).toBe(true);
    expect(cloned['__proto__']).toEqual({ polluted: 'yes' });
    expect(Object.prototype.hasOwnProperty.call(Object.prototype, 'polluted')).toBe(false);

    await adapter.shutdown();
    await expect(resultPromise).resolves.toEqual({ answers: {}, cancelled: true });
  });

  it('fecha pedidos, rejeita novas entradas e delega shutdown uma única vez', async () => {
    const { ui } = controlledUI();
    const shutdown = vi.fn(async () => undefined);
    const adapter = createCommandBridgeDialogAdapter({
      lifecycle: vi.fn(async () => undefined),
      shutdown,
    }, ui);
    const pending = adapter.present(request());

    const first = adapter.shutdown();
    const second = adapter.shutdown();
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    await expect(pending).resolves.toEqual({ answers: {}, cancelled: true });
    await expect(adapter.present(request('after-shutdown'))).rejects.toMatchObject({ code: 'bridge-closed' });
    expect(shutdown).toHaveBeenCalledTimes(1);
  });

  it('a porta concreta da store só chama cancel quando o id está ativo', async () => {
    const active = payload('owned');
    const resolve = vi.fn();
    useQuestionnaireUIStore.setState({ active, _activeResolve: resolve });

    expect(questionnaireCommandDialogUI.cancelById('foreign')).toBe(false);
    expect(resolve).not.toHaveBeenCalled();
    expect(questionnaireCommandDialogUI.cancelById('owned')).toBe(true);
    expect(resolve).toHaveBeenCalledWith({ answers: {}, cancelled: true });
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
  });
});
