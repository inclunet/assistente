import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

function installDocumentMock() {
  const globalWithDoc = globalThis as typeof globalThis & {
    document: Document;
    requestAnimationFrame: (cb: FrameRequestCallback) => number;
  };
  const originalDocument = globalWithDoc.document;
  const originalRaf = globalWithDoc.requestAnimationFrame;

  globalWithDoc.requestAnimationFrame = (cb: FrameRequestCallback) => {
    cb(0);
    return 0;
  };

  globalWithDoc.document = {
    activeElement: null,
    contains: () => true,
  } as unknown as Document;

  return () => {
    globalWithDoc.document = originalDocument;
    globalWithDoc.requestAnimationFrame = originalRaf;
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

    const globalWithDoc = globalThis as typeof globalThis & { document: Document };
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: element as unknown as Element,
      configurable: true,
    });

    const p = mod.requestConfirm({ title: 'X', message: 'Y' });
    mod.useConfirmStore.getState().confirm();

    await expect(p).resolves.toBe(true);
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it('não restaura foco ao confirmar uma decisão que navega, mas restaura ao cancelar', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');

    const focus = vi.fn();
    const element = { focus };
    const globalWithDoc = globalThis as typeof globalThis & { document: Document };
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: element as unknown as Element,
      configurable: true,
    });

    const confirmed = mod.requestConfirm({
      title: 'Configurar voz',
      message: 'Deseja configurar?',
      restoreFocusOnConfirm: false,
    });
    mod.useConfirmStore.getState().confirm();
    await expect(confirmed).resolves.toBe(true);
    expect(focus).not.toHaveBeenCalled();

    const cancelled = mod.requestConfirm({
      title: 'Configurar voz',
      message: 'Deseja configurar?',
      restoreFocusOnConfirm: false,
    });
    mod.useConfirmStore.getState().cancel();
    await expect(cancelled).resolves.toBe(false);
    expect(focus).toHaveBeenCalledTimes(1);
  });
});
