import { describe, expect, it } from 'vitest';
import {
  commandShortcutFromKeyboardEvent,
  formatCommandKeyboardTrigger,
  formatCommandShortcut,
  isCommandKeyboardTrigger,
  isCommandShortcutSequence,
  isCommandShortcut,
  normalizeCommandShortcutSequence,
  serializeCommandKeyboardTrigger,
  serializeCommandShortcut,
  validKeyboardCode,
} from './commandShortcut';
import type { CommandShortcut, CommandShortcutSequence } from './commandShortcut';

describe('commandShortcut', () => {
  it('valida exatamente os códigos aceitos pelo contrato Go', () => {
    expect(validKeyboardCode('KeyA')).toBe(true);
    expect(validKeyboardCode('Digit9')).toBe(true);
    expect(validKeyboardCode('F24')).toBe(true);
    expect(validKeyboardCode('F25')).toBe(false);
    expect(validKeyboardCode('Keya')).toBe(false);
    expect(validKeyboardCode('Numpad1')).toBe(false);
  });

  it('normaliza ordem e serializa sem location', () => {
    const shortcut: CommandShortcut = { version: 1, code: 'KeyN', modifiers: ['Shift', 'Control'] };
    expect(serializeCommandShortcut(shortcut)).toBe('{"version":1,"code":"KeyN","modifiers":["Control","Shift"]}');
    expect(formatCommandShortcut(shortcut)).toBe('Control+Shift+KeyN');
    expect(isCommandShortcut({ ...shortcut, modifiers: ['Control', 'Control'] })).toBe(false);
  });

  it('rejeita repeat, IME, modifier-only e AltGr', () => {
    const base = { code: 'KeyA', key: 'a' };
    expect(commandShortcutFromKeyboardEvent(new KeyboardEvent('keydown', { ...base, repeat: true }))).toBeNull();
    expect(commandShortcutFromKeyboardEvent(new KeyboardEvent('keydown', { ...base, isComposing: true }))).toBeNull();
    expect(commandShortcutFromKeyboardEvent(new KeyboardEvent('keydown', { code: 'Control', key: 'Control', ctrlKey: true }))).toBeNull();
    const altGr = new KeyboardEvent('keydown', { ...base, ctrlKey: true, altKey: true });
    Object.defineProperty(altGr, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    expect(commandShortcutFromKeyboardEvent(altGr)).toBeNull();
  });

  it('valida e normaliza uma sequência v2 estrita de dois passos', () => {
    const sequence: CommandShortcutSequence = {
      version: 2 as const,
      steps: [
        { code: 'KeyK', modifiers: ['Control', 'Shift'] },
        { code: 'KeyN', modifiers: [] },
      ],
    };
    expect(isCommandShortcutSequence(sequence)).toBe(true);
    expect(isCommandKeyboardTrigger(sequence)).toBe(true);
    expect(serializeCommandKeyboardTrigger(sequence)).toBe('{"version":2,"steps":[{"code":"KeyK","modifiers":["Control","Shift"]},{"code":"KeyN","modifiers":[]}]}');
    expect(formatCommandKeyboardTrigger(sequence)).toBe('Control+Shift+KeyK KeyN');
    expect(normalizeCommandShortcutSequence(sequence)).toEqual(sequence);
  });

  it('rejeita sequência v2 com forma extra, mais/menos passos ou Escape final', () => {
    const base = { version: 2 as const, steps: [{ code: 'KeyK', modifiers: ['Control'] as const }, { code: 'KeyN', modifiers: [] as const }] };
    expect(isCommandShortcutSequence({ ...base, extra: true })).toBe(false);
    expect(isCommandShortcutSequence({ ...base, steps: [base.steps[0]] })).toBe(false);
    expect(isCommandShortcutSequence({ ...base, steps: [base.steps[0], { code: 'Escape', modifiers: [] }] })).toBe(false);
    expect(isCommandShortcutSequence({ ...base, steps: [base.steps[0], { code: 'KeyN', modifiers: ['Shift'] }] })).toBe(false);
    expect(isCommandShortcutSequence({ ...base, steps: [{ ...base.steps[0], extra: true }, base.steps[1]] })).toBe(false);
  });
});
