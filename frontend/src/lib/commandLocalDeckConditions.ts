import { isLocalUICommand } from './commandLocalUI';
import {
  createLocalPaletteConditionResolverFromParsed,
  parseLocalPaletteConditions,
  type LocalCommandPaletteVisualContext,
} from './commandLocalPaletteConditions';

/** Resolve o único comando LOCAL_UI permitido por um evento condicionado do Deck. */
export function resolveLocalDeckConditionCommand(
  raw: unknown,
  context: LocalCommandPaletteVisualContext | null | undefined,
): string | null {
  const conditions = parseLocalPaletteConditions(raw);
  return conditions ? resolveLocalDeckConditionCommandFromParsed(conditions, context) : null;
}

export function resolveLocalDeckConditionCommandFromParsed(
  conditions: readonly import('./commandLocalKeyboard').LocalCommandPaletteCondition[],
  context: LocalCommandPaletteVisualContext | null | undefined,
): string | null {
  if (conditions.length === 0 || !context || conditions.some(condition => !isLocalUICommand(condition.commandId))) return null;
  const resolve = createLocalPaletteConditionResolverFromParsed(conditions);
  const matches = conditions.filter(condition => resolve(condition.commandId, context));
  return matches.length === 1 ? matches[0].commandId : null;
}
