import {
  DECISION_REPEAT_TRIGGER,
  DECISION_RESPOND_COMMAND_ID,
  type DialogCommandScope,
} from './commandBridge';
import {
  isDecisionQuestionnaire,
  type QuestionnairePayload,
} from '../components/ui/QuestionnaireDialog';

type ValidDecisionPayload = QuestionnairePayload & {
  kind: 'decision';
  actions: NonNullable<QuestionnairePayload['actions']>;
};

let nextDecisionScopeGeneration = 0n;

function validText(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0 && value.trim() === value;
}

function validQuestionnaireText(value: unknown): boolean {
  if (typeof value === 'string') return value.length > 0 && value.trim() === value;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const text = value as { key?: unknown; fallback?: unknown; params?: unknown };
  if (text.key !== undefined && (typeof text.key !== 'string' || text.key.trim() !== text.key)) return false;
  if (text.fallback !== undefined && (typeof text.fallback !== 'string' || text.fallback.trim() !== text.fallback)) return false;
  if (text.params !== undefined && (!text.params || typeof text.params !== 'object' || Array.isArray(text.params))) return false;
  return validText(text.key) || validText(text.fallback);
}

function validDecisionAction(action: unknown): boolean {
  if (!action || typeof action !== 'object') return false;
  const value = action as {
    id?: unknown;
    label?: unknown;
    variant?: unknown;
    shortcut?: unknown;
    primary?: unknown;
    polarity?: unknown;
    scope?: unknown;
  };
  if (!validText(value.id) || !validQuestionnaireText(value.label)) return false;
  if (value.variant !== undefined && !['primary', 'secondary', 'danger', 'ghost', 'outline'].includes(value.variant as string)) return false;
  if (value.shortcut !== undefined && !validText(value.shortcut)) return false;
  if (value.primary !== undefined && typeof value.primary !== 'boolean') return false;
  if (value.polarity !== undefined && !['affirmative', 'negative'].includes(value.polarity as string)) return false;
  if (value.scope !== undefined && !['current', 'conversation', 'persistent', 'profile', 'global'].includes(value.scope as string)) return false;
  return true;
}

/** Valida o payload de decisão e, quando indicado, seu vínculo com o ID esperado. */
export function isValidDecisionQuestionnairePayload(
  value: unknown,
  expectedDialogId?: string,
): value is ValidDecisionPayload {
  if (!value || typeof value !== 'object' || !isDecisionQuestionnaire(value as QuestionnairePayload)) return false;
  const payload = value as QuestionnairePayload;
  if (!validText(payload.id) || (expectedDialogId !== undefined && payload.id !== expectedDialogId)) return false;
  const actions = payload.actions!;
  return actions.every(validDecisionAction)
    && new Set(actions.map((action) => action.id)).size === actions.length;
}

/**
 * Cria scope somente para uma decisão estruturalmente válida e para seu ID
 * exato. O scope limita o dispatcher da UI; não autoriza a operação no backend.
 */
export function createDecisionQuestionnaireScope(
  payload: unknown,
  expectedDialogId?: string,
): DialogCommandScope | null {
  if (!isValidDecisionQuestionnairePayload(payload, expectedDialogId)) return null;
  return Object.freeze({
    dialogId: payload.id,
    kind: 'decision' as const,
    generation: (++nextDecisionScopeGeneration).toString(),
    allowedCommandIds: Object.freeze([DECISION_RESPOND_COMMAND_ID]) as ['decision.respond'],
    allowedTriggerSpecs: Object.freeze([DECISION_REPEAT_TRIGGER]) as ['keyboard.local:Ctrl+Shift+R'],
  });
}
