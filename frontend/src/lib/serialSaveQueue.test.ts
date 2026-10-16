import { describe, expect, it } from 'vitest';
import { enqueueSave, whenSavesSettled } from './serialSaveQueue';

function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const tick = () => new Promise<void>((r) => { setTimeout(r, 0); });

describe('serialSaveQueue', () => {
  it('executa os salvamentos da mesma chave um por vez, na ordem', async () => {
    const first = deferred();
    const order: string[] = [];
    const a = enqueueSave('k-ordem', async () => { order.push('a:start'); await first.promise; order.push('a:end'); });
    const b = enqueueSave('k-ordem', async () => { order.push('b'); });
    await tick();
    expect(order).toEqual(['a:start']);
    first.resolve();
    await Promise.all([a, b]);
    expect(order).toEqual(['a:start', 'a:end', 'b']);
  });

  it('uma falha rejeita só quem a enfileirou e não trava a fila', async () => {
    const failing = enqueueSave('k-falha', async () => { throw new Error('recusado'); });
    const next = enqueueSave('k-falha', async () => 'ok');
    await expect(failing).rejects.toThrow('recusado');
    await expect(next).resolves.toBe('ok');
  });

  it('whenSavesSettled espera os salvamentos pendentes, mesmo com falha', async () => {
    const pending = deferred();
    void enqueueSave('k-espera', () => pending.promise).catch(() => undefined);
    let settled = false;
    void whenSavesSettled('k-espera').then(() => { settled = true; });
    await tick();
    expect(settled).toBe(false);
    pending.reject(new Error('falhou'));
    await whenSavesSettled('k-espera');
    expect(settled).toBe(true);
  });

  it('chaves diferentes não esperam uma pela outra', async () => {
    const blocked = deferred();
    void enqueueSave('k-a', () => blocked.promise);
    await expect(enqueueSave('k-b', async () => 'livre')).resolves.toBe('livre');
    await whenSavesSettled('k-sem-fila');
    blocked.resolve();
  });
});
