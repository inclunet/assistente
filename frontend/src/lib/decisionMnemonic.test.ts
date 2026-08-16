import { describe, expect, it } from 'vitest';
import {
  assignMnemonics,
  findMnemonicIndex,
  isEditableKeyboardTarget,
  parseMnemonicMarker,
} from './decisionMnemonic';

describe('decisionMnemonic', () => {
  it('parseia marcador & no rótulo', () => {
    expect(parseMnemonicMarker('&Sim')).toEqual({ displayLabel: 'Sim', mnemonic: 's' });
    expect(parseMnemonicMarker('Ca&ncelar')).toEqual({
      displayLabel: 'Cancelar',
      mnemonic: 'n',
    });
    expect(parseMnemonicMarker('&Áudio')).toEqual({ displayLabel: 'Áudio', mnemonic: 'a' });
  });

  it('atribuí mnemônicos únicos evitando colisão', () => {
    expect(assignMnemonics([{ label: 'Confirmar' }, { label: 'Cancelar' }])).toEqual([
      'c',
      'a',
    ]);
  });

  it('respeita shortcut explícito', () => {
    expect(
      assignMnemonics([
        { label: 'Permitir', shortcut: 'p' },
        { label: 'Negar', shortcut: 'n' },
      ]),
    ).toEqual(['p', 'n']);
  });

  it('findMnemonicIndex casa letra acentuada com mnemônico normalizado', () => {
    expect(findMnemonicIndex('Áudio', 'a')).toBe(0);
  });

  it('detecta alvo editável', () => {
    const input = document.createElement('input');
    input.type = 'text';
    expect(isEditableKeyboardTarget(input)).toBe(true);

    const button = document.createElement('input');
    button.type = 'button';
    expect(isEditableKeyboardTarget(button)).toBe(false);
  });
});
