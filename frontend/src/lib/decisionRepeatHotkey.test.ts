import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  createDecisionRepeatHotkeyCoordinator,
  type DecisionRepeatHotkeyAPI,
} from './decisionRepeatHotkey';
import type { ModalRegistrySnapshot } from './modalRegistry';

function modalSnapshot(topID: string | null): ModalRegistrySnapshot {
  return {
    generation: 'test:1',
    snapshotGeneration: 'test:1',
    generationNumber: 1,
    topID,
    ids: topID ? [topID] : [],
    dialogCommandScope: null,
    chatPresentationCommandIds: null,
  };
}

async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function harness(options: {
  topID?: string | null;
  api?: Partial<DecisionRepeatHotkeyAPI>;
  ownership?: Partial<{ isReady: () => boolean; owns: (event: KeyboardEvent) => boolean; dispose: () => void }>;
} = {}) {
  let topID: string | null = options.topID ?? 'decision-a';
  let eventListener: ((payload: unknown) => void) | undefined;
  let modalListener: (() => void) | undefined;
  const api: DecisionRepeatHotkeyAPI = {
    OpenDecisionRepeatHotkeySession: vi.fn(async () => 'session-1'),
    SetDecisionRepeatHotkey: vi.fn(async () => undefined),
    CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
    ...options.api,
  };
  const ownership = {
    isReady: vi.fn(() => true),
    owns: vi.fn(() => true),
    dispose: vi.fn(),
    ...options.ownership,
  };
  const coordinator = createDecisionRepeatHotkeyCoordinator({
    getAPI: () => api,
    subscribeEvent: vi.fn((_name, listener) => {
      eventListener = listener;
      return () => { eventListener = undefined; };
    }),
    subscribeModal: vi.fn((listener) => {
      modalListener = listener;
      return () => { modalListener = undefined; };
    }),
    readModal: () => modalSnapshot(topID),
    acquireOwnership: () => ownership,
    waitForBridge: async () => true,
  });
  return {
    api,
    ownership,
    coordinator,
    setTopID(next: string | null) {
      topID = next;
      modalListener?.();
    },
    emit(payload: unknown) {
      eventListener?.(payload);
    },
  };
}

afterEach(() => {
  vi.useRealTimers();
});

describe('decisionRepeatHotkey', () => {
  it('assina evento e stack antes de abrir a sessão', async () => {
    const calls: string[] = [];
    const api: DecisionRepeatHotkeyAPI = {
      OpenDecisionRepeatHotkeySession: vi.fn(async () => {
        calls.push('open');
        return 'session-1';
      }),
      SetDecisionRepeatHotkey: vi.fn(async () => undefined),
      CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
    };
    const event = vi.fn((_name: string, _listener: (payload: unknown) => void) => {
      calls.push('event');
      return () => undefined;
    });
    const stack = vi.fn((_listener: () => void) => {
      calls.push('stack');
      return () => undefined;
    });
    const coordinator = createDecisionRepeatHotkeyCoordinator({
      getAPI: () => api,
      subscribeEvent: event,
      subscribeModal: stack,
      readModal: () => modalSnapshot('decision-a'),
      acquireOwnership: () => ({ isReady: () => true, owns: () => true, dispose: () => undefined }),
      waitForBridge: async () => true,
    });

    coordinator.register('decision-a', vi.fn());
    await flush();
    expect(calls.indexOf('event')).toBeLessThan(calls.indexOf('open'));
    expect(calls.indexOf('stack')).toBeLessThan(calls.indexOf('open'));
    coordinator.dispose();
  });

  it('deriva o alvo do topmost real, bloqueia modal não-decisão e restaura o inferior', async () => {
    const h = harness();
    const lower = vi.fn();
    const upper = vi.fn();
    const releaseLower = h.coordinator.register('decision-a', lower);
    await flush();
    h.setTopID('decision-b');
    const releaseUpper = h.coordinator.register('decision-b', upper);
    await vi.waitFor(() => expect(h.api.SetDecisionRepeatHotkey).toHaveBeenCalled());
    expect(h.api.SetDecisionRepeatHotkey).toHaveBeenLastCalledWith(
      'session-1',
      expect.any(Number),
      expect.any(String),
    );

    h.setTopID('ordinary-modal');
    await flush();
    expect(h.api.CloseDecisionRepeatHotkeySession).toHaveBeenCalled();

    h.setTopID('decision-a');
    await vi.waitFor(() => expect(h.api.OpenDecisionRepeatHotkeySession).toHaveBeenCalledTimes(3));
    releaseUpper();
    releaseLower();
    h.coordinator.dispose();
  });

  it('fecha a sessão aberta quando o modal fecha durante Open', async () => {
    let resolveOpen!: (session: string) => void;
    const open = vi.fn(() => new Promise<string>((resolve) => { resolveOpen = resolve; }));
    const h = harness({ api: { OpenDecisionRepeatHotkeySession: open } });
    const release = h.coordinator.register('decision-a', vi.fn());
    await flush();
    release();
    resolveOpen('late-session');
    await flush();
    expect(h.api.SetDecisionRepeatHotkey).not.toHaveBeenCalled();
    expect(h.api.CloseDecisionRepeatHotkeySession).toHaveBeenCalledWith('late-session');
    h.coordinator.dispose();
  });

  it('torna unsupported sem loop quando o backend retorna sessão vazia', async () => {
    const open = vi.fn(async () => '');
    const h = harness({ api: { OpenDecisionRepeatHotkeySession: open } });
    h.coordinator.register('decision-a', vi.fn());
    await flush();
    await vi.waitFor(() => expect(open).toHaveBeenCalledOnce());
    expect(h.api.SetDecisionRepeatHotkey).not.toHaveBeenCalled();
    h.coordinator.dispose();
  });

  it('encerra retries de transporte e só tenta novamente com nova ativação', async () => {
    vi.useFakeTimers();
    const open = vi.fn(async () => { throw new Error('transport'); });
    const h = harness({ api: { OpenDecisionRepeatHotkeySession: open } });
    const release = h.coordinator.register('decision-a', vi.fn());
    await flush();
    await vi.runAllTimersAsync();
    expect(open).toHaveBeenCalledTimes(4);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(open).toHaveBeenCalledTimes(4);
    release();
    h.setTopID('ordinary-modal');
    h.setTopID('decision-a');
    h.coordinator.register('decision-a', vi.fn());
    await vi.runAllTimersAsync();
    expect(open.mock.calls.length).toBeGreaterThan(4);
    h.coordinator.dispose();
  });

  it('espera o bridge inicial antes de concluir fallback unsupported', async () => {
    let resolveBridge!: (available: boolean) => void;
    let api: DecisionRepeatHotkeyAPI | undefined;
    const open = vi.fn(async () => 'session-1');
    const h = harness();
    h.coordinator.dispose();
    const coordinator = createDecisionRepeatHotkeyCoordinator({
      getAPI: () => api,
      subscribeEvent: () => () => undefined,
      subscribeModal: () => () => undefined,
      readModal: () => modalSnapshot('decision-a'),
      acquireOwnership: () => ({ isReady: () => true, owns: () => true, dispose: () => undefined }),
      waitForBridge: () => new Promise((resolve) => { resolveBridge = resolve; }),
    });
    coordinator.register('decision-a', vi.fn());
    await flush();
    expect(open).not.toHaveBeenCalled();
    api = { OpenDecisionRepeatHotkeySession: open, SetDecisionRepeatHotkey: vi.fn(async () => undefined), CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined) };
    resolveBridge(true);
    await flush();
    expect(open).toHaveBeenCalledOnce();
    coordinator.dispose();
  });

  it('não inicia bootstrap sem cleanup de evento e não agenda loop', async () => {
    vi.useFakeTimers();
    const open = vi.fn(async () => 'session-1');
    const coordinator = createDecisionRepeatHotkeyCoordinator({
      getAPI: () => ({
        OpenDecisionRepeatHotkeySession: open,
        SetDecisionRepeatHotkey: vi.fn(async () => undefined),
        CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
      }),
      subscribeEvent: () => undefined,
      subscribeModal: () => () => undefined,
      readModal: () => modalSnapshot('decision-a'),
      acquireOwnership: () => ({ isReady: () => true, owns: () => true, dispose: () => undefined }),
      waitForBridge: async () => true,
    });

    coordinator.register('decision-a', vi.fn());
    await flush();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(open).not.toHaveBeenCalled();
    coordinator.dispose();
  });

  it('limpa a assinatura de evento se a assinatura da stack falhar', async () => {
    const eventCleanup = vi.fn();
    const open = vi.fn(async () => 'session-1');
    const coordinator = createDecisionRepeatHotkeyCoordinator({
      getAPI: () => ({
        OpenDecisionRepeatHotkeySession: open,
        SetDecisionRepeatHotkey: vi.fn(async () => undefined),
        CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
      }),
      subscribeEvent: () => eventCleanup,
      subscribeModal: () => { throw new Error('stack subscription failed'); },
      readModal: () => modalSnapshot('decision-a'),
      acquireOwnership: () => ({ isReady: () => true, owns: () => true, dispose: () => undefined }),
      waitForBridge: async () => true,
    });

    coordinator.register('decision-a', vi.fn());
    await flush();
    expect(eventCleanup).toHaveBeenCalledOnce();
    expect(open).not.toHaveBeenCalled();
    coordinator.dispose();
  });

  it('não reserva sem lease de ownership válido', async () => {
    const open = vi.fn(async () => 'session-1');
    const coordinator = createDecisionRepeatHotkeyCoordinator({
      getAPI: () => ({
        OpenDecisionRepeatHotkeySession: open,
        SetDecisionRepeatHotkey: vi.fn(async () => undefined),
        CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
      }),
      subscribeEvent: () => () => undefined,
      subscribeModal: () => () => undefined,
      readModal: () => modalSnapshot('decision-a'),
      acquireOwnership: () => { throw new Error('ownership unavailable'); },
      waitForBridge: async () => true,
    });

    coordinator.register('decision-a', vi.fn());
    await flush();
    expect(open).not.toHaveBeenCalled();
    coordinator.dispose();
  });

  it('honra ownership durante Set pendente e entrega apenas a repetição nativa', async () => {
    let resolveSet!: () => void;
    const set = vi.fn(() => new Promise<void>((resolve) => { resolveSet = resolve; }));
    const h = harness({ api: { SetDecisionRepeatHotkey: set } });
    const repeat = vi.fn();
    h.coordinator.register('decision-a', repeat);
    await vi.waitFor(() => expect(set).toHaveBeenCalled());
    const key = new KeyboardEvent('keydown', { key: 'r', ctrlKey: true, shiftKey: true });
    expect(h.coordinator.ownsNative(key)).toBe(true);
    resolveSet();
    await flush();
    const [, revision, dialogId] = (h.api.SetDecisionRepeatHotkey as ReturnType<typeof vi.fn>).mock.calls[0];
    h.emit({ sessionId: 'session-1', revision, dialogId });
    expect(repeat).toHaveBeenCalledOnce();
    h.coordinator.dispose();
  });

  it('mantém ownership até fechar a reserva durante dispose', async () => {
    let resolveSet!: () => void;
    const set = vi.fn(() => new Promise<void>((resolve) => { resolveSet = resolve; }));
    const close = vi.fn(async () => undefined);
    const h = harness({ api: { SetDecisionRepeatHotkey: set, CloseDecisionRepeatHotkeySession: close } });
    h.coordinator.register('decision-a', vi.fn());
    await vi.waitFor(() => expect(set).toHaveBeenCalled());
    h.coordinator.dispose();
    expect(h.ownership.dispose).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
    resolveSet();
    await vi.waitFor(() => expect(close).toHaveBeenCalledWith('session-1'));
    await flush();
    expect(h.ownership.dispose).toHaveBeenCalledOnce();
  });

  it('não perde o close quando dispose ocorre durante heartbeat Set', async () => {
    vi.useFakeTimers();
    let resolveHeartbeat!: () => void;
    let setCalls = 0;
    const set = vi.fn(() => {
      setCalls += 1;
      if (setCalls === 1) return Promise.resolve();
      return new Promise<void>((resolve) => { resolveHeartbeat = resolve; });
    });
    const close = vi.fn(async () => undefined);
    const h = harness({ api: { SetDecisionRepeatHotkey: set, CloseDecisionRepeatHotkeySession: close } });
    h.coordinator.register('decision-a', vi.fn());
    await vi.waitFor(() => expect(set).toHaveBeenCalledOnce());
    await vi.advanceTimersByTimeAsync(8_000);
    await vi.waitFor(() => expect(set).toHaveBeenCalledTimes(2));

    h.coordinator.dispose();
    expect(h.ownership.dispose).not.toHaveBeenCalled();
    resolveHeartbeat();
    await vi.waitFor(() => expect(close).toHaveBeenCalledWith('session-1'));
    await vi.runAllTimersAsync();
    expect(h.ownership.dispose).toHaveBeenCalledOnce();
  });

  it('recusa evento atrasado, input/IME focado e permite fora da janela', async () => {
    const h = harness();
    const repeat = vi.fn();
    h.coordinator.register('decision-a', repeat);
    await vi.waitFor(() => expect(h.api.SetDecisionRepeatHotkey).toHaveBeenCalled());
    const [, revision, dialogId] = (h.api.SetDecisionRepeatHotkey as ReturnType<typeof vi.fn>).mock.calls[0];
    const validEvent = { sessionId: 'session-1', revision, dialogId };
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    h.emit({ sessionId: 'old', revision, dialogId });
    expect(repeat).not.toHaveBeenCalled();

    h.emit(validEvent);
    expect(repeat).not.toHaveBeenCalled();

    input.blur();
    document.dispatchEvent(new Event('compositionstart'));
    h.emit(validEvent);
    expect(repeat).not.toHaveBeenCalled();

    document.dispatchEvent(new Event('compositionend'));
    h.emit(validEvent);
    expect(repeat).toHaveBeenCalledOnce();

    input.focus();
    vi.spyOn(document, 'hasFocus').mockReturnValue(false);
    h.emit(validEvent);
    expect(repeat).toHaveBeenCalledTimes(2);

    h.setTopID('ordinary-modal');
    await vi.waitFor(() => expect(h.api.CloseDecisionRepeatHotkeySession).toHaveBeenCalled());
    h.setTopID('decision-a');
    await vi.waitFor(() => expect(h.api.SetDecisionRepeatHotkey).toHaveBeenCalledTimes(2));
    h.emit(validEvent);
    expect(repeat).toHaveBeenCalledTimes(2);

    input.remove();
    h.coordinator.dispose();
  });
});
