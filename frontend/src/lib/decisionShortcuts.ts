export type DecisionActionPolarity = 'affirmative' | 'negative';

export type DecisionActionScope =
  | 'current'
  | 'conversation'
  | 'persistent'
  | 'profile'
  | 'global';

export interface SemanticDecisionAction {
  id: string;
  polarity?: DecisionActionPolarity;
  scope?: DecisionActionScope;
}

export interface DecisionShortcut {
  aria: string;
  display: string;
  key: 'Enter' | 'Backspace';
  ctrlKey: boolean;
  shiftKey: boolean;
}

export interface ResolvedDecisionShortcuts {
  byActionId: ReadonlyMap<string, readonly DecisionShortcut[]>;
  byAriaChord: ReadonlyMap<string, string>;
  collisions: readonly string[];
}

function shortcutFor(
  polarity: DecisionActionPolarity,
  scope: DecisionActionScope,
): DecisionShortcut {
  const key = polarity === 'affirmative' ? 'Enter' : 'Backspace';
  const persistent =
    scope === 'persistent' || scope === 'profile' || scope === 'global';
  const ctrlKey = scope === 'current' || persistent;
  const shiftKey = scope === 'conversation' || persistent;
  const modifiers = [
    ...(ctrlKey ? ['Ctrl'] : []),
    ...(shiftKey ? ['Shift'] : []),
  ];
  const ariaModifiers = [
    ...(ctrlKey ? ['Control'] : []),
    ...(shiftKey ? ['Shift'] : []),
  ];

  return {
    aria: [...ariaModifiers, key].join('+'),
    display: [...modifiers, key].join('+'),
    key,
    ctrlKey,
    shiftKey,
  };
}

/**
 * Resolve atalhos somente de metadados semânticos explícitos.
 * Em colisão, o chord é removido de todas as ações envolvidas.
 */
export function resolveDecisionShortcuts(
  actions: readonly SemanticDecisionAction[],
): ResolvedDecisionShortcuts {
  const candidates = new Map<
    string,
    Array<{ actionId: string; shortcut: DecisionShortcut }>
  >();

  for (const action of actions) {
    if (!action.polarity || !action.scope) continue;
    const shortcut = shortcutFor(action.polarity, action.scope);
    const entries = candidates.get(shortcut.aria) ?? [];
    entries.push({ actionId: action.id, shortcut });
    candidates.set(shortcut.aria, entries);
  }

  const mutableByActionId = new Map<string, DecisionShortcut[]>();
  const byAriaChord = new Map<string, string>();
  const collisions: string[] = [];

  for (const [chord, entries] of candidates) {
    if (entries.length > 1) {
      collisions.push(
        `${chord}: ${entries.map((entry) => entry.actionId).join(', ')}`,
      );
      continue;
    }

    const [{ actionId, shortcut }] = entries;
    const actionShortcuts = mutableByActionId.get(actionId) ?? [];
    actionShortcuts.push(shortcut);
    mutableByActionId.set(actionId, actionShortcuts);
    byAriaChord.set(chord, actionId);
  }

  return {
    byActionId: mutableByActionId,
    byAriaChord,
    collisions,
  };
}

export function shortcutForKeyboardEvent(
  event: KeyboardEvent,
): string | undefined {
  if (event.altKey || event.metaKey) return undefined;
  if (!event.ctrlKey && !event.shiftKey) return undefined;
  if (event.key !== 'Enter' && event.key !== 'Backspace') return undefined;

  return [
    ...(event.ctrlKey ? ['Control'] : []),
    ...(event.shiftKey ? ['Shift'] : []),
    event.key,
  ].join('+');
}
