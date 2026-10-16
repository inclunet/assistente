import { describe, expect, it, vi } from 'vitest';
import { createCommandGlobalOwnership } from './commandGlobalOwnership';

const INSTANCE = '123e4567-e89b-12d3-a456-426614174000';
const OTHER_INSTANCE = '123e4567-e89b-12d3-a456-426614174001';

function frame(revision: number, combinations: Array<{ key: number; modifiers: number }> = [{ key: 65, modifiers: 2 }], instanceId = INSTANCE) {
  return { version: 1, instanceId, revision, platform: 'windows', combinations };
}

function keyEvent(type: 'keydown' | 'keyup', keyCode: number, init: KeyboardEventInit = {}): KeyboardEvent {
  const event = new KeyboardEvent(type, { cancelable: true, ...init });
  Object.defineProperty(event, 'keyCode', { configurable: true, value: keyCode });
  return event;
}

describe('command global ownership', () => {
  it('copia o snapshot e aplica ACK/change sincronamente', () => {
    const ack = vi.fn();
    const onChange = vi.fn();
    const controller = createCommandGlobalOwnership({ ack, onChange });
    const snapshot = frame(1);

    expect(controller.applyFrame(snapshot)).toBe(true);
    snapshot.combinations[0].modifiers = 0;

    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 65))).toBe(false);
    expect(ack).toHaveBeenCalledOnce();
    expect(ack).toHaveBeenCalledWith({ instanceId: INSTANCE, revision: 1 });
    expect(onChange).toHaveBeenCalledOnce();
  });

  it('rejeita revisão stale e outra instância sem perder o snapshot aceito', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    expect(controller.applyFrame(frame(3))).toBe(true);
    expect(controller.applyFrame(frame(2, [{ key: 65, modifiers: 0 }]))).toBe(false);
    expect(controller.applyFrame(frame(4, [{ key: 65, modifiers: 0 }], OTHER_INSTANCE))).toBe(false);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(true);
  });

  it('aceita máscaras distintas para o mesmo VK e deduplica pares exatos', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    expect(controller.applyFrame(frame(1, [
      { key: 65, modifiers: 1 },
      { key: 65, modifiers: 2 },
      { key: 65, modifiers: 2 },
    ]))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 65, { altKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(true);
  });

  it('usa keyCode como VK lógico e ignora event.code', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    controller.applyFrame(frame(1, [{ key: 65, modifiers: 2 }]));
    expect(controller.owns(keyEvent('keydown', 65, { code: 'KeyQ', ctrlKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 81, { code: 'KeyA', ctrlKey: true }))).toBe(false);
  });

  it('compara exatamente as máscaras Win/Alt/Ctrl/Shift/Meta', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    controller.applyFrame(frame(1, [
      { key: 65, modifiers: 1 },
      { key: 66, modifiers: 2 },
      { key: 67, modifiers: 4 },
      { key: 68, modifiers: 8 },
      { key: 69, modifiers: 15 },
    ]));
    expect(controller.owns(keyEvent('keydown', 65, { altKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 66, { ctrlKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 67, { shiftKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 68, { metaKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 69, { altKey: true, ctrlKey: true, shiftKey: true, metaKey: true }))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 69, { ctrlKey: true }))).toBe(false);
  });

  it('limpa por frame vazio e não trata IME 229 como tecla', () => {
    const onChange = vi.fn();
    const controller = createCommandGlobalOwnership({ ack: vi.fn(), onChange });
    controller.applyFrame(frame(1));
    expect(controller.applyFrame(frame(2, []))).toBe(true);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(false);
    expect(controller.applyFrame(frame(3))).toBe(true);
    expect(controller.applyFrame(frame(4, []))).toBe(true);
    expect(controller.owns(keyEvent('keyup', 65, { ctrlKey: true }))).toBe(false);
    expect(controller.owns(keyEvent('keydown', 229, { ctrlKey: true }))).toBe(false);
    expect(onChange).toHaveBeenCalledTimes(4);
  });

  it('falha fechado para snapshots malformados e preserva o estado anterior', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    controller.applyFrame(frame(1));
    expect(controller.applyFrame({ ...frame(2), revision: 0 })).toBe(false);
    expect(controller.applyFrame({ ...frame(2), combinations: [{ key: 0, modifiers: 2 }] })).toBe(false);
    expect(controller.applyFrame({ ...frame(2), extra: true })).toBe(false);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(true);
  });

  it('fixa a instância até o fim do lifecycle e encerra o controller', () => {
    const controller = createCommandGlobalOwnership({ ack: vi.fn() });
    controller.applyFrame(frame(1));
    expect(controller.applyFrame(frame(1, [], OTHER_INSTANCE))).toBe(false);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(true);
    controller.dispose();
    expect(controller.applyFrame(frame(2))).toBe(false);
    expect(controller.owns(keyEvent('keydown', 65, { ctrlKey: true }))).toBe(false);
  });
});
