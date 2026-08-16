import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DecisionDialog,
  type DecisionAction,
} from './DecisionDialog';
import {
  isDecisionQuestionnaire,
  type QuestionnairePayload,
} from './QuestionnaireDialog';
import { resolveQuestionnaireText } from '../../lib/questionnaireText';

/** Chave de Answers alinhada a questionnaire.AnswerActionID (Go). */
export const DECISION_ANSWER_ACTION_ID = 'actionId';

export interface DecisionQuestionnaireHostProps {
  data: QuestionnairePayload | null;
  onAction: (answers: Record<string, unknown>) => void;
  onCancel: () => void;
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
  const description = resolveQuestionnaireText(t, data?.description);
  const body = data?.body?.trim() ? data.body : undefined;

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
    const deny = actions.find((a) => a.id === 'deny' || a.variant === 'outline');
    return deny?.id ?? actions[actions.length - 1]?.id;
  }, [actions]);

  if (!open || actions.length === 0) {
    return null;
  }

  return (
    <DecisionDialog
      isOpen
      title={title}
      description={description || title}
      body={body}
      actions={actions as [DecisionAction, ...DecisionAction[]]}
      severity="permission"
      safeActionId={safeActionId}
      onAction={(actionId) => onAction({ [DECISION_ANSWER_ACTION_ID]: actionId })}
      onCancel={onCancel}
    />
  );
}
