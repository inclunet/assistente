import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function installDocumentMock() {
  const originalDocument = (globalThis as any).document;
  const originalRaf = (globalThis as any).requestAnimationFrame;

  (globalThis as any).requestAnimationFrame = (cb: (t: number) => void) => {
    cb(0);
    return 0;
  };

  (globalThis as any).document = {
    activeElement: null,
    contains: () => true,
  };

  return () => {
    (globalThis as any).document = originalDocument;
    (globalThis as any).requestAnimationFrame = originalRaf;
  };
}

describe('confirmStore', () => {
  let restoreGlobals: (() => void) | null = null;

  beforeEach(() => {
    restoreGlobals = installDocumentMock();
  });

  afterEach(() => {
    restoreGlobals?.();
    restoreGlobals = null;
  });

  it('resolve true quando confirmado', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');

    const promise = mod.requestConfirm({ title: 'Excluir', message: 'Tem certeza?' });

    expect(mod.useConfirmStore.getState().active?.title).toBe('Excluir');

    mod.useConfirmStore.getState().confirm();

    await expect(promise).resolves.toBe(true);
    expect(mod.useConfirmStore.getState().active).toBe(null);
  });

  it('enfileira confirmações e respeita a ordem', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');

    const p1 = mod.requestConfirm({ title: 'A', message: 'msg A' });
    const p2 = mod.requestConfirm({ title: 'B', message: 'msg B' });

    expect(mod.useConfirmStore.getState().active?.title).toBe('A');

    mod.useConfirmStore.getState().cancel();
    await expect(p1).resolves.toBe(false);

    expect(mod.useConfirmStore.getState().active?.title).toBe('B');

    mod.useConfirmStore.getState().confirm();
    await expect(p2).resolves.toBe(true);

    expect(mod.useConfirmStore.getState().active).toBe(null);
  });

  it('tenta restaurar foco ao finalizar', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');

    const focus = vi.fn();
    const element = { focus };

    (globalThis as any).document.activeElement = element;

    const p = mod.requestConfirm({ title: 'X', message: 'Y' });
    mod.useConfirmStore.getState().confirm();

    await expect(p).resolves.toBe(true);
    expect(focus).toHaveBeenCalledTimes(1);
  });
});
