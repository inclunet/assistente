import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
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
  announce: vi.fn(),
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
    bodyLabel: { key: 'app.questionnaire.shell.bodyLabel', fallback: 'Comando solicitado' },
    actions: [
      { id: 'allow', label: { key: 'app.questionnaire.shell.submit', fallback: 'Permitir' }, primary: true, variant: 'primary', polarity: 'affirmative', scope: 'current' },
      { id: 'deny', label: { key: 'app.questionnaire.shell.cancel', fallback: 'Negar' }, variant: 'outline', polarity: 'negative', scope: 'current' },
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
    expect(screen.getByRole('group', { name: 'Comando solicitado' })).toBeInTheDocument();
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

  it('transporta escopos current/conversation/global da decisão de rede', () => {
    const onAction = vi.fn();
    const data: QuestionnairePayload = {
      ...shellDecision(),
      id: 'network',
      actions: [
        { id: 'once', label: 'Uma vez', polarity: 'affirmative', scope: 'current' },
        { id: 'session', label: 'Conversa', polarity: 'affirmative', scope: 'conversation' },
        { id: 'global', label: 'Global', polarity: 'affirmative', scope: 'global' },
        { id: 'deny', label: 'Negar', polarity: 'negative', scope: 'current' },
      ],
    };
    render(
      <DecisionQuestionnaireHost data={data} onAction={onAction} onCancel={vi.fn()} />,
    );

    expect(screen.getByRole('button', { name: 'Uma vez' })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Enter'),
    );
    expect(screen.getByRole('button', { name: 'Conversa' })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Shift+Enter'),
    );
    expect(screen.getByRole('button', { name: 'Global' })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Shift+Enter'),
    );

    fireEvent.keyDown(document, { key: 'Enter', shiftKey: true });
    expect(onAction).toHaveBeenCalledWith({
      [DECISION_ANSWER_ACTION_ID]: 'session',
    });
  });

  it('transporta ação persistente do ACP', () => {
    const onAction = vi.fn();
    const data: QuestionnairePayload = {
      ...shellDecision(),
      id: 'acp',
      actions: [
        { id: 'allow-once', label: 'Permitir uma vez', polarity: 'affirmative', scope: 'current' },
        { id: 'allow-always', label: 'Permitir sempre', polarity: 'affirmative', scope: 'persistent' },
        { id: 'deny', label: 'Negar', polarity: 'negative', scope: 'current' },
      ],
    };
    render(
      <DecisionQuestionnaireHost data={data} onAction={onAction} onCancel={vi.fn()} />,
    );

    fireEvent.keyDown(document, {
      key: 'Enter',
      ctrlKey: true,
      shiftKey: true,
    });
    expect(onAction).toHaveBeenCalledWith({
      [DECISION_ANSWER_ACTION_ID]: 'allow-always',
    });
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

  it('renderiza readonly_code no body e envia rejectReason com reject', () => {
    const onAction = vi.fn();
    const data: QuestionnairePayload = {
      id: 'edit-1',
      kind: 'decision',
      title: { fallback: 'Confirmar edição' },
      description: { fallback: 'Revise a alteração' },
      actions: [
        { id: 'apply', label: { fallback: 'Aplicar' }, primary: true, variant: 'primary' },
        { id: 'reject', label: { fallback: 'Rejeitar' }, variant: 'outline' },
      ],
      questions: [
        { id: 'before', type: 'readonly_code', prompt: { fallback: 'Antes' }, content: 'old' },
        { id: 'after', type: 'readonly_code', prompt: { fallback: 'Depois' }, content: 'new' },
      ],
      rejectReason: {
        id: 'reject_reason',
        label: { fallback: 'Motivo da rejeição (opcional)' },
        placeholder: { fallback: 'Explique' },
        maxLen: 2000,
      },
    };

    render(
      <DecisionQuestionnaireHost data={data} onAction={onAction} onCancel={vi.fn()} />,
    );

    expect(screen.getByRole('group', { name: 'Antes' })).toHaveTextContent('old');
    expect(screen.getByRole('group', { name: 'Depois' })).toHaveTextContent('new');
    expect(screen.getByRole('alertdialog').querySelector('.modal-body')).toHaveAttribute(
      'role',
      'application',
    );

    fireEvent.change(screen.getByLabelText(/Motivo da rejeição/i), {
      target: { value: 'prefiro o original' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Rejeitar/i }));
    expect(onAction).toHaveBeenCalledWith({
      [DECISION_ANSWER_ACTION_ID]: 'reject',
      reject_reason: 'prefiro o original',
    });
  });

  it('foca o bloco marcado com autoFocus, não o começo do body', async () => {
    const data: QuestionnairePayload = {
      id: 'edit-2',
      kind: 'decision',
      title: { fallback: 'Confirmar edição' },
      description: { fallback: 'Revise a alteração' },
      actions: [
        { id: 'apply', label: { fallback: 'Aplicar' }, primary: true, variant: 'primary' },
        { id: 'reject', label: { fallback: 'Rejeitar' }, variant: 'outline' },
      ],
      questions: [
        { id: 'before', type: 'readonly_code', prompt: { fallback: 'Antes' }, content: 'old' },
        {
          id: 'after',
          type: 'readonly_code',
          prompt: { fallback: 'Depois' },
          content: 'new',
          autoFocus: true,
        },
      ],
    };

    // O jsdom não faz layout: sem encenar o `offsetParent` o modal considera
    // invisível todo alvo de foco e cai no container, mascarando o que se testa.
    const descritor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent');
    Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
      configurable: true,
      get() {
        return document.body;
      },
    });
    try {
      render(
        <DecisionQuestionnaireHost data={data} onAction={vi.fn()} onCancel={vi.fn()} />,
      );

      await waitFor(() => {
        expect(screen.getByRole('document', { name: 'Depois' })).toHaveFocus();
      });
      expect(screen.getAllByRole('document')).toHaveLength(1);
    } finally {
      if (descritor) {
        Object.defineProperty(HTMLElement.prototype, 'offsetParent', descritor);
      } else {
        delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
      }
    }
  });

  it('passa answers no onCancel com motivo', () => {
    const onCancel = vi.fn();
    const data: QuestionnairePayload = {
      ...shellDecision(),
      rejectReason: {
        id: 'reject_reason',
        label: { fallback: 'Motivo' },
      },
    };
    render(
      <DecisionQuestionnaireHost data={data} onAction={vi.fn()} onCancel={onCancel} />,
    );
    fireEvent.change(screen.getByLabelText('Motivo'), {
      target: { value: 'depois' },
    });
    fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledWith({ reject_reason: 'depois' });
  });
});
