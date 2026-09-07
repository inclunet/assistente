import { describe, it, expect } from 'vitest';
import type { TFunction } from 'i18next';

import {
  questionnaireClosedMessage,
  QUESTIONNAIRE_CLOSED_CANCELLED,
  QUESTIONNAIRE_CLOSED_TIMEOUT,
} from './questionnaireClosed';

const t = ((key: string) => key) as unknown as TFunction;

describe('questionnaireClosedMessage', () => {
  it('diz que quem perguntou desistiu', () => {
    const message = questionnaireClosedMessage(t, {
      id: 'pergunta-1',
      reason: QUESTIONNAIRE_CLOSED_CANCELLED,
    });

    expect(message).toBe('app.questionnaire.closedCancelled');
  });

  it('diz que o prazo de resposta acabou', () => {
    const message = questionnaireClosedMessage(t, {
      id: 'pergunta-1',
      reason: QUESTIONNAIRE_CLOSED_TIMEOUT,
    });

    expect(message).toBe('app.questionnaire.closedTimeout');
  });

  it('avisa do sumiço mesmo sem motivo conhecido', () => {
    expect(questionnaireClosedMessage(t, { id: 'pergunta-1' })).toBe('app.questionnaire.closed');
    expect(questionnaireClosedMessage(t, { reason: 'motivo_novo' })).toBe('app.questionnaire.closed');
  });
});
