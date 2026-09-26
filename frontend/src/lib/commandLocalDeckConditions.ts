import { isLocalUICommand } from './commandLocalUI';
import {
  createLocalPaletteConditionResolverFromParsed,
  parseLocalPaletteConditions,
  resolveLocalPaletteConditionSelectionFromParsed,
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
  return resolveLocalDeckConditionSelectionFromParsed(conditions, context)?.commandId ?? null;
}

export function resolveLocalDeckConditionSelectionFromParsed(
  conditions: readonly import('./commandLocalKeyboard').LocalCommandPaletteCondition[],
  context: LocalCommandPaletteVisualContext | null | undefined,
): { commandId: string; arguments?: Readonly<Record<string, unknown>> } | null {
  if (conditions.length === 0 || !context || conditions.some(condition => !isLocalUICommand(condition.commandId))) return null;
  const resolve = createLocalPaletteConditionResolverFromParsed(conditions);
  const matches = conditions.filter(condition => resolve(condition.commandId, context));
  if (matches.length !== 1) return null;
  const selection = resolveLocalPaletteConditionSelectionFromParsed(conditions, matches[0].commandId, context);
  return selection?.available ? { commandId: matches[0].commandId, ...(selection.arguments ? { arguments: selection.arguments } : {}) } : null;
}
