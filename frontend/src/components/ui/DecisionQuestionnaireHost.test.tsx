import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import {
  DecisionQuestionnaireHost,
  DECISION_ANSWER_ACTION_ID,
} from './DecisionQuestionnaireHost';
import { isDecisionQuestionnaire } from './QuestionnaireDialog';
import type { QuestionnairePayload } from './QuestionnaireDialog';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: string | { defaultValue?: string }) => {
      if (typeof opts === 'string') return opts;
      return opts?.defaultValue ?? key;
    },
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
    announceRequest: vi.fn(),
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playSound: vi.fn(),
  SOUND_TYPES: { ALERT: 'alert' },
}));

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (s: { config: { decisionAlertSound: boolean } }) => unknown) =>
    selector({ config: { decisionAlertSound: false } }),
}));

function shellDecision(): QuestionnairePayload {
  return {
    id: 'q1',
    kind: 'decision',
    title: { key: 'app.questionnaire.shell.title', fallback: 'Confirmar execução de comando' },
    description: { key: 'app.questionnaire.shell.prompt', fallback: 'Permitir a execução deste comando?' },
    body: 'ls -la',
    actions: [
      { id: 'allow', label: { key: 'app.questionnaire.shell.submit', fallback: 'Permitir' }, primary: true, variant: 'primary' },
      { id: 'deny', label: { key: 'app.questionnaire.shell.cancel', fallback: 'Negar' }, variant: 'outline' },
    ],
    questions: [],
  };
}

describe('isDecisionQuestionnaire', () => {
  it('reconhece kind=decision com ações', () => {
    expect(isDecisionQuestionnaire(shellDecision())).toBe(true);
  });

  it('rejeita questionário de formulário', () => {
    expect(
      isDecisionQuestionnaire({
        id: 'q2',
        questions: [{ id: 'a', type: 'text', prompt: 'Nome' }],
      }),
    ).toBe(false);
  });

  it('rejeita decision sem ações', () => {
    expect(
      isDecisionQuestionnaire({ id: 'q3', kind: 'decision', actions: [], questions: [] }),
    ).toBe(false);
  });
});

describe('DecisionQuestionnaireHost', () => {
  it('não renderiza quando o payload não é decisão', () => {
    const { container } = render(
      <DecisionQuestionnaireHost
        data={{ id: 'q', questions: [] }}
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('abre DecisionDialog e responde com actionId', () => {
    const onAction = vi.fn();
    render(
      <DecisionQuestionnaireHost
        data={shellDecision()}
        onAction={onAction}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(screen.getByText('ls -la')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /Permitir/i }));
    expect(onAction).toHaveBeenCalledWith({ [DECISION_ANSWER_ACTION_ID]: 'allow' });
  });

  it('nega via ação deny (não cancela o diálogo)', () => {
    const onAction = vi.fn();
    const onCancel = vi.fn();
    render(
      <DecisionQuestionnaireHost
        data={shellDecision()}
        onAction={onAction}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /Negar/i }));
    expect(onAction).toHaveBeenCalledWith({ [DECISION_ANSWER_ACTION_ID]: 'deny' });
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('respeita allowCancel=false ignorando ESC', () => {
    const onCancel = vi.fn();
    render(
      <DecisionQuestionnaireHost
        data={{ ...shellDecision(), allowCancel: false }}
        onAction={vi.fn()}
        onCancel={onCancel}
      />,
    );
    fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' });
    expect(onCancel).not.toHaveBeenCalled();
  });

  it('permite ESC quando allowCancel não é false', () => {
    const onCancel = vi.fn();
    render(
      <DecisionQuestionnaireHost
        data={{ ...shellDecision(), allowCancel: true }}
        onAction={vi.fn()}
        onCancel={onCancel}
      />,
    );
    fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' });
    expect(onCancel).toHaveBeenCalled();
  });
});
