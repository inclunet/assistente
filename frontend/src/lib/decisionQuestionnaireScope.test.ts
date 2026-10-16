import { describe, expect, it } from 'vitest';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import {
  createDecisionQuestionnaireScope,
  isValidDecisionQuestionnairePayload,
} from './decisionQuestionnaireScope';

function decision(id: string, actions: unknown[] = [{ id: 'allow', label: 'Permitir' }]): QuestionnairePayload {
  return { id, kind: 'decision', questions: [], actions: actions as QuestionnairePayload['actions'] };
}

describe('decisionQuestionnaireScope', () => {
  it('valida payload, vincula ID exato e cria gerações monotônicas com allowlists fixas', () => {
    const firstPayload = decision('dialog-a');
    const first = createDecisionQuestionnaireScope(firstPayload, 'dialog-a');
    const second = createDecisionQuestionnaireScope(decision('dialog-b'), 'dialog-b');

    expect(first).toMatchObject({
      dialogId: 'dialog-a',
      kind: 'decision',
      allowedCommandIds: ['decision.respond'],
      allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'],
    });
    expect(first?.generation).toMatch(/^[1-9]\d*$/);
    expect(BigInt(second!.generation)).toBeGreaterThan(BigInt(first!.generation));
    expect(Object.isFrozen(first)).toBe(true);
    expect(Object.isFrozen(first?.allowedCommandIds)).toBe(true);
    expect(Object.isFrozen(first?.allowedTriggerSpecs)).toBe(true);
    expect(createDecisionQuestionnaireScope(firstPayload, 'dialog-other')).toBeNull();
  });

  it('não cria scope para formulário ou payloads/ações de decisão inválidos', () => {
    const malformed = [
      { id: 'form', questions: [] },
      { ...decision('empty'), actions: [] },
      { ...decision('blank-id'), id: ' ' },
      decision('duplicate', [{ id: 'same', label: 'Um' }, { id: 'same', label: 'Dois' }]),
      decision('bad-action', [null]),
      decision('bad-action-id', [{ id: ' ', label: 'Ação' }]),
      decision('bad-label', [{ id: 'allow', label: {} }]),
      decision('bad-variant', [{ id: 'allow', label: 'Permitir', variant: 'unknown' }]),
      decision('bad-shortcut', [{ id: 'allow', label: 'Permitir', shortcut: {} }]),
      decision('bad-primary', [{ id: 'allow', label: 'Permitir', primary: 'yes' }]),
      decision('bad-polarity', [{ id: 'allow', label: 'Permitir', polarity: 'maybe' }]),
      decision('bad-scope', [{ id: 'allow', label: 'Permitir', scope: 'tenant' }]),
    ];

    for (const payload of malformed) {
      expect(isValidDecisionQuestionnairePayload(payload)).toBe(false);
      expect(createDecisionQuestionnaireScope(payload)).toBeNull();
    }
  });
});
