export type CommandShortcutModifier = 'Control' | 'Alt' | 'Shift' | 'Meta';

export interface CommandShortcut {
  version: 1;
  code: string;
  modifiers: CommandShortcutModifier[];
}

export interface CommandShortcutSequenceStep {
  code: string;
  modifiers: CommandShortcutModifier[];
}

export interface CommandShortcutSequence {
  version: 2;
  steps: [CommandShortcutSequenceStep, CommandShortcutSequenceStep];
}

export type CommandKeyboardTrigger = CommandShortcut | CommandShortcutSequence;

export const COMMAND_SHORTCUT_MODIFIERS: readonly CommandShortcutModifier[] = ['Control', 'Alt', 'Shift', 'Meta'];

const KEYBOARD_CODES = new Set([
  'Enter', 'Escape', 'Tab', 'Space', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight',
  'Home', 'End', 'PageUp', 'PageDown', 'Delete', 'Backspace', 'Insert',
]);

/** Mantém exatamente a gramática de validKeyboardCode em internal/commandconfig/documents.go. */
export function validKeyboardCode(code: string): boolean {
  if (/^Key[A-Z]$/.test(code) || /^Digit[0-9]$/.test(code)) return true;
  if (/^F(?:[1-9]|1[0-9]|2[0-4])$/.test(code)) return true;
  return KEYBOARD_CODES.has(code);
}

export function isCommandShortcut(value: unknown): value is CommandShortcut {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<CommandShortcut>;
  if (candidate.version !== 1 || typeof candidate.code !== 'string' || !validKeyboardCode(candidate.code)) return false;
  if (!Array.isArray(candidate.modifiers)) return false;
  return candidate.modifiers.every((modifier): modifier is CommandShortcutModifier =>
    typeof modifier === 'string' && COMMAND_SHORTCUT_MODIFIERS.includes(modifier as CommandShortcutModifier),
  ) && new Set(candidate.modifiers).size === candidate.modifiers.length;
}

function isModifierList(value: unknown): value is CommandShortcutModifier[] {
  return Array.isArray(value) && value.every((modifier): modifier is CommandShortcutModifier =>
    typeof modifier === 'string' && COMMAND_SHORTCUT_MODIFIERS.includes(modifier as CommandShortcutModifier),
  ) && new Set(value).size === value.length;
}

function isSequenceStep(value: unknown): value is CommandShortcutSequenceStep {
  if (!value || typeof value !== 'object') return false;
  const step = value as Partial<CommandShortcutSequenceStep>;
  if (Object.keys(value).some((key) => key !== 'code' && key !== 'modifiers')) return false;
  return typeof step.code === 'string' && validKeyboardCode(step.code) && isModifierList(step.modifiers);
}

export function isCommandShortcutSequence(value: unknown): value is CommandShortcutSequence {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<CommandShortcutSequence>;
  if (Object.keys(value).some((key) => key !== 'version' && key !== 'steps')) return false;
  if (candidate.version !== 2 || !Array.isArray(candidate.steps) || candidate.steps.length !== 2) return false;
  const [prefix, final] = candidate.steps;
  if (!isSequenceStep(prefix) || !isSequenceStep(final)) return false;
  const prefixModifiers = prefix.modifiers;
  const hasPrefixModifier = prefixModifiers.some((modifier) => modifier === 'Control' || modifier === 'Alt' || modifier === 'Meta');
  if (!hasPrefixModifier) return false;
  if (final.modifiers.length !== 0 || final.code === 'Escape') return false;
  if (/^(Control|Alt|Shift|Meta)(Left|Right)?$/.test(prefix.code) || /^(Control|Alt|Shift|Meta)(Left|Right)?$/.test(final.code)) return false;
  return true;
}

export function isCommandKeyboardTrigger(value: unknown): value is CommandKeyboardTrigger {
  return isCommandShortcut(value) || isCommandShortcutSequence(value);
}

export function normalizeCommandShortcut(shortcut: CommandShortcut): CommandShortcut {
  if (!isCommandShortcut(shortcut)) throw new Error('Invalid command shortcut');
  return {
    version: 1,
    code: shortcut.code,
    modifiers: COMMAND_SHORTCUT_MODIFIERS.filter((modifier) => shortcut.modifiers.includes(modifier)),
  };
}

export function normalizeCommandShortcutSequence(sequence: CommandShortcutSequence): CommandShortcutSequence {
  if (!isCommandShortcutSequence(sequence)) throw new Error('Invalid command shortcut sequence');
  return {
    version: 2,
    steps: sequence.steps.map((step) => ({
      code: step.code,
      modifiers: COMMAND_SHORTCUT_MODIFIERS.filter((modifier) => step.modifiers.includes(modifier)),
    })) as [CommandShortcutSequenceStep, CommandShortcutSequenceStep],
  };
}

/** Produz o JSON aceito por decodeKeyboard; não inclui location. */
export function serializeCommandShortcut(shortcut: CommandShortcut): string {
  return JSON.stringify(normalizeCommandShortcut(shortcut));
}

export function serializeCommandKeyboardTrigger(trigger: CommandKeyboardTrigger): string {
  if (trigger.version === 1) return serializeCommandShortcut(trigger);
  return JSON.stringify(normalizeCommandShortcutSequence(trigger));
}

export function formatCommandShortcut(shortcut: CommandShortcut | null): string {
  if (!shortcut) return '';
  const normalized = normalizeCommandShortcut(shortcut);
  return [...normalized.modifiers, normalized.code].join('+');
}

export function formatCommandKeyboardTrigger(trigger: CommandKeyboardTrigger | null): string {
  if (!trigger) return '';
  if (trigger.version === 1) return formatCommandShortcut(trigger);
  const normalized = normalizeCommandShortcutSequence(trigger);
  return normalized.steps.map((step) => [...step.modifiers, step.code].join('+')).join(' ');
}

type KeyboardCaptureEvent = Pick<KeyboardEvent, 'code' | 'repeat' | 'isComposing' | 'keyCode' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'metaKey' | 'getModifierState'>;

/** Ctrl+Alt is ambiguous on Windows unless both physical modifiers were observed. */
export function createCommandModifierState() {
  const held = new Set<string>();
  return {
    observe(event: Pick<KeyboardEvent, 'type' | 'code' | 'ctrlKey' | 'altKey' | 'repeat' | 'getModifierState'>) {
      if (!event.ctrlKey) { held.delete('ControlLeft'); held.delete('ControlRight'); }
      if (!event.altKey) { held.delete('AltLeft'); held.delete('AltRight'); }
      if (event.type === 'keyup') held.delete(event.code);
      else if (event.type === 'keydown' && !event.repeat && /^(Control|Alt)(Left|Right)$/.test(event.code)) held.add(event.code);
      // Some WebViews report AltGraph with a synthetic Control event.
      if (event.type === 'keydown' && event.getModifierState('AltGraph')) held.add('AltRight');
    },
    allowsControlAlt() {
      return (held.has('ControlLeft') || held.has('ControlRight')) && held.has('AltLeft') && !held.has('AltRight');
    },
    clear() { held.clear(); },
  };
}

export function commandShortcutFromKeyboardEvent(event: KeyboardCaptureEvent, explicitControlAlt = false): CommandShortcut | null {
  if (event.repeat || event.isComposing || event.keyCode === 229) return null;
  if (event.getModifierState('AltGraph') || (event.ctrlKey && event.altKey && !event.shiftKey && !explicitControlAlt)) return null;
  if (/^(Control|Alt|Shift|Meta)(Left|Right)?$/.test(event.code)) return null;
  if (!validKeyboardCode(event.code)) return null;
  const flags: [CommandShortcutModifier, boolean][] = [
    ['Control', event.ctrlKey], ['Alt', event.altKey], ['Shift', event.shiftKey], ['Meta', event.metaKey],
  ];
  return { version: 1, code: event.code, modifiers: flags.filter(([, pressed]) => pressed).map(([modifier]) => modifier) };
}
