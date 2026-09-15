import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useQuestionnaireUIStore } from './questionnaireUIStore';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import type { DialogCommandScope } from '../lib/commandBridge';

function payload(id: string): QuestionnairePayload {
  return {
    id,
    kind: 'decision',
    questions: [],
    actions: [{ id: 'allow', label: 'Permitir' }],
  };
}

describe('questionnaireUIStore.cancelById', () => {
  beforeEach(() => {
    useQuestionnaireUIStore.setState({ active: null, activeScope: null, queue: [], _activeResolve: null });
  });

  it('remove e resolve item enfileirado sem tocar o diálogo ativo', async () => {
    const activeResult = vi.fn();
    useQuestionnaireUIStore.setState({ active: payload('active'), _activeResolve: activeResult });
    const queued = useQuestionnaireUIStore.getState().request(payload('queued'));

    expect(useQuestionnaireUIStore.getState().cancelById('queued')).toBe(true);
    await expect(queued).resolves.toEqual({ answers: {}, cancelled: true });
    expect(activeResult).not.toHaveBeenCalled();
    expect(useQuestionnaireUIStore.getState().active?.id).toBe('active');
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(0);
  });

  it('cancela o ativo exato e promove a fila, sem aceitar id inválido ou terceiro', async () => {
    const activeResult = vi.fn();
    useQuestionnaireUIStore.setState({ active: payload('active'), _activeResolve: activeResult });
    const queued = useQuestionnaireUIStore.getState().request(payload('queued'));

    expect(useQuestionnaireUIStore.getState().cancelById('')).toBe(false);
    expect(useQuestionnaireUIStore.getState().cancelById('foreign')).toBe(false);
    expect(activeResult).not.toHaveBeenCalled();
    expect(useQuestionnaireUIStore.getState().cancelById('active')).toBe(true);

    expect(activeResult).toHaveBeenCalledWith({ answers: {}, cancelled: true });
    expect(useQuestionnaireUIStore.getState().active?.id).toBe('queued');
    useQuestionnaireUIStore.getState().cancelById('queued');
    await expect(queued).resolves.toEqual({ answers: {}, cancelled: true });
  });

  it('rejeita id repetido antes de adicionar outro item à fila', async () => {
    useQuestionnaireUIStore.setState({ active: payload('active') });
    const queued = useQuestionnaireUIStore.getState().request(payload('queued'));

    await expect(useQuestionnaireUIStore.getState().request(payload('active'))).rejects.toThrow('already pending');
    await expect(useQuestionnaireUIStore.getState().request(payload('queued'))).rejects.toThrow('already pending');
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(1);
    useQuestionnaireUIStore.getState().cancelById('queued');
    await expect(queued).resolves.toEqual({ answers: {}, cancelled: true });
  });

  it('recusa escopo de outro diálogo e conserva cópia imutável ao promover a fila', async () => {
    const scope = {
      dialogId: 'queued', kind: 'decision', generation: '1',
      allowedCommandIds: ['decision.respond'],
      allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'],
    } as DialogCommandScope;
    await expect(useQuestionnaireUIStore.getState().request(payload('foreign'), scope)).rejects.toThrow('scope is invalid');
    const first = useQuestionnaireUIStore.getState().request(payload('active'));
    const queued = useQuestionnaireUIStore.getState().request(payload('queued'), scope);
    Object.assign(scope, { dialogId: 'mutated', generation: '99' });
    useQuestionnaireUIStore.getState().submit({});
    await first;
    const current = useQuestionnaireUIStore.getState().activeScope;
    expect(current?.dialogId).toBe('queued');
    expect(current?.generation).toBe('1');
    expect(Object.isFrozen(current)).toBe(true);
    expect(Object.isFrozen(current?.allowedCommandIds)).toBe(true);
    useQuestionnaireUIStore.getState().cancelById('queued');
    await queued;
    expect(useQuestionnaireUIStore.getState().activeScope).toBeNull();
  });
});
