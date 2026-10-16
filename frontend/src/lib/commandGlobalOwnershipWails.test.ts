import { describe, expect, it, vi } from 'vitest';
import { acquireGlobalCommandOwnership, connectGlobalCommandOwnership } from './commandGlobalOwnershipWails';
import type { GlobalCommandOwnershipConnectionOptions } from './commandGlobalOwnershipWails';

const INSTANCE = '123e4567-e89b-12d3-a456-426614174000';
const OTHER_INSTANCE = '123e4567-e89b-12d3-a456-426614174001';

function frame(revision: number, instanceId = INSTANCE, combinations: Array<{ key: number; modifiers: number }> = [{ key: 65, modifiers: 2 }]) {
  return { version: 1, instanceId, revision, platform: 'windows', combinations };
}

function keyEvent(keyCode: number, init: KeyboardEventInit = {}): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, ...init });
  Object.defineProperty(event, 'keyCode', { configurable: true, value: keyCode });
  return event;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

function harness(snapshot: Promise<unknown> | unknown) {
  let listener: ((raw: unknown) => void) | undefined;
  const unsubscribe = vi.fn();
  const subscribe = vi.fn((next: (raw: unknown) => void) => {
    listener = next;
    return unsubscribe;
  });
  const readSnapshot = vi.fn(async () => snapshot);
  const acknowledge = vi.fn(async () => true);
  const connection = connectGlobalCommandOwnership({ subscribe, readSnapshot, acknowledge });
  return {
    connection,
    subscribe,
    readSnapshot,
    acknowledge,
    unsubscribe,
    emit: async (raw: unknown) => { listener?.(raw); await flush(); },
  };
}

describe('commandGlobalOwnershipWails', () => {
  it('isola falhas de listeners e retira somente a inscrição do consumidor encerrado', async () => {
    let listener!: (raw: unknown) => void;
    const options = {
      subscribe: (next: (raw: unknown) => void) => { listener = next; return () => {}; },
      readSnapshot: async () => frame(0, INSTANCE, []),
      acknowledge: vi.fn(async () => true),
    };
    const changes = vi.fn();
    const broken = acquireGlobalCommandOwnership({ ...options, onChange: () => { throw new Error('consumer'); } });
    const first = acquireGlobalCommandOwnership({ ...options, onChange: changes });
    const second = acquireGlobalCommandOwnership({ ...options, onChange: changes });
    try {
      await broken.ready;
      first.dispose();
      listener(frame(1));
      expect(changes).toHaveBeenCalledOnce();
      expect(second.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
      expect(options.acknowledge).toHaveBeenCalledWith(INSTANCE, 1);
    } finally { first.dispose(); second.dispose(); broken.dispose(); }
  });
  it('instala a subscrição antes do getter, faz bootstrap rev0 e aplica frame buffered antes do ACK', async () => {
    const snapshot = deferred<unknown>();
    const ack = deferred<boolean>();
    let listener: ((raw: unknown) => void) | undefined;
    const subscribe = vi.fn((next: (raw: unknown) => void) => {
      listener = next;
      return vi.fn();
    });
    const readSnapshot = vi.fn(async () => snapshot.promise);
    const acknowledge = vi.fn(() => ack.promise);
    const connection = connectGlobalCommandOwnership({ subscribe, readSnapshot, acknowledge });

    expect(subscribe.mock.invocationCallOrder[0]).toBeLessThan(readSnapshot.mock.invocationCallOrder[0]);
    listener?.(frame(1));
    await flush();
    expect(connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(false);

    snapshot.resolve(frame(0, INSTANCE, []));
    await connection.ready;
    expect(connection.isReady()).toBe(true);
    expect(connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
    expect(acknowledge).toHaveBeenCalledWith(INSTANCE, 1);

    ack.resolve(true);
    await flush();
    connection.dispose();
  });

  it('mantém o modo local para snapshot explicitamente não suportado e não envia ACK', async () => {
    const { connection, acknowledge } = harness({ version: 1, instanceId: INSTANCE, revision: 0, platform: 'unsupported', combinations: [] });
    await connection.ready;

    expect(connection.isReady()).toBe(true);
    expect(connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(false);
    expect(acknowledge).not.toHaveBeenCalled();
    connection.dispose();
  });

  it('permanece não-ready para uma plataforma desconhecida', async () => {
    const { connection, acknowledge } = harness({ version: 1, instanceId: INSTANCE, revision: 0, platform: 'linux', combinations: [] });
    await connection.ready;

    expect(connection.isReady()).toBe(false);
    expect(connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(false);
    expect(acknowledge).not.toHaveBeenCalled();
    connection.dispose();
  });

  it('permanece não pronto quando o bootstrap falha', async () => {
    const failure = new Error('bridge unavailable');
    const { connection } = harness(Promise.reject(failure));
    await connection.ready.catch(() => undefined);
    expect(connection.isReady()).toBe(false);
    connection.dispose();
  });

  it('recupera em um novo lifecycle depois de uma falha transitória de leitura', async () => {
    vi.useFakeTimers();
    try {
      const readSnapshot = vi.fn()
        .mockRejectedValueOnce(new Error('transient bridge failure'))
        .mockResolvedValueOnce(frame(0, INSTANCE, []));
      const connection = connectGlobalCommandOwnership({
        subscribe: vi.fn(() => vi.fn()),
        readSnapshot,
        acknowledge: vi.fn(async () => true),
      });

      await connection.ready;
      expect(connection.isReady()).toBe(false);
      expect(readSnapshot).toHaveBeenCalledOnce();
      await vi.advanceTimersByTimeAsync(999);
      expect(readSnapshot).toHaveBeenCalledOnce();
      await vi.advanceTimersByTimeAsync(1);
      expect(connection.isReady()).toBe(true);
      expect(readSnapshot).toHaveBeenCalledTimes(2);
      connection.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it('cancela o retry de bootstrap ao fazer dispose', async () => {
    vi.useFakeTimers();
    try {
      const readSnapshot = vi.fn(async () => { throw new Error('bridge unavailable'); });
      const connection = connectGlobalCommandOwnership({
        subscribe: vi.fn(() => vi.fn()),
        readSnapshot,
        acknowledge: vi.fn(async () => true),
      });

      await connection.ready;
      connection.dispose();
      await vi.advanceTimersByTimeAsync(1000);
      expect(readSnapshot).toHaveBeenCalledOnce();
    } finally {
      vi.useRealTimers();
    }
  });

  it('falha fechado com API Wails incompleta e recupera no retry após a API aparecer', async () => {
    vi.useFakeTimers();
    try {
      let available = false;
      const acknowledge = vi.fn(async () => true);
      const app = {
        GetGlobalCommandOwnership: vi.fn(async () => frame(0, INSTANCE, [])),
        get AckGlobalCommandOwnership() {
          return available ? acknowledge : undefined;
        },
      };
      const target = { go: { app: { App: app } } } as unknown as Window;
      const connection = connectGlobalCommandOwnership({ target, subscribe: vi.fn(() => vi.fn()) });

      await connection.ready;
      expect(connection.isReady()).toBe(false);
      expect(app.GetGlobalCommandOwnership).not.toHaveBeenCalled();

      available = true;
      await vi.advanceTimersByTimeAsync(1000);
      expect(connection.isReady()).toBe(true);
      expect(app.GetGlobalCommandOwnership).toHaveBeenCalledOnce();
      connection.dispose();
    } finally {
      vi.useRealTimers();
    }
  });

  it('usa o port Wails padrão, instala o frame antes do ACK e chama os métodos corretos', async () => {
    let listener: ((raw: unknown) => void) | undefined;
    const subscribe = vi.fn((next: (raw: unknown) => void) => {
      listener = next;
      return vi.fn();
    });
    const app = {
      GetGlobalCommandOwnership: vi.fn(async () => frame(0, INSTANCE, [])),
      AckGlobalCommandOwnership: vi.fn(async () => true),
    };
    const target = { go: { app: { App: app } } } as unknown as NonNullable<GlobalCommandOwnershipConnectionOptions['target']>;
    let connection: ReturnType<typeof connectGlobalCommandOwnership>;
    app.AckGlobalCommandOwnership.mockImplementation(async () => {
      expect(connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
      return true;
    });
    connection = connectGlobalCommandOwnership({ target, subscribe });

    await connection.ready;
    listener?.(frame(1));
    await flush();

    expect(app.GetGlobalCommandOwnership).toHaveBeenCalledOnce();
    expect(app.AckGlobalCommandOwnership).toHaveBeenCalledWith(INSTANCE, 1);
    connection.dispose();
  });

  it('não envia ACK se o port padrão estiver aguardando o bridge quando ocorre dispose', async () => {
    let listener: ((raw: unknown) => void) | undefined;
    const subscribe = vi.fn((next: (raw: unknown) => void) => {
      listener = next;
      return vi.fn();
    });
    const app = {
      GetGlobalCommandOwnership: vi.fn(async () => frame(0, INSTANCE, [])),
      AckGlobalCommandOwnership: vi.fn(async () => true),
    };
    const target = { go: { app: { App: app } } } as unknown as NonNullable<GlobalCommandOwnershipConnectionOptions['target']>;
    const bridge = target.go;
    const connection = connectGlobalCommandOwnership({ target, subscribe });
    await connection.ready;

    target.go = undefined;
    listener?.(frame(1));
    connection.dispose();
    target.go = bridge;
    await flush();

    expect(app.AckGlobalCommandOwnership).not.toHaveBeenCalled();
  });

  it('não chama getter nem ACK depois de dispose enquanto aguarda o bridge Wails', async () => {
    vi.useFakeTimers();
    try {
      const app = {
        GetGlobalCommandOwnership: vi.fn(async () => frame(0, INSTANCE, [])),
        AckGlobalCommandOwnership: vi.fn(async () => true),
      };
      const target = {} as NonNullable<GlobalCommandOwnershipConnectionOptions['target']>;
      const connection = connectGlobalCommandOwnership({
        target,
        subscribe: vi.fn(() => vi.fn()),
      });

      connection.dispose();
      await connection.ready;
      await vi.runAllTimersAsync();

      expect(app.GetGlobalCommandOwnership).not.toHaveBeenCalled();
      expect(app.AckGlobalCommandOwnership).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('ignora revisions stale e frames de outra instância', async () => {
    const h = harness(frame(0, INSTANCE, []));
    await h.connection.ready;
    await h.emit(frame(2));
    await h.emit(frame(1, INSTANCE, [{ key: 65, modifiers: 1 }]));
    await h.emit(frame(3, OTHER_INSTANCE));

    expect(h.connection.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
    expect(h.connection.owns(keyEvent(65, { altKey: true }))).toBe(false);
    expect(h.acknowledge).toHaveBeenCalledTimes(1);
    h.connection.dispose();
  });

  it('não envia ACK tardio nem reaplica eventos depois de dispose', async () => {
    const snapshot = deferred<unknown>();
    const h = harness(snapshot.promise);
    h.connection.dispose();
    snapshot.resolve(frame(0, INSTANCE, []));
    await h.connection.ready.catch(() => undefined);
    await h.emit(frame(1));
    expect(h.connection.isReady()).toBe(false);
    expect(h.acknowledge).not.toHaveBeenCalled();
    expect(h.unsubscribe).toHaveBeenCalledOnce();
  });

  it('compartilha uma única subscrição e getter, retém o estado até o último consumidor e limpa depois', async () => {
    const snapshot = deferred<unknown>();
    let listener: ((raw: unknown) => void) | undefined;
    const unsubscribe = vi.fn();
    const subscribe = vi.fn((next: (raw: unknown) => void) => {
      listener = next;
      return unsubscribe;
    });
    const readSnapshot = vi.fn(async () => snapshot.promise);
    const acknowledge = vi.fn(async () => true);
    const firstChange = vi.fn();
    const secondChange = vi.fn();
    const first = acquireGlobalCommandOwnership({ subscribe, readSnapshot, acknowledge, onChange: firstChange });
    const second = acquireGlobalCommandOwnership({ subscribe, readSnapshot, acknowledge, onChange: secondChange });

    expect(subscribe).toHaveBeenCalledOnce();
    expect(readSnapshot).toHaveBeenCalledOnce();
    expect(first.isReady()).toBe(false);
    expect(first.owns(keyEvent(65, { ctrlKey: true }))).toBe(false);

    snapshot.resolve(frame(0, INSTANCE, []));
    await first.ready;
    expect(first.isReady()).toBe(true);
    listener?.(frame(1));
    await flush();
    expect(first.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
    expect(second.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);
    expect(firstChange).toHaveBeenCalledOnce();
    expect(secondChange).toHaveBeenCalledOnce();
    expect(firstChange.mock.invocationCallOrder[0]).toBeLessThan(acknowledge.mock.invocationCallOrder[0]);

    first.dispose();
    expect(unsubscribe).not.toHaveBeenCalled();
    expect(second.isReady()).toBe(true);
    expect(second.owns(keyEvent(65, { ctrlKey: true }))).toBe(true);

    second.dispose();
    expect(unsubscribe).toHaveBeenCalledOnce();
    expect(second.isReady()).toBe(false);
    expect(second.owns(keyEvent(65, { ctrlKey: true }))).toBe(false);
  });
});
