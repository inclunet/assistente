import { afterEach, describe, expect, it } from 'vitest';
import { acquireCommandFocusTracking, observeCommandShortcutComposition, readCommandCompositionState } from './commandFocusContext';

const cleanup: Array<() => void> = [];
afterEach(() => {
  while (cleanup.length) cleanup.pop()?.();
  document.body.replaceChildren();
});

function fixture(track = true) {
  if (track) cleanup.push(acquireCommandFocusTracking(document));
  const input = document.createElement('textarea');
  document.body.append(input);
  input.focus();
  input.addEventListener('keydown', observeCommandShortcutComposition);
  return input;
}

describe('evidência IME do atalho local', () => {
  it('observa keydown não composing no campo atual sem exigir um ciclo IME anterior', () => {
    const input = fixture();
    expect(readCommandCompositionState(document, true)).toBe('unknown');
    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', ctrlKey: true, isComposing: false }));
    expect(readCommandCompositionState(document, true)).toBe('inactive');
  });
  it('não transforma composição ativa em inactive por um keydown sem flag', () => {
    const input = fixture();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', ctrlKey: true, isComposing: false }));
    expect(readCommandCompositionState(document, true)).toBe('active');
  });
  it.each([{ isComposing: true }, { keyCode: 229 }])('recusa evidência composing %j', (flags) => {
    const input = fixture();
    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT', ...flags }));
    expect(readCommandCompositionState(document, true)).toBe('active');
  });
  it('sem tracker adquirido permanece unknown', () => {
    const input = fixture(false);
    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT' }));
    expect(readCommandCompositionState(document, true)).toBe('unknown');
  });
  it('evento de outro campo não certifica o foco atual', () => {
    const input = fixture();
    const second = document.createElement('textarea');
    document.body.append(second);
    second.focus();
    input.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyT' }));
    expect(readCommandCompositionState(document, true)).toBe('unknown');
  });
});
