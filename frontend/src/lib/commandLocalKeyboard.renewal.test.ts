import { afterEach, describe, expect, it, vi } from 'vitest';
import { createLocalCommandKeyboard, type LocalCommandKeyboardMap } from './commandLocalKeyboard';

const shortcut = { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const] };
const map = (generation: string, commandId: string, validUntil?: number): LocalCommandKeyboardMap => ({
  generation,
  ...(validUntil === undefined ? {} : { validUntil }),
  bindings: [{ shortcut, commandId, handler: 'backend' }],
});
const keydown = () => window.dispatchEvent(new KeyboardEvent('keydown', {
  code: 'KeyK', key: 'k', ctrlKey: true, cancelable: true,
}));
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

describe('renew() do mapa de comandos local', () => {
  afterEach(() => { vi.useRealTimers(); });

  it('mantém o mapa antigo durante a preparação e publica o novo mapa de forma atômica', async () => {
    const pending = deferred<LocalCommandKeyboardMap>();
    const order: string[] = [];
    const reset = vi.fn(async (generation: string) => { order.push(`reset:${generation}`); });
    const onDown = vi.fn(async () => {});
    const accepted: string[] = [];
    const loadMap = vi.fn().mockResolvedValueOnce(map('old', 'command.old')).mockReturnValueOnce(pending.promise);
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset, blocked: () => false,
      onMapAccepted: value => { accepted.push(value.generation); order.push(`publish:${value.generation}`); },
    });
    try {
      await controller.refresh();
      const renewal = controller.renew();
      await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
      keydown();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'old', commandId: 'command.old' }));
      expect(reset).not.toHaveBeenCalled();

      pending.resolve(map('new', 'command.new'));
      await renewal;
      keydown();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'new', commandId: 'command.new' }));
      expect(accepted).toEqual(['old', 'new']);
      expect(reset).toHaveBeenCalledTimes(1);
      expect(reset).toHaveBeenCalledWith('old');
      expect(order.slice(-2)).toEqual(['publish:new', 'reset:old']);
    } finally { controller.dispose(); }
  });

  it('não reseta a geração quando a renovação aceita o mesmo generation', async () => {
    const reset = vi.fn(async (_generation: string) => {});
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const loadMap = vi.fn().mockResolvedValueOnce(map('same', 'command.old')).mockResolvedValueOnce(map('same', 'command.new'));
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp, reset, blocked: () => false,
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'same', commandId: 'command.old' }));
      reset.mockClear();
      await controller.renew();
      expect(reset).not.toHaveBeenCalled();
      window.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyK', key: 'k', ctrlKey: true }));
      await vi.waitFor(() => expect(onUp).toHaveBeenCalledWith(expect.objectContaining({ generation: 'same', commandId: 'command.old' })));
      expect(loadMap).toHaveBeenCalledTimes(2);
    } finally { controller.dispose(); }
  });

  it('descarta a tecla pressionada quando a renovação publica outra geração', async () => {
    const onDown = vi.fn(async () => {});
    const onUp = vi.fn(async () => {});
    const loadMap = vi.fn().mockResolvedValueOnce(map('old', 'command.old')).mockResolvedValueOnce(map('new', 'command.new'));
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp, reset: vi.fn(async () => {}), blocked: () => false,
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'old', commandId: 'command.old' }));
      await controller.renew();
      window.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyK', key: 'k', ctrlKey: true }));
      expect(onUp).not.toHaveBeenCalled();
      keydown();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'new', commandId: 'command.new' }));
    } finally { controller.dispose(); }
  });

  it('mantém o mapa vigente em falha de transporte, respeita blocked e repete com backoff', async () => {
    vi.useFakeTimers();
    const loadMap = vi.fn()
      .mockResolvedValueOnce(map('current', 'command.current'))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(map('renewed', 'command.renewed'));
    const blocked = vi.fn(() => true);
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked,
    });
    try {
      await controller.refresh();
      await controller.renew();
      keydown();
      expect(blocked).toHaveBeenCalledWith('command.current', expect.any(KeyboardEvent));
      expect(onDown).not.toHaveBeenCalled();
      blocked.mockReturnValue(false);
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'current', commandId: 'command.current' }));
      await vi.advanceTimersByTimeAsync(999);
      expect(loadMap).toHaveBeenCalledTimes(2);
      await vi.advanceTimersByTimeAsync(1);
      await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(3));
      keydown();
      expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'renewed', commandId: 'command.renewed' }));
    } finally { controller.dispose(); }
  });

  it('falha fechado quando acceptMap recusa a renovação', async () => {
    const onDown = vi.fn(async () => {});
    const reset = vi.fn(async (_generation: string) => {});
    const controller = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn().mockResolvedValueOnce(map('current', 'command.current')).mockResolvedValueOnce(map('rejected', 'command.rejected')),
      onDown, onUp: vi.fn(async () => {}), reset, blocked: () => false,
      acceptMap: candidate => candidate.generation === 'current',
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'current', commandId: 'command.current' }));
      onDown.mockClear();
      reset.mockClear();
      await controller.renew();
      keydown();
      expect(onDown).not.toHaveBeenCalled();
      expect(reset).toHaveBeenCalledWith('current');
    } finally { controller.dispose(); }
  });

  it.each([
    ['generation vazio', { generation: '', bindings: [] }],
    ['bindings malformado', { generation: 'bad', bindings: null }],
    ['prazo já expirado', { ...map('expired', 'command.expired'), validUntil: 0 }],
  ])('falha fechado para renovação com %s', async (_case, invalid) => {
    const onDown = vi.fn(async () => {});
    const reset = vi.fn(async (_generation: string) => {});
    const controller = createLocalCommandKeyboard({
      target: window,
      loadMap: vi.fn().mockResolvedValueOnce(map('current', 'command.current')).mockResolvedValueOnce(invalid as LocalCommandKeyboardMap),
      onDown, onUp: vi.fn(async () => {}), reset, blocked: () => false,
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'current', commandId: 'command.current' }));
      onDown.mockClear();
      reset.mockClear();
      await controller.renew();
      keydown();
      expect(onDown).not.toHaveBeenCalled();
      expect(reset).toHaveBeenCalledWith('current');
    } finally { controller.dispose(); }
  });

  it.each(['refresh', 'blur', 'dispose'] as const)('descarta renovação atrasada após %s', async action => {
    const pending = deferred<LocalCommandKeyboardMap>();
    const replacement = deferred<LocalCommandKeyboardMap>();
    const loadMap = vi.fn()
      .mockResolvedValueOnce(map('old', 'command.old'))
      .mockReturnValueOnce(pending.promise)
      .mockReturnValueOnce(replacement.promise);
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false,
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'old', commandId: 'command.old' }));
      onDown.mockClear();
      const renewal = controller.renew();
      await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
      let hardRefresh: Promise<void> | undefined;
      if (action === 'refresh') {
        hardRefresh = controller.refresh();
        await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(3));
        keydown();
        expect(onDown).not.toHaveBeenCalledWith(expect.objectContaining({ commandId: 'command.old' }));
      } else if (action === 'blur') {
        window.dispatchEvent(new FocusEvent('blur'));
      } else {
        controller.dispose();
      }
      pending.resolve(map('stale-renewal', 'command.stale'));
      await renewal;
      if (hardRefresh) {
        replacement.resolve(map('authoritative', 'command.authoritative'));
        await hardRefresh;
      }
      keydown();
      if (action === 'refresh') {
        expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ generation: 'authoritative', commandId: 'command.authoritative' }));
      } else {
        expect(onDown).not.toHaveBeenCalledWith(expect.objectContaining({ commandId: 'command.stale' }));
      }
      expect(onDown).not.toHaveBeenCalledWith(expect.objectContaining({ commandId: 'command.old' }));
    } finally { controller.dispose(); }
  });

  it.each([
    ['same generation', 'same', true],
    ['new generation', 'new', false],
  ] as const)('preserva a sequência em %s somente se a geração for igual', async (_case, nextGeneration, continues) => {
    const sequence = {
      version: 2 as const,
      steps: [{ code: 'KeyK', modifiers: ['Control'] as const }, { code: 'KeyN', modifiers: [] as const }],
    };
    const sequenceEvent = (code: string) => new KeyboardEvent('keydown', {
      code, key: code === 'KeyK' ? 'k' : 'n', ctrlKey: code === 'KeyK', cancelable: true,
    });
    const onDown = vi.fn(async () => {});
    const loadMap = vi.fn()
      .mockResolvedValueOnce({ generation: 'same', bindings: [{ shortcut: sequence, commandId: 'command.sequence', handler: 'backend' as const }] })
      .mockResolvedValueOnce({ generation: nextGeneration, bindings: [{ shortcut: sequence, commandId: 'command.sequence', handler: 'backend' as const }] });
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false,
    });
    try {
      await controller.refresh();
      window.dispatchEvent(sequenceEvent('KeyK'));
      await controller.renew();
      window.dispatchEvent(sequenceEvent('KeyN'));
      expect(onDown).toHaveBeenCalledTimes(continues ? 1 : 0);
    } finally { controller.dispose(); }
  });

  it.each([
    ['same generation', 'same', true],
    ['new generation', 'new', false],
  ] as const)('preserva o estado dos modificadores em %s somente se a geração for igual', async (_case, nextGeneration, executes) => {
    const controlAlt = { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const, 'Alt' as const] };
    const makeMap = (generation: string, commandId: string): LocalCommandKeyboardMap => ({
      generation, bindings: [{ shortcut: controlAlt, commandId, handler: 'backend' }],
    });
    const loadMap = vi.fn().mockResolvedValueOnce(makeMap('same', 'command.before')).mockResolvedValueOnce(makeMap(nextGeneration, 'command.after'));
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false,
    });
    try {
      await controller.refresh();
      window.dispatchEvent(new KeyboardEvent('keydown', { code: 'ControlLeft', ctrlKey: true }));
      window.dispatchEvent(new KeyboardEvent('keydown', { code: 'AltLeft', ctrlKey: true, altKey: true }));
      await controller.renew();
      window.dispatchEvent(new KeyboardEvent('keydown', {
        code: 'KeyK', key: 'k', ctrlKey: true, altKey: true, cancelable: true,
      }));
      expect(onDown).toHaveBeenCalledTimes(executes ? 1 : 0);
      if (executes) expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'same', commandId: 'command.after' }));
    } finally { controller.dispose(); }
  });

  it('não estende validUntil: o prazo vence enquanto renew está pendente', async () => {
    vi.useFakeTimers();
    const pending = deferred<LocalCommandKeyboardMap>();
    const deadline = Date.now() + 100;
    const loadMap = vi.fn().mockResolvedValueOnce(map('current', 'command.current', deadline)).mockReturnValueOnce(pending.promise);
    const onDown = vi.fn(async () => {});
    const controller = createLocalCommandKeyboard({
      target: window, loadMap, onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false,
    });
    try {
      await controller.refresh();
      keydown();
      expect(onDown).toHaveBeenCalledWith(expect.objectContaining({ generation: 'current', commandId: 'command.current' }));
      window.dispatchEvent(new KeyboardEvent('keyup', { code: 'KeyK', key: 'k', ctrlKey: true }));
      onDown.mockClear();
      const renewal = controller.renew();
      await vi.waitFor(() => expect(loadMap).toHaveBeenCalledTimes(2));
      vi.setSystemTime(deadline);
      keydown();
      expect(onDown).not.toHaveBeenCalled();
      pending.resolve(map('already-expired-renewal', 'command.late', deadline));
      await renewal;
      keydown();
      expect(onDown).not.toHaveBeenCalledWith(expect.objectContaining({ commandId: 'command.late' }));
    } finally { controller.dispose(); }
  });
});
