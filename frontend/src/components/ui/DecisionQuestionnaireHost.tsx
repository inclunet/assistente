import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DecisionDialog,
  type DecisionAction,
  type DecisionReadingRegion,
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

  const readingRegions: DecisionReadingRegion[] = useMemo(() => {
    if (bodyQuestions.length > 0) {
      return bodyQuestions.map((q) => ({
        id: q.id,
        label: resolveQuestionnaireText(t, q.prompt, q.id),
        content: q.content ?? '',
        autoFocus: q.autoFocus,
      }));
    }
    const plain = data?.body?.trim();
    if (!plain) return [];
    return [{
      id: 'body',
      label: resolveQuestionnaireText(
        t,
        data?.bodyLabel,
        t('ui.decisionDialog.detailsRegion'),
      ),
      content: plain,
      autoFocus: true,
    }];
  }, [bodyQuestions, data?.body, data?.bodyLabel, t]);

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
      polarity: action.polarity,
      scope: action.scope,
    }));
  }, [open, data?.actions, t]);

  const size = readingRegions.length > 1 ? 'lg' : 'sm';

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
      readingRegions={readingRegions}
      size={size}
      rejectReason={rejectReason}
      actions={actions as [DecisionAction, ...DecisionAction[]]}
      severity={data.severity ?? 'permission'}
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
