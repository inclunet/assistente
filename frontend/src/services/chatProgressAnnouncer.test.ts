import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CHAT_PROGRESS_BURST_WINDOW_MS,
  createChatProgressAnnouncer,
  type ChatProgressTool,
} from './chatProgressAnnouncer';

const tool = (callId: string, name = callId): ChatProgressTool => ({
  callId,
  name,
  origin: 'builtin',
  surfaceOrigin: {
    conversationId: 'conversation-1',
    surfaceId: 'tab-1',
    sessionKey: 'tab-1:conversation-1',
    surfaceType: 'page',
  },
});

describe('chatProgressAnnouncer', () => {
  beforeEach(() => vi.useFakeTimers());

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  it('anuncia uma ferramenta pendente depois da janela de rajada', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1', 'buscar'));
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS - 1);
    expect(announce).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith([{ state: 'running', tools: [tool('call-1', 'buscar')] }]);
  });

  it('agrupa uma rajada rápida e elimina início repetido', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1', 'buscar'));
    announcer.toolStarted(tool('call-2', 'ler arquivo'));
    announcer.toolStarted(tool('call-1', 'buscar'));
    announcer.toolEnded('call-1', 'ok');
    announcer.toolEnded('call-2', 'ok');

    expect(announce).not.toHaveBeenCalled();
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce.mock.calls[0]).toEqual([[{
      state: 'done',
      tools: [tool('call-1', 'buscar'), tool('call-2', 'ler arquivo')],
    }]]);
  });

  it('garante que a ferramenta longa seja anunciada antes de tool_end', () => {
    const order: string[] = [];
    const announcer = createChatProgressAnnouncer({
      announce: (groups) => order.push(groups.map((group) => group.state).join('+')),
    });

    announcer.toolStarted(tool('call-1', 'buscar'));
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    expect(order).toEqual(['running']);
    announcer.toolEnded('call-1', 'ok');
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);

    expect(order).toEqual(['running', 'done']);
  });

  it('agrupa conclusões de muitas ferramentas longas em um único done', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });
    const tools = Array.from({ length: 20 }, (_, index) => tool(`call-${index}`, `tool-${index}`));

    tools.forEach((item) => announcer.toolStarted(item));
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    tools.forEach((item) => announcer.toolEnded(item.callId, 'ok'));

    expect(announce).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);

    expect(announce).toHaveBeenCalledTimes(2);
    expect(announce.mock.calls[1][0][0].state).toBe('done');
    expect(announce.mock.calls[1][0][0].tools).toHaveLength(20);
  });

  it('emite um resumo único quando done e running coexistem no flush', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1', 'concluída'));
    announcer.toolStarted(tool('call-2', 'longa'));
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    announcer.toolStarted(tool('call-3', 'nova'));
    announcer.toolEnded('call-1', 'ok');
    announcer.finishSegment();

    expect(announce).toHaveBeenCalledTimes(2);
    expect(announce.mock.calls[1][0]).toEqual([
      { state: 'done', tools: [tool('call-1', 'concluída')] },
      { state: 'running', tools: [tool('call-3', 'nova')] },
    ]);
  });

  it('não transforma uma ferramenta já concluída em running ao fechar o segmento', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1', 'rápida'));
    announcer.toolEnded('call-1', 'ok');
    announcer.finishSegment();

    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce).toHaveBeenCalledWith([{ state: 'done', tools: [tool('call-1', 'rápida')] }]);
  });

  it('limpa o timer ao terminar e ao cancelar o controller', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1'));
    announcer.toolEnded('call-1', 'error');
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    expect(announce).not.toHaveBeenCalled();

    announcer.toolStarted(tool('call-2'));
    announcer.dispose();
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);
    expect(announce).not.toHaveBeenCalled();
  });

  it('permite anunciar a nova tentativa sem repetir a tentativa anterior', () => {
    const announce = vi.fn();
    const announcer = createChatProgressAnnouncer({ announce });

    announcer.toolStarted(tool('call-1', 'buscar'));
    announcer.toolFailed('call-1', true);
    announcer.toolStarted(tool('call-1', 'buscar'));
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);

    expect(announce).toHaveBeenCalledTimes(1);
    expect(announce.mock.calls[0][0][0].state).toBe('running');
  });

  it('isola o estado de ferramentas entre instâncias de conversa', () => {
    const first = vi.fn();
    const second = vi.fn();
    const firstAnnouncer = createChatProgressAnnouncer({ announce: first });
    const secondAnnouncer = createChatProgressAnnouncer({ announce: second });

    firstAnnouncer.toolStarted(tool('call-1', 'conversa 1'));
    secondAnnouncer.toolStarted({
      ...tool('call-1', 'conversa 2'),
      surfaceOrigin: {
        conversationId: 'conversation-2',
        surfaceId: 'tab-2',
        sessionKey: 'tab-2:conversation-2',
        surfaceType: 'page',
      },
    });
    firstAnnouncer.dispose();
    vi.advanceTimersByTime(CHAT_PROGRESS_BURST_WINDOW_MS);

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    expect(second.mock.calls[0][0][0].tools[0]).toMatchObject({ name: 'conversa 2' });
  });
});
