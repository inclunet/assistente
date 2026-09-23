import { afterEach, describe, expect, it } from 'vitest';
import {
  ensureModalCleanup,
  getModalRegistrySnapshot,
  getTopmostDialogCommandScope,
  getTopmostChatPresentationCommandIds,
  registerChatPresentationModalScope,
  registerOpenModal,
  updateOpenModalScope,
  unregisterOpenModal,
} from './modalRegistry';

const decisionScope = {
  dialogId: 'modal-test-a',
  kind: 'decision' as const,
  generation: '7',
  allowedCommandIds: ['decision.respond'] as const,
  allowedTriggerSpecs: ['keyboard.local:Ctrl+Shift+R'] as const,
};

function addOverlay(): HTMLDivElement {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  document.body.appendChild(overlay);
  return overlay;
}

afterEach(() => {
  document.querySelectorAll('.modal-overlay').forEach((element) => element.remove());
  unregisterOpenModal('modal-test-a');
  unregisterOpenModal('modal-test-b');
  ensureModalCleanup();
});

describe('modal registry snapshots', () => {
  it.each(['chat.pinned.open', 'chat.tokens.open'])('permite %s somente no scope topmost explícito', (commandID) => {
    addOverlay();
    registerOpenModal('modal-test-a');
    const dispose = registerChatPresentationModalScope('modal-test-a', [commandID]);
    expect(dispose).toBeTypeOf('function');
    expect(getTopmostChatPresentationCommandIds()).toEqual([commandID]);
    const generation = getModalRegistrySnapshot().generation;
    registerOpenModal('modal-test-b');
    expect(getTopmostChatPresentationCommandIds()).toBeNull();
    unregisterOpenModal('modal-test-b');
    expect(getTopmostChatPresentationCommandIds()).toEqual([commandID]);
    expect(getModalRegistrySnapshot().generation).not.toBe(generation);
    dispose?.();
    expect(getTopmostChatPresentationCommandIds()).toBeNull();
  });

  it('recusa clear e IDs desconhecidos sem ampliar scope de apresentação ou decisão', () => {
    addOverlay();
    registerOpenModal('modal-test-a');
    for (const commandID of ['chat.conversation.clear', 'chat.future.open']) {
      expect(registerChatPresentationModalScope('modal-test-a', [commandID])).toBeUndefined();
      expect(getTopmostChatPresentationCommandIds()).toBeNull();
    }
    registerOpenModal('modal-test-a', decisionScope);
    expect(registerChatPresentationModalScope('modal-test-a', ['chat.tokens.open'])).toBeUndefined();
    expect(getTopmostDialogCommandScope()).toMatchObject(decisionScope);
  });

  it('exposes the real top id and changes generation only on stack changes', () => {
    const overlay = addOverlay();
    const initial = getModalRegistrySnapshot();

    registerOpenModal('modal-test-a');
    const first = getModalRegistrySnapshot();
    expect(first.topID).toBe('modal-test-a');
    expect(first.snapshotGeneration).toBe(first.generation);
    expect(first.generationNumber).toBeGreaterThan(initial.generationNumber);

    registerOpenModal('modal-test-a');
    expect(getModalRegistrySnapshot().generation).toBe(first.generation);

    registerOpenModal('modal-test-b');
    const second = getModalRegistrySnapshot();
    expect(second.topID).toBe('modal-test-b');
    expect(second.generationNumber).toBeGreaterThan(first.generationNumber);
    expect(second.ids).toEqual(['modal-test-a', 'modal-test-b']);

    unregisterOpenModal('not-registered');
    expect(getModalRegistrySnapshot().generation).toBe(second.generation);

    overlay.remove();
    ensureModalCleanup();
    const cleaned = getModalRegistrySnapshot();
    expect(cleaned.topID).toBeNull();
    expect(cleaned.generationNumber).toBeGreaterThan(second.generationNumber);
    expect(getModalRegistrySnapshot().generation).toBe(cleaned.generation);
  });

  it('returns a detached snapshot of ids', () => {
    const overlay = addOverlay();
    registerOpenModal('modal-test-a');
    const snapshot = getModalRegistrySnapshot();
    const ids = snapshot.ids as string[];
    expect(Object.isFrozen(ids)).toBe(true);
    expect(() => ids.push('forged')).toThrow(TypeError);
    expect(getModalRegistrySnapshot().ids).toEqual(['modal-test-a']);
    registerOpenModal('modal-test-b');
    expect(snapshot.ids).toEqual(['modal-test-a']);
    expect(getModalRegistrySnapshot().ids).toEqual(['modal-test-a', 'modal-test-b']);
    overlay.remove();
    ensureModalCleanup();
  });

  it('reconciles a lost overlay notification during snapshot capture', () => {
    const overlay = addOverlay();
    registerOpenModal('modal-test-a');
    expect(getModalRegistrySnapshot().topID).toBe('modal-test-a');

    overlay.remove();
    const snapshot = getModalRegistrySnapshot();
    expect(snapshot.topID).toBeNull();
    expect(snapshot.ids).toEqual([]);
  });

  it('expõe somente o scope do topmost e bloqueia fallback sem scope', () => {
    const overlay = addOverlay();
    registerOpenModal('modal-test-a', decisionScope);
    expect(getTopmostDialogCommandScope()).toMatchObject(decisionScope);

    registerOpenModal('modal-test-b');
    const blocked = getModalRegistrySnapshot();
    expect(blocked.topID).toBe('modal-test-b');
    expect(blocked.dialogCommandScope).toBeNull();
    expect(getTopmostDialogCommandScope()).toBeNull();

    unregisterOpenModal('modal-test-b');
    const restored = getModalRegistrySnapshot();
    expect(restored.dialogCommandScope).toMatchObject(decisionScope);
    expect(Object.isFrozen(restored.dialogCommandScope)).toBe(true);
    expect(Object.isFrozen(restored.dialogCommandScope?.allowedCommandIds)).toBe(true);

    overlay.remove();
    ensureModalCleanup();
  });

  it('incrementa a geração quando o scope da mesma instância muda', () => {
    const overlay = addOverlay();
    registerOpenModal('modal-test-a', decisionScope);
    const first = getModalRegistrySnapshot();

    registerOpenModal('modal-test-a', { ...decisionScope, generation: '8' });
    const second = getModalRegistrySnapshot();
    expect(second.ids).toEqual(first.ids);
    expect(second.generationNumber).toBeGreaterThan(first.generationNumber);
    expect(second.dialogCommandScope?.generation).toBe('8');

    overlay.remove();
    ensureModalCleanup();
  });

  it('descarta scope inválido em vez de ampliar a superfície de comandos', () => {
    const overlay = addOverlay();
    registerOpenModal('modal-test-a', {
      ...decisionScope,
      allowedCommandIds: ['other.command'] as never,
    });
    expect(getModalRegistrySnapshot().dialogCommandScope).toBeNull();
    overlay.remove();
    ensureModalCleanup();
  });

  it('não registra scope para uma instância ausente', () => {
    expect(updateOpenModalScope('modal-test-a', decisionScope)).toBe(false);
    expect(getModalRegistrySnapshot().ids).toEqual([]);
    expect(getModalRegistrySnapshot().dialogCommandScope).toBeNull();
  });
});
