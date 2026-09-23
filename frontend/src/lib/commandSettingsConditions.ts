import type { CommandCondition } from '../types/commandSettingsTypes';
import { isContextualPagePaletteCommand, isContextualPaletteCommand } from './commandContextualPalette';
import { isCommandLayerAction } from './commandLayerActions';
import { isEditorMermaidMutation } from './commandEditorMermaid';

function isContextualDeckCommand(id: string): boolean {
  return isEditorMermaidMutation(id) || (isContextualPaletteCommand(id) && !isContextualPagePaletteCommand(id));
}

/** Supressões da paleta conservam a seleção, mas não têm comando de execução. */
export function commandConditionTargetID(origin: string, commandId: string, triggerSpec: string, suppressed: boolean): string {
  if (!suppressed) return commandId;
  if (origin !== 'palette') return '';
  try {
    const spec: unknown = JSON.parse(triggerSpec);
    if (!spec || typeof spec !== 'object' || Array.isArray(spec)) return '';
    const value = spec as Record<string, unknown>;
    return Object.keys(value).length === 2 && value.version === 1 && typeof value.selection === 'string'
      ? value.selection : '';
  } catch { return ''; }
}

/** Disponibilidade do editor acompanha as fontes autoritativas de cada origem. */
export function commandConditionSupportedByOrigin(origin: string, field: string, localUI: boolean, commandID = ''): boolean {
  if (origin === 'streamdeck.key' && !localUI && isCommandLayerAction(commandID)) {
    return ['app.focused', 'surface.type', 'surface.id', 'profile', 'foreground.process', 'device'].includes(field);
  }
  if ((origin === 'palette' || origin === 'streamdeck.key') && isContextualPagePaletteCommand(commandID)) {
    return ['app.focused', 'surface.type', 'profile'].includes(field);
  }
  if (origin === 'keyboard.local' || ((origin === 'palette' || origin === 'streamdeck.key') && localUI) ||
      (origin === 'streamdeck.key' && isContextualDeckCommand(commandID)) ||
      (origin === 'palette' && (isContextualPaletteCommand(commandID) || isCommandLayerAction(commandID)))) {
    return ['app.focused', 'surface.type', 'surface.id', 'profile'].includes(field);
  }
  if (localUI) return false;
  if (origin === 'streamdeck.key') return ['profile', 'foreground.process', 'device'].includes(field);
  if (origin === 'keyboard.global') return ['profile', 'foreground.process'].includes(field);
  return origin === 'palette' && field === 'profile';
}

/** Mantém o par tipo/aba sem converter uma referência ausente em outra aba. */
export function reconcileCommandSurfaceCondition(
  previous: CommandCondition,
  next: CommandCondition,
  tabs: readonly { id: string; type: string }[],
): CommandCondition {
  const id = next.clauses.find(clause => clause.field === 'surface.id')?.value;
  if (typeof id !== 'string') return next;
  const previousId = previous.clauses.find(clause => clause.field === 'surface.id')?.value;
  const tab = tabs.find(candidate => candidate.id === id);
  if (id !== previousId && tab) {
    return { version: 1, clauses: [
      ...next.clauses.filter(clause => clause.field !== 'surface.type'),
      { field: 'surface.type', value: tab.type },
    ] };
  }
  const oldType = previous.clauses.find(clause => clause.field === 'surface.type')?.value;
  const newType = next.clauses.find(clause => clause.field === 'surface.type')?.value;
  if (oldType !== newType && (!tab || tab.type !== newType)) {
    return { version: 1, clauses: next.clauses.filter(clause => clause.field !== 'surface.id') };
  }
  return next;
}
