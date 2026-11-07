import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { canonicalTestFixture, canonicalTestItem } from './chatTimelineTestFixture';
import { readChatCompletion, type ChatCompletion } from './chatCompletion';

let fixture: ReturnType<typeof canonicalTestFixture>;
vi.mock('./chatTimelineWails', () => ({ getChatTimelineClient: () => fixture.client }));
const done: ChatCompletion = { conversationId: 'conversation', itemId: 'response', turnId: 'turn',
  assistantMessageId: 'assistant', reason: 'completed' };
beforeEach(() => { fixture = canonicalTestFixture([canonicalTestItem('response', 'corpo')]); });
afterEach(async () => { await fixture.client.dispose(); });

describe('leitura exata da conclusão canônica', () => {
  it.each([false, true])('lê depois do terminal mesmo com evento visual já entregue=%s', async visualDelivered => {
    const visual = fixture.presentation('visual');
    if (visualDelivered) await visual.open();
    const previous = visual.window;
    expect(await readChatCompletion(done)).toBe('corpo');
    expect(visual.window).toBe(previous);
    expect(fixture.port.open).toHaveBeenLastCalledWith('conversation', expect.stringMatching(/^completion:/), null,
      { cursor: '', anchor: '', anchorItemId: 'response', direction: 'around', limit: 1 });
    expect(fixture.port.close).toHaveBeenCalledTimes(1);
    if (visualDelivered) await visual.close();
  });
  it('compõe só as referências da conclusão, sem intermediário nem diagnóstico', async () => {
    const item = canonicalTestItem('response');
    item.segments = [{ segmentId: 'intermediate', kind: 'text', text: 'não aplicar' },
      { segmentId: 'a', kind: 'text', text: 'parte ' }, { segmentId: 'b', kind: 'text', text: 'final' }];
    item.segments[1].conclusion = item.segments[2].conclusion = true;
    fixture.setItems([item]);
    expect(await readChatCompletion(done)).toBe('parte final');
  });
  it.each(['cancelled', 'error', 'limit_reached', 'output_limit'])('não aplica terminal %s', async reason => {
    await expect(readChatCompletion({ ...done, reason })).rejects.toThrow();
    expect(fixture.port.open).not.toHaveBeenCalled();
  });
  it.each([{ state: 'streaming' }, { representativeMessageId: 'retry-response' }, { turnId: 'other-turn' }])
  ('recusa substituição após o terminal: %j', async change => {
    fixture.setItems([canonicalTestItem('response', 'conteúdo de outra tentativa', change)]);
    await expect(readChatCompletion(done)).rejects.toThrow('replaced or unavailable');
    expect(fixture.port.close).toHaveBeenCalledTimes(1);
  });
  it('não usa outra resposta quando o alvo está ausente', async () => {
    fixture.setItems([canonicalTestItem('other', 'não aplicar')]);
    await expect(readChatCompletion(done)).rejects.toThrow();
    expect(fixture.port.close).toHaveBeenCalledTimes(1);
  });
  it('recusa leitura cancelada sem abrir capability', async () => {
    const controller = new AbortController(); controller.abort();
    await expect(readChatCompletion(done, controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(fixture.port.open).not.toHaveBeenCalled();
  });
  it('cancela Open pendente e descarta o snapshot tardio sem entregar texto', async () => {
    const originalOpen = fixture.port.open;
    let release!: () => void;
    const blocked = new Promise<void>(resolve => { release = resolve; });
    fixture.port.open = vi.fn(async (...args: Parameters<typeof originalOpen>) => {
      const snapshot = await originalOpen(...args);
      await blocked;
      return snapshot;
    });
    const controller = new AbortController();
    const close = vi.spyOn(fixture.client, 'close');
    const result = readChatCompletion(done, controller.signal);
    const rejected = expect(result).rejects.toMatchObject({ name: 'AbortError' });
    await vi.waitFor(() => expect(fixture.port.open).toHaveBeenCalledTimes(1));
    controller.abort();
    await rejected;
    try {
      expect(close).toHaveBeenCalledTimes(1);
      expect(fixture.client.getWindow('conversation', vi.mocked(fixture.port.open).mock.calls[0][1])).toBeUndefined();
    } finally { release(); }
    await vi.waitFor(() => expect(fixture.client.hasSurface('conversation',
      vi.mocked(fixture.port.open).mock.calls[0][1])).toBe(false));
    expect(fixture.port.close).toHaveBeenCalledTimes(1);
  });
});
