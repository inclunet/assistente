import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ReadCommandContextFrame,
  ReadFocusContext,
  ReadSurfaceContext,
  ReadSurfaceContextDetailed,
  registerSurfaceContext,
  type SurfaceContext,
} from './commandContextProviders';
import {
  acquireCommandFocusTracking,
} from './commandFocusContext';

const cleanups: Array<() => void> = [];

afterEach(() => {
  while (cleanups.length > 0) cleanups.pop()?.();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function context(surfaceId: string, overrides: Partial<SurfaceContext> = {}): SurfaceContext {
  return {
    surfaceType: 'editor',
    surfaceId,
    snapshotVersion: 'snapshot-1',
    ...overrides,
  } as SurfaceContext;
}

describe('command context providers', () => {
  it('captures active control capabilities with an opaque stable identity', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const button = document.createElement('button');
    button.id = 'secret-control-id';
    button.textContent = 'secret visible text';
    button.setAttribute('aria-label', 'secret label');
    button.value = 'secret value';
    document.body.appendChild(button);
    button.focus();

    const first = ReadFocusContext();
    const second = ReadFocusContext();
    expect(first.hasFocus).toBe(true);
    expect(first.detached).toBe(false);
    expect(first.control?.capabilities).toMatchObject({
      isControl: true,
      button: true,
      editable: false,
      readOnly: false,
      disabled: false,
    });
    expect(first.composition).toBe('inactive');
    expect(first.control?.identity).toBe(second.control?.identity);
    expect(first.control?.identity).not.toContain('secret');
    expect(Object.isFrozen(first.control)).toBe(true);
    expect(Object.isFrozen(first.control?.capabilities)).toBe(true);
    expect(JSON.stringify(first)).not.toContain('secret');

    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    document.body.appendChild(checkbox);
    checkbox.focus();
    expect(ReadFocusContext().control?.capabilities).toMatchObject({
      input: true,
      isControl: true,
      editable: false,
    });

    const editingHost = document.createElement('section');
    editingHost.setAttribute('contenteditable', 'plaintext-only');
    const inheritedEditor = document.createElement('span');
    inheritedEditor.tabIndex = 0;
    editingHost.appendChild(inheritedEditor);
    document.body.appendChild(editingHost);
    inheritedEditor.focus();
    expect(ReadFocusContext().control?.capabilities).toMatchObject({
      contentEditable: true,
      editable: true,
    });
  });

  it('tracks IME conservatively, including lost composition notifications', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const release = acquireCommandFocusTracking();
    cleanups.push(release);

    expect(ReadFocusContext().composition).toBe('unknown');
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');
    input.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, isComposing: true }));
    expect(ReadFocusContext().composition).toBe('active');
    input.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, keyCode: 229 }));
    expect(ReadFocusContext().composition).toBe('active');
    // Sem composiçãoend, a leitura seguinte não inventa inactive.
    expect(ReadFocusContext().composition).toBe('active');
    input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('inactive');
  });

  it('trata blur como perda de prova e usa o DOM atual após reentrada', () => {
    const input = document.createElement('input');
    const button = document.createElement('button');
    document.body.append(input, button);
    const release = acquireCommandFocusTracking();
    cleanups.push(release);

    input.focus();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');
    button.focus();
    expect(ReadFocusContext().control?.capabilities.button).toBe(true);
    expect(ReadFocusContext().composition).toBe('inactive');

    let reentrant: ReturnType<typeof ReadFocusContext> | undefined;
    const onCompositionStart = () => {
      reentrant = ReadFocusContext();
    };
    input.addEventListener('compositionstart', onCompositionStart);
    input.focus();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    input.removeEventListener('compositionstart', onCompositionStart);
    expect(reentrant?.composition).toBe('active');
  });

  it('invalida composição no blur da janela mesmo sem blur do input', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const release = acquireCommandFocusTracking();
    cleanups.push(release);

    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('inactive');

    window.dispatchEvent(new Event('blur'));
    expect(document.activeElement).toBe(input);
    expect(ReadFocusContext().composition).toBe('unknown');
  });

  it('não aceita compositionend atrasado do elemento que perdeu o foco', () => {
    const first = document.createElement('input');
    const second = document.createElement('input');
    document.body.append(first, second);
    const release = acquireCommandFocusTracking();
    cleanups.push(release);

    first.focus();
    first.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');

    second.focus();
    expect(ReadFocusContext().composition).toBe('unknown');
    first.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));

    // O fim observado no input antigo não prova o estado do novo input.
    expect(ReadFocusContext().composition).toBe('unknown');
    second.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    second.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('inactive');
  });

  it('perde a evidência quando activeElement muda sem qualquer notificação', () => {
    const first = document.createElement('input');
    const second = document.createElement('input');
    document.body.append(first, second);
    const release = acquireCommandFocusTracking();
    cleanups.push(release);

    first.focus();
    first.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');

    const originalActiveElement = Object.getOwnPropertyDescriptor(document, 'activeElement');
    Object.defineProperty(document, 'activeElement', {
      configurable: true,
      get: () => second,
    });
    try {
      // Não houve focusout/focusin: a leitura deve comparar a fonte atual.
      expect(ReadFocusContext().composition).toBe('unknown');

      // Eventos do elemento antigo não podem marcar o novo foco como ativo.
      first.dispatchEvent(new CompositionEvent('compositionupdate', { bubbles: true }));
      first.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
      expect(ReadFocusContext().composition).toBe('unknown');
    } finally {
      if (originalActiveElement) {
        Object.defineProperty(document, 'activeElement', originalActiveElement);
      } else {
        delete (document as unknown as Record<string, unknown>).activeElement;
      }
    }
  });

  it('faz refcount, remove listeners no último release e reinstala em estado unknown', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const first = acquireCommandFocusTracking();
    const second = acquireCommandFocusTracking();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');
    first();
    input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('inactive');
    second();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('unknown');

    const reinstalled = acquireCommandFocusTracking();
    cleanups.push(reinstalled);
    expect(ReadFocusContext().composition).toBe('unknown');
  });

  it('dispose do tracker não deixa composição antiga nem listener eterno', () => {
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const release = acquireCommandFocusTracking();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('active');
    release();
    expect(ReadFocusContext().composition).toBe('unknown');
    input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('unknown');
  });

  it('does not expose a detached active element as a current control', () => {
    const detached = document.createElement('input');
    const descriptor = Object.getOwnPropertyDescriptor(document, 'activeElement');
    Object.defineProperty(document, 'activeElement', {
      configurable: true,
      get: () => detached,
    });
    const result = ReadFocusContext();
    expect(result.detached).toBe(true);
    expect(result.control).toBeNull();
    if (descriptor) {
      Object.defineProperty(document, 'activeElement', descriptor);
    } else {
      delete (document as unknown as Record<string, unknown>).activeElement;
    }
  });

  it('re-reads the getter, returns a detached clone, and protects the lease', () => {
    let current = context('surface-1', { title: 'first' });
    const oldCleanup = registerSurfaceContext('surface-1', () => current);
    cleanups.push(oldCleanup);

    const first = ReadSurfaceContext('surface-1');
    expect(first?.title).toBe('first');
    expect(Object.isFrozen(first)).toBe(true);
    try {
      (first as { title?: string }).title = 'mutated';
    } catch {
      // Strict-mode assignment to a frozen detached snapshot is expected.
    }
    current = context('surface-1', { title: 'second' });
    expect(ReadSurfaceContext('surface-1')?.title).toBe('second');

    const newCleanup = registerSurfaceContext('surface-1', () => context('surface-1', { title: 'new' }));
    cleanups.push(newCleanup);
    oldCleanup();
    expect(ReadSurfaceContext('surface-1')?.title).toBe('new');
  });

  it('reports missing, stale, and schema-invalid contexts closed', () => {
    expect(ReadSurfaceContextDetailed('missing')).toEqual({ status: 'missing', context: null });

    const staleCleanup = registerSurfaceContext('stale', () =>
      context('stale', {
        capturedAt: new Date(Date.now() - 10_000).toISOString(),
        staleAfterMs: 100,
      }),
    );
    cleanups.push(staleCleanup);
    expect(ReadSurfaceContextDetailed('stale')).toEqual({ status: 'stale', context: null });

    const invalidCleanup = registerSurfaceContext('invalid', () => ({
      ...context('invalid'),
      unknownPayloadField: 'not allowed',
    } as SurfaceContext));
    cleanups.push(invalidCleanup);
    expect(ReadSurfaceContextDetailed('invalid')).toEqual({ status: 'invalid', context: null });
    expect(ReadSurfaceContext('invalid')).toBeUndefined();

    const invalidDateCleanup = registerSurfaceContext('invalid-date', () =>
      context('invalid-date', { capturedAt: '2026-02-30T12:00:00Z' }),
    );
    cleanups.push(invalidDateCleanup);
    expect(ReadSurfaceContextDetailed('invalid-date').status).toBe('invalid');

    const futureCleanup = registerSurfaceContext('future', () =>
      context('future', { capturedAt: new Date(Date.now() + 60_000).toISOString() }),
    );
    cleanups.push(futureCleanup);
    expect(ReadSurfaceContextDetailed('future').status).toBe('invalid');

    const surrogateCleanup = registerSurfaceContext('surrogate', () =>
      context('surrogate', { snapshotVersion: '\uD800' }),
    );
    cleanups.push(surrogateCleanup);
    expect(ReadSurfaceContextDetailed('surrogate').status).toBe('invalid');
  });

  it('rejects cyclic surface payloads and never infers a surface from the active tab', () => {
    const cyclic = context('cyclic') as unknown as Record<string, unknown>;
    cyclic.metadata = cyclic;
    const cleanup = registerSurfaceContext('cyclic', () => cyclic as SurfaceContext);
    cleanups.push(cleanup);
    expect(ReadSurfaceContextDetailed('cyclic').status).toBe('invalid');
    expect(ReadSurfaceContext('unregistered-tab')).toBeUndefined();
  });

  it('detaches __proto__ keys without invoking a prototype setter', () => {
    const metadata = Object.create(null) as Record<string, unknown>;
    Object.defineProperty(metadata, '__proto__', {
      configurable: true,
      enumerable: true,
      value: { safe: true },
    });
    const cleanup = registerSurfaceContext('prototype-safe', () =>
      context('prototype-safe', { metadata }),
    );
    cleanups.push(cleanup);
    const result = ReadSurfaceContext('prototype-safe');
    expect(result?.metadata?.['__proto__']).toEqual({ safe: true });
    expect(Object.getPrototypeOf(result?.metadata)).toBeNull();
    expect(({} as Record<string, unknown>).polluted).toBeUndefined();
  });

  it('builds a typed read frame from the registry, focus, and explicit surface id', () => {
    const cleanup = registerSurfaceContext('surface-frame', () => context('surface-frame'));
    cleanups.push(cleanup);
    const frame = ReadCommandContextFrame('surface-frame');
    expect(frame.version).toBe(1);
    expect(frame.modal).toHaveProperty('topID');
    expect(frame.surface?.surfaceId).toBe('surface-frame');
    expect(frame.profile).toBeNull();
    expect(Object.isFrozen(frame)).toBe(true);
  });
});
