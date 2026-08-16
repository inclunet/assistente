/**
 * Mnemônicos de ação para DecisionDialog (AEP-0091 D6).
 * `&X` no rótulo marca a letra; senão a primeira letra livre do label.
 */

export function parseMnemonicMarker(label: string): {
  displayLabel: string;
  mnemonic: string | undefined;
} {
  const match = label.match(/&([A-Za-z0-9])/);
  if (match && match.index !== undefined) {
    const letter = match[1];
    const displayLabel =
      label.slice(0, match.index) + letter + label.slice(match.index + 2);
    return { displayLabel, mnemonic: letter.toLowerCase() };
  }
  return { displayLabel: label, mnemonic: undefined };
}

function normalizeKeyChar(ch: string): string | undefined {
  const normalized = ch.toLowerCase().normalize('NFD').replace(/\p{M}/gu, '');
  if (/^[a-z0-9]$/.test(normalized)) return normalized;
  return undefined;
}

/**
 * Atribui um mnemônico único por ação (ordem estável).
 * `shortcut` explícito tem prioridade; depois `&X` no label; depois primeira letra livre.
 */
export function assignMnemonics(
  actions: ReadonlyArray<{ label: string; shortcut?: string }>,
): string[] {
  const used = new Set<string>();

  return actions.map((action) => {
    const explicit = action.shortcut?.trim();
    if (explicit) {
      const key = normalizeKeyChar(explicit[0]);
      if (key && !used.has(key)) {
        used.add(key);
        return key;
      }
    }

    const { displayLabel, mnemonic } = parseMnemonicMarker(action.label);
    if (mnemonic && !used.has(mnemonic)) {
      used.add(mnemonic);
      return mnemonic;
    }

    for (const ch of displayLabel) {
      const key = normalizeKeyChar(ch);
      if (key && !used.has(key)) {
        used.add(key);
        return key;
      }
    }

    return '';
  });
}

export function isEditableKeyboardTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true;
  if (tag === 'INPUT') {
    const type = (target as HTMLInputElement).type.toLowerCase();
    return !['button', 'submit', 'checkbox', 'radio', 'reset', 'file', 'image'].includes(type);
  }
  return false;
}
