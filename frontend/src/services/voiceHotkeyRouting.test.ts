/** @vitest-environment jsdom */
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  captureVoiceHotkeyTarget,
  parseVoiceHotkeyEvent,
  registerVoiceHotkeyRecipient,
} from './voiceHotkeyRouting';
import { registerOpenModal, unregisterOpenModal } from '../lib/modalRegistry';

const cleanups: Array<() => void> = [];

function register(
  profileSlug = 'perfil',
  isEligible: () => boolean = () => true,
  deliver = vi.fn(),
  getProfileGeneration: () => number = () => 0,
) {
  cleanups.push(registerVoiceHotkeyRecipient({
    getProfileSlug: () => profileSlug,
    getProfileGeneration,
    isEligible,
    deliver,
  }));
  return deliver;
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  document.querySelectorAll('.modal-overlay').forEach((element) => element.remove());
  unregisterOpenModal('voice-hotkey-test-modal');
});

describe('voiceHotkeyRouting', () => {
  it('valida o evento sem aceitar campos obrigatórios ausentes', () => {
    expect(parseVoiceHotkeyEvent({
      triggerType: 'hotkey', bringToFront: true, triggerId: 123, profileId: 456,
    })).toEqual({ triggerType: 'hotkey', bringToFront: true });
    expect(parseVoiceHotkeyEvent({ triggerType: 'hotkey' })).toBeNull();
    expect(parseVoiceHotkeyEvent({ bringToFront: false })).toBeNull();
  });

  it('captura somente o receptor único e elegível do perfil exato', () => {
    let firstActive = true;
    let secondActive = false;
    const first = register('perfil', () => firstActive);
    const second = register('perfil', () => secondActive);

    const firstTarget = captureVoiceHotkeyTarget('perfil', { triggerType: 'hotkey', bringToFront: false });
    expect(firstTarget?.execute()).toBe(true);
    expect(first).toHaveBeenCalledOnce();
    expect(second).not.toHaveBeenCalled();

    firstActive = false;
    secondActive = true;
    const secondTarget = captureVoiceHotkeyTarget('perfil', { triggerType: 'hotkey', bringToFront: true });
    expect(secondTarget?.execute()).toBe(true);
    expect(second).toHaveBeenCalledOnce();

    firstActive = true;
    expect(captureVoiceHotkeyTarget('perfil', { triggerType: 'hotkey', bringToFront: true })).toBeUndefined();
  });

  it('recusa a inscrição removida durante a captura', () => {
    const deliver = vi.fn();
    let remove: () => void = () => undefined;
    const oldCleanup = registerVoiceHotkeyRecipient({
      getProfileSlug: () => 'outro', getProfileGeneration: () => 0, isEligible: () => true, deliver,
    });
    cleanups.push(oldCleanup);
    cleanups.push(registerVoiceHotkeyRecipient({
      getProfileSlug: () => 'outro', getProfileGeneration: () => 0,
      isEligible: () => { remove(); return false; }, deliver: vi.fn(),
    }));
    remove = oldCleanup;
    expect(captureVoiceHotkeyTarget('outro', { triggerType: 'hotkey', bringToFront: false })).toBeUndefined();
    expect(deliver).not.toHaveBeenCalled();
  });

  it('falha fechado para perfil duplicado, exceção de eligibility, modal e painel inativo', () => {
    register('duplicado');
    register('duplicado');
    expect(captureVoiceHotkeyTarget('duplicado', { triggerType: 'hotkey', bringToFront: false })).toBeUndefined();

    register('quebrado', () => { throw new Error('gone'); });
    expect(captureVoiceHotkeyTarget('quebrado', { triggerType: 'hotkey', bringToFront: false })).toBeUndefined();

    let enabled = false;
    register('inativo', () => enabled);
    expect(captureVoiceHotkeyTarget('inativo', { triggerType: 'hotkey', bringToFront: false })).toBeUndefined();

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.appendChild(overlay);
    registerOpenModal('voice-hotkey-test-modal');
    enabled = true;
    expect(captureVoiceHotkeyTarget('inativo', { triggerType: 'hotkey', bringToFront: false })).toBeUndefined();
  });

  it('não faz retarget quando o receptor capturado troca de perfil', () => {
    let slug = 'perfil-a';
    const first = vi.fn();
    cleanups.push(registerVoiceHotkeyRecipient({
      getProfileSlug: () => slug, getProfileGeneration: () => 0, isEligible: () => true, deliver: first,
    }));
    const other = register('perfil-b');
    const target = captureVoiceHotkeyTarget('perfil-a', { triggerType: 'hotkey', bringToFront: true });
    slug = 'perfil-b';
    expect(target?.isCurrent()).toBe(false);
    expect(target?.execute()).toBe(false);
    expect(first).not.toHaveBeenCalled();
    expect(other).not.toHaveBeenCalled();
  });

  it('executa exatamente uma vez e invalida após mudança modal ou desmontagem', () => {
    const deliver = register();
    const target = captureVoiceHotkeyTarget('perfil', { triggerType: 'hotkey', bringToFront: false })!;
    expect(target.execute()).toBe(true);
    expect(target.execute()).toBe(false);
    expect(deliver).toHaveBeenCalledOnce();

    const next = captureVoiceHotkeyTarget('perfil', { triggerType: 'hotkey', bringToFront: false })!;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.appendChild(overlay);
    registerOpenModal('voice-hotkey-test-modal');
    expect(next.isCurrent()).toBe(false);
    expect(next.execute()).toBe(false);
    expect(deliver).toHaveBeenCalledOnce();
  });
});
