import { describe, expect, it, vi } from 'vitest';
import { apidto } from '@wailsjs/go/models';
import { TimelineRecoveryReader, type TimelineRecoveryTransport } from './timelineRecoveryReader';

const reference = { projectionId: 'projection', incarnation: 'storage', turnId: 'turn', executionId: 'execution' };
const chunk = (data: Partial<apidto.TimelineRecoveryChunk> = {}) =>
  new apidto.TimelineRecoveryChunk({ iteration: 1, generation: '7', part: 0, kind: 'text', byteOffset: 0, text: 'a🙂', ...data });
const page = (data: Partial<apidto.TimelineRecoveryPage> = {}) =>
  new apidto.TimelineRecoveryPage({ conversationId: 'chat', reference, found: true, contentVersion: '8',
    chunks: [chunk()], nextCursor: 'next', ...data });
const receipt = () => new apidto.TimelineRecoveryReservation({ conversationId: 'chat', reference, contentVersion: '8' });
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
function fixture() {
  let active = true;
  const transport = {
    readRecovery: vi.fn<TimelineRecoveryTransport['readRecovery']>()
      .mockResolvedValueOnce(page()).mockResolvedValue(page({ chunks: [chunk({ byteOffset: 5, text: 'fim' })], nextCursor: '' })),
    reserveRecovery: vi.fn<TimelineRecoveryTransport['reserveRecovery']>().mockResolvedValue(receipt()),
    releaseRecovery: vi.fn<TimelineRecoveryTransport['releaseRecovery']>().mockResolvedValue(undefined),
  };
  const closeWindow = vi.fn().mockResolvedValue(undefined);
  const released = vi.fn();
  const reader = new TimelineRecoveryReader(transport, 'chat', 'window', reference, () => active, closeWindow, released);
  return { reader, transport, closeWindow, released, deactivate: () => { active = false; reader.invalidate(); } };
}

describe('TimelineRecoveryReader', () => {
  it('reserva a versão lida e fornece páginas sob demanda com offsets UTF-8', async () => {
    const f = fixture();
    await f.reader.open();
    expect(f.transport.readRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '');
    expect(f.transport.reserveRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '8');
    expect((await f.reader.next())?.chunks[0].text).toBe('a🙂');
    expect((await f.reader.next())?.chunks[0].text).toBe('fim');
    expect(await f.reader.next()).toBeUndefined();
    expect(f.transport.releaseRecovery).not.toHaveBeenCalled();
    await Promise.all([f.reader.close(), f.reader.close()]);
    expect(f.transport.releaseRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '8');
    expect(f.released).toHaveBeenCalledOnce();
    await expect(f.reader.next()).rejects.toThrow('closed');
  });

  it('aceita páginas vazias que avançam sem inventar conteúdo ou fim', async () => {
    const f = fixture();
    f.transport.readRecovery.mockReset().mockResolvedValueOnce(page({ chunks: [] }))
      .mockResolvedValueOnce(page({ chunks: [], nextCursor: 'last' }))
      .mockResolvedValueOnce(page({ nextCursor: '' }));
    await f.reader.open();
    expect((await f.reader.next())?.chunks).toEqual([]);
    expect((await f.reader.next())?.chunks).toEqual([]);
    expect((await f.reader.next())?.chunks[0].text).toBe('a🙂');
    expect(await f.reader.next()).toBeUndefined();
    await f.reader.close();
  });

  it.each([
    ['conversa', () => page({ conversationId: 'other' })],
    ['referência', () => page({ reference: { ...reference, executionId: 'other' } as apidto.TimelineRecoveryReference })],
    ['versão', () => page({ contentVersion: '9' })],
    ['offset', () => page({ chunks: [chunk({ byteOffset: 4 })] })],
    ['geração', () => page({ chunks: [chunk({ generation: '9', byteOffset: 5 })] })],
    ['ordem', () => page({ chunks: [chunk({ kind: 'reasoning' })] })],
    ['UTF-8', () => page({ chunks: [chunk({ byteOffset: 5, text: '\ud800' })] })],
    ['bytes', () => page({ chunks: [chunk({ byteOffset: 5, text: '🙂'.repeat(32768) })] })],
    ['chunks', () => page({ chunks: Array.from({ length: 129 }, () => chunk()) })],
    ['cursor repetido', () => page({ chunks: [] })],
    ['cursor grande', () => page({ chunks: [], nextCursor: 'a'.repeat(16385) })],
    ['ausência', () => page({ found: false, chunks: [], contentVersion: '', nextCursor: '' })],
  ])('recusa página adulterada (%s) e libera a versão adquirida', async (_name, invalid) => {
    const f = fixture();
    f.transport.readRecovery.mockReset().mockResolvedValueOnce(page()).mockResolvedValueOnce(invalid());
    await f.reader.open();
    await f.reader.next();
    await expect(f.reader.next()).rejects.toThrow('Invalid');
    expect(f.transport.releaseRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '8');
  });

  it('ausência inicial não cria reserva nem autoriza descarte', async () => {
    const f = fixture();
    f.transport.readRecovery.mockReset().mockResolvedValue(page({ found: false, chunks: [], contentVersion: '', nextCursor: '' }));
    await expect(f.reader.open()).rejects.toThrow('Invalid');
    expect(f.transport.reserveRecovery).not.toHaveBeenCalled();
    expect(f.transport.releaseRecovery).not.toHaveBeenCalled();
  });

  it.each(['versão mudou', 'execução ativa', 'resposta perdida'])('limpa aquisição ambígua: %s', async message => {
    const f = fixture();
    f.transport.reserveRecovery.mockRejectedValueOnce(new Error(message));
    await expect(f.reader.open()).rejects.toThrow(message);
    expect(f.transport.reserveRecovery).toHaveBeenCalledOnce();
    expect(f.transport.releaseRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '8');
  });

  it('recibo divergente não substitui a identidade usada no cleanup', async () => {
    const f = fixture();
    f.transport.reserveRecovery.mockResolvedValueOnce(new apidto.TimelineRecoveryReservation({
      conversationId: 'other', reference: { ...reference, executionId: 'other' }, contentVersion: '99',
    }));
    await expect(f.reader.open()).rejects.toThrow('Invalid');
    expect(f.transport.releaseRecovery).toHaveBeenCalledExactlyOnceWith('window', reference, '8');
  });

  it('expõe falha de cleanup e conserva dados para liberação explícita', async () => {
    const f = fixture();
    f.transport.reserveRecovery.mockRejectedValueOnce(new Error('aquisição ambígua'));
    f.transport.releaseRecovery.mockRejectedValueOnce(new Error('release falhou'));
    await expect(f.reader.open()).rejects.toMatchObject({ errors: [expect.any(Error), expect.any(Error)] });
    expect(f.released).not.toHaveBeenCalled();
    await f.reader.close();
    expect(f.transport.releaseRecovery).toHaveBeenCalledTimes(2);
    expect(f.released).toHaveBeenCalledOnce();
  });

  it.each(['read', 'reserve'] as const)('Close durante %s não libera antes de Reserve terminar', async phase => {
    const f = fixture();
    const read = deferred<apidto.TimelineRecoveryPage>();
    const reserve = deferred<apidto.TimelineRecoveryReservation>();
    if (phase === 'read') f.transport.readRecovery.mockReset().mockReturnValue(read.promise);
    else f.transport.reserveRecovery.mockReturnValue(reserve.promise);
    const opened = f.reader.open();
    const rejected = expect(opened).rejects.toThrow('closed');
    await vi.waitFor(() => expect(phase === 'read' ? f.transport.readRecovery : f.transport.reserveRecovery).toHaveBeenCalledOnce());
    const closed = f.reader.close();
    expect(f.transport.releaseRecovery).not.toHaveBeenCalled();
    if (phase === 'read') read.resolve(page());
    else reserve.resolve(receipt());
    await Promise.all([rejected, closed]);
    expect(f.transport.releaseRecovery).toHaveBeenCalledTimes(phase === 'read' ? 0 : 1);
  });

  it('capability invalidada cancela pelo Close remoto e rejeita resposta tardia', async () => {
    const f = fixture();
    const reserve = deferred<apidto.TimelineRecoveryReservation>();
    f.transport.reserveRecovery.mockReturnValue(reserve.promise);
    const opened = f.reader.open();
    const rejected = expect(opened).rejects.toThrow('closed');
    await vi.waitFor(() => expect(f.transport.reserveRecovery).toHaveBeenCalledOnce());
    f.deactivate();
    await f.reader.close();
    expect(f.closeWindow).toHaveBeenCalledOnce();
    reserve.resolve(receipt());
    await rejected;
    expect(f.transport.releaseRecovery).not.toHaveBeenCalled();
  });

  it('serializa next sem abortar a leitura que já está em curso', async () => {
    const f = fixture();
    await f.reader.open();
    await f.reader.next();
    const pending = deferred<apidto.TimelineRecoveryPage>();
    f.transport.readRecovery.mockReturnValueOnce(pending.promise);
    const next = f.reader.next();
    await expect(f.reader.next()).rejects.toThrow('busy');
    pending.resolve(page({ chunks: [chunk({ byteOffset: 5 })], nextCursor: '' }));
    expect((await next)?.chunks[0].byteOffset).toBe(5);
    await f.reader.close();
  });

  it('limita percurso de páginas vazias sem repetir RPC indefinidamente', async () => {
    const f = fixture();
    let count = 0;
    f.transport.readRecovery.mockReset().mockImplementation(async () => page({ chunks: [], nextCursor: 'cursor' + (++count) }));
    await f.reader.open();
    for (let i = 0; i < 256; i++) await f.reader.next();
    await expect(f.reader.next()).rejects.toThrow('Invalid');
    expect(f.transport.readRecovery).toHaveBeenCalledTimes(256);
    expect(f.transport.releaseRecovery).toHaveBeenCalledOnce();
  });

  it('limita bytes totais mesmo com páginas individuais válidas', async () => {
    const f = fixture();
    let count = 0;
    f.transport.readRecovery.mockReset().mockImplementation(async () => page({
      chunks: [chunk({ iteration: count++, text: 'x'.repeat(65536) })], nextCursor: 'cursor' + count,
    }));
    await f.reader.open();
    for (let i = 0; i < 128; i++) await f.reader.next();
    await expect(f.reader.next()).rejects.toThrow('Invalid');
    expect(f.transport.releaseRecovery).toHaveBeenCalledOnce();
  });
});
