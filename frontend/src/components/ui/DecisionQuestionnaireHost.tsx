import { useMemo, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DecisionDialog,
  type DecisionAction,
  type DecisionRejectReason,
} from './DecisionDialog';
import {
  isDecisionQuestionnaire,
  type QuestionnairePayload,
  type QuestionnaireQuestion,
} from './QuestionnaireDialog';
import { resolveQuestionnaireText } from '../../lib/questionnaireText';

/** Chave de Answers alinhada a questionnaire.AnswerActionID (Go). */
export const DECISION_ANSWER_ACTION_ID = 'actionId';

export interface DecisionQuestionnaireHostProps {
  data: QuestionnairePayload | null;
  onAction: (answers: Record<string, unknown>) => void;
  onCancel: (answers?: Record<string, unknown>) => void;
}

function questionHasBodyContent(q: QuestionnaireQuestion): boolean {
  if (q.type === 'readonly_code') return true;
  return typeof q.content === 'string' && q.content.length > 0;
}

/**
 * Renderiza DecisionDialog quando o backend manda kind=decision (AEP-0091).
 * Sem payload de decisão, não renderiza nada (QuestionnaireDialog cobre o resto).
 */
export function DecisionQuestionnaireHost({
  data,
  onAction,
  onCancel,
}: DecisionQuestionnaireHostProps) {
  const { t } = useTranslation();

  const open = isDecisionQuestionnaire(data);

  const title = resolveQuestionnaireText(
    t,
    data?.title,
    t('ui.questionnaire.defaultTitle', 'Questionário'),
  );
  const descriptionParts = [
    resolveQuestionnaireText(t, data?.description),
    resolveQuestionnaireText(t, data?.hint),
  ].filter(Boolean);
  const description = descriptionParts.join(' ') || title;

  const bodyQuestions = useMemo(
    () => (data?.questions ?? []).filter(questionHasBodyContent),
    [data?.questions],
  );

  const body: ReactNode = useMemo(() => {
    if (bodyQuestions.length > 0) {
      return (
        <div className="decision-dialog__questions">
          {bodyQuestions.map((q) => {
            const labelId = `decision-q-label-${q.id}`;
            const prompt = resolveQuestionnaireText(t, q.prompt, q.id);
            return (
              <section key={q.id} className="decision-dialog__question">
                <h3 id={labelId} className="decision-dialog__question-label">
                  {prompt}
                </h3>
                <pre
                  className="decision-dialog__question-content"
                  data-decision-question={q.id}
                  tabIndex={0}
                  role="region"
                  aria-labelledby={labelId}
                >
                  {q.content ?? ''}
                </pre>
              </section>
            );
          })}
        </div>
      );
    }
    const plain = data?.body?.trim();
    return plain || undefined;
  }, [bodyQuestions, data?.body, t]);

  const rejectReason: DecisionRejectReason | undefined = useMemo(() => {
    if (!data?.rejectReason) return undefined;
    return {
      id: data.rejectReason.id,
      label: resolveQuestionnaireText(t, data.rejectReason.label),
      placeholder: resolveQuestionnaireText(t, data.rejectReason.placeholder) || undefined,
      maxLen: data.rejectReason.maxLen,
    };
  }, [data?.rejectReason, t]);

  const actions: DecisionAction[] = useMemo(() => {
    if (!open || !data?.actions) return [];
    return data.actions.map((action) => ({
      id: action.id,
      label: resolveQuestionnaireText(t, action.label, action.id),
      variant: action.variant,
      shortcut: action.shortcut
        ? resolveQuestionnaireText(t, action.shortcut)
        : undefined,
      primary: action.primary,
    }));
  }, [open, data?.actions, t]);

  const safeActionId = useMemo(() => {
    const deny = actions.find((a) => a.id === 'deny' || a.id === 'cancel' || a.id === 'reject' || a.variant === 'outline');
    return deny?.id ?? actions[actions.length - 1]?.id;
  }, [actions]);

  const severity = useMemo(
    () => (actions.some((a) => a.variant === 'danger') ? 'destructive' : 'permission'),
    [actions],
  );

  const size = bodyQuestions.length > 0 ? 'lg' : 'sm';

  // Focar o container do body faria a leitura começar no primeiro bloco. A
  // confirmação de edição marca o "Depois" com autoFocus justamente para o
  // NVDA abrir no texto resultante, e não no original.
  const initialFocusSelector = useMemo(() => {
    const target = bodyQuestions.find((q) => q.autoFocus);
    if (!target) return undefined;
    return `[data-decision-question="${CSS.escape(target.id)}"]`;
  }, [bodyQuestions]);

  // Respeita o contrato: allowCancel=false bloqueia ESC/X/clique fora, para o
  // backend não receber Cancelled=true de um pedido que exige uma das ações.
  const allowCancel = data?.allowCancel !== false;

  if (!open || actions.length === 0) {
    return null;
  }

  return (
    <DecisionDialog
      // key={data.id}: decisões em fila trocam o `data` sem o Modal fechar;
      // remontar força reanúncio e foco inicial para o novo pedido (NVDA).
      key={data?.id}
      isOpen
      title={title}
      description={description || title}
      body={body}
      size={size}
      rejectReason={rejectReason}
      actions={actions as [DecisionAction, ...DecisionAction[]]}
      severity={severity}
      safeActionId={safeActionId}
      initialFocusSelector={initialFocusSelector}
      // App restaura o foco após submit/cancel; evita restauração dupla.
      returnFocusOnClose={false}
      // allowCancel=false esconde o X e desliga ESC/clique fora (sem armadilha
      // de foco); só as ações fecham o diálogo.
      allowClose={allowCancel}
      onAction={(actionId, extras) =>
        onAction({ [DECISION_ANSWER_ACTION_ID]: actionId, ...extras })
      }
      onCancel={(extras) => onCancel(extras)}
    />
  );
}
