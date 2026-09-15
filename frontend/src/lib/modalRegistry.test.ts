import { afterEach, describe, expect, it } from 'vitest';
import {
  ensureModalCleanup,
  getModalRegistrySnapshot,
  registerOpenModal,
  unregisterOpenModal,
} from './modalRegistry';

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
});
