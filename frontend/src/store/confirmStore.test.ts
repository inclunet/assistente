import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const restoreDefaultFocusMock = vi.hoisted(() => vi.fn(() => true));
const isModalOpenMock = vi.hoisted(() => vi.fn(() => false));

vi.mock('../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: restoreDefaultFocusMock,
}));

vi.mock('../lib/modalRegistry', () => ({
  isModalOpen: isModalOpenMock,
}));

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

  const body = {} as HTMLElement;
  globalWithDoc.document = {
    activeElement: null,
    body,
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
    restoreDefaultFocusMock.mockClear();
    isModalOpenMock.mockReset();
    isModalOpenMock.mockReturnValue(false);
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
    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });

  it('usa o foco padrão quando o gatilho foi removido', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');
    const trigger = { focus: vi.fn() } as unknown as HTMLElement;
    const globalWithDoc = globalThis as typeof globalThis & { document: Document };
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: trigger,
      configurable: true,
      writable: true,
    });

    const promise = mod.requestConfirm({ title: 'X', message: 'Y' });
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: globalWithDoc.document.body,
      configurable: true,
      writable: true,
    });
    globalWithDoc.document.contains = ((node: Node) => node !== trigger) as typeof document.contains;
    mod.useConfirmStore.getState().confirm();

    await expect(promise).resolves.toBe(true);
    expect(trigger.focus).not.toHaveBeenCalled();
    expect(restoreDefaultFocusMock).toHaveBeenCalledOnce();
  });

  it('não rouba foco legítimo assumido depois do diálogo', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');
    const trigger = { focus: vi.fn() } as unknown as HTMLElement;
    const otherControl = {} as HTMLElement;
    const globalWithDoc = globalThis as typeof globalThis & { document: Document };
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: trigger,
      configurable: true,
      writable: true,
    });

    const promise = mod.requestConfirm({ title: 'X', message: 'Y' });
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: otherControl,
      configurable: true,
      writable: true,
    });
    mod.useConfirmStore.getState().confirm();

    await expect(promise).resolves.toBe(true);
    expect(trigger.focus).not.toHaveBeenCalled();
    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });

  it('não restaura foco enquanto outro modal está aberto', async () => {
    vi.resetModules();
    const mod = await import('./confirmStore');
    isModalOpenMock.mockReturnValue(true);
    const trigger = { focus: vi.fn() } as unknown as HTMLElement;
    const globalWithDoc = globalThis as typeof globalThis & { document: Document };
    Object.defineProperty(globalWithDoc.document, 'activeElement', {
      value: trigger,
      configurable: true,
    });

    const promise = mod.requestConfirm({ title: 'X', message: 'Y' });
    mod.useConfirmStore.getState().confirm();

    await expect(promise).resolves.toBe(true);
    expect(trigger.focus).not.toHaveBeenCalled();
    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });
});
