/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import type { MouseEvent as ReactMouseEvent } from 'react';

import { useContextMenu, useMessageActions } from './useContextMenu';
import type { MenuItem } from '../components/menu';

const getMessageMenuItemsMock = vi.hoisted(() => vi.fn());
const consumeSkipFocusRestoreMock = vi.hoisted(() => vi.fn());

const ttsServiceMock = vi.hoisted(() => ({
  getVolume: vi.fn(() => 0.75),
  hasVoiceConfig: vi.fn(() => true),
  speakAsRole: vi.fn(async () => {}),
  stop: vi.fn(),
}));

const messageAudioServiceMock = vi.hoisted(() => ({
  getAudioFromDB: vi.fn(),
  generateAndSaveAudio: vi.fn(),
  stopCurrentAudio: vi.fn(),
  playAudioBase64: vi.fn(),
  playAudioBlob: vi.fn(),
  saveAudioToDB: vi.fn(),
}));

vi.mock('../lib/messageMenuItems', () => ({
  getMessageMenuItems: (...args: unknown[]) => getMessageMenuItemsMock(...args),
}));

vi.mock('../store/chatStore', () => ({
  useChatStore: {
    getState: () => ({ consumeSkipFocusRestore: consumeSkipFocusRestoreMock }),
  },
}));

vi.mock('../services/tts', () => ({
  ttsService: ttsServiceMock,
}));

vi.mock('../services/messageAudio', () => ({
  messageAudioService: messageAudioServiceMock,
}));

describe('useContextMenu', () => {
  beforeEach(() => {
    getMessageMenuItemsMock.mockReturnValue([{ id: 'copy' } as MenuItem]);
    consumeSkipFocusRestoreMock.mockReturnValue(false);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('exibe menu com posicao e itens', () => {
    const { result } = renderHook(() => useContextMenu({}));

    const target = document.createElement('button');
    const event = {
      preventDefault: vi.fn(),
      currentTarget: target,
      target,
      clientX: 12,
      clientY: 24,
    } as unknown as ReactMouseEvent;

    act(() => {
      result.current.showMenu(event, { id: 1, content: 'Oi' } as never, true);
    });

    expect(result.current.menuVisible).toBe(true);
    expect(result.current.menuPosition).toEqual({ x: 12, y: 24 });
    expect(result.current.menuItems).toHaveLength(1);
  });

  it('restaura foco ao esconder menu', async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useContextMenu({}));

    const target = document.createElement('button');
    target.tabIndex = 0;
    document.body.appendChild(target);
    target.focus();

    const event = {
      preventDefault: vi.fn(),
      currentTarget: target,
      target,
      clientX: 0,
      clientY: 0,
    } as unknown as ReactMouseEvent;

    act(() => {
      result.current.showMenu(event, { id: 1, content: 'Oi' } as never, true);
      result.current.hideMenu();
    });

    act(() => {
      vi.runAllTimers();
    });

    expect(document.activeElement).toBe(target);

    vi.useRealTimers();
  });

  it('nao restaura foco quando store sinaliza skip', () => {
    consumeSkipFocusRestoreMock.mockReturnValue(true);
    vi.useFakeTimers();
    const { result } = renderHook(() => useContextMenu({}));

    const target = document.createElement('button');
    target.tabIndex = 0;

    const event = {
      preventDefault: vi.fn(),
      currentTarget: target,
      target,
      clientX: 0,
      clientY: 0,
    } as unknown as ReactMouseEvent;

    act(() => {
      result.current.showMenu(event, { id: 1, content: 'Oi' } as never, true);
      result.current.hideMenu();
    });

    act(() => {
      vi.runAllTimers();
    });

    expect(document.activeElement).not.toBe(target);
    vi.useRealTimers();
  });
});

describe('useMessageActions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: {
        writeText: vi.fn().mockResolvedValue(undefined),
      },
      configurable: true,
    });
  });

  it('copia mensagem e anuncia sucesso', async () => {
    const announce = vi.fn();
    const { result } = renderHook(() => useMessageActions({ onAnnounce: announce }));

    await act(async () => {
      await result.current.copyMessage({ content: 'Teste' } as never, false);
    });

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('Teste');
    expect(announce).toHaveBeenCalledWith('Mensagem copiada.');
  });

  it('reproduz audio do banco quando disponivel', async () => {
    messageAudioServiceMock.getAudioFromDB.mockResolvedValue({
      audio: 'base64',
      mimeType: 'audio/mpeg',
    });
    ttsServiceMock.hasVoiceConfig.mockReturnValue(true);

    const { result } = renderHook(() => useMessageActions());

    await act(async () => {
      await result.current.speakMessage({ id: 10, content: 'Ola', role: 'assistant' } as never);
    });

    expect(messageAudioServiceMock.playAudioBase64).toHaveBeenCalledWith('base64', 'audio/mpeg', 0.75);
  });

  it('usa speakAsRole quando DB e backend falham', async () => {
    messageAudioServiceMock.getAudioFromDB.mockResolvedValue(null);
    messageAudioServiceMock.generateAndSaveAudio.mockResolvedValue(null);
    ttsServiceMock.hasVoiceConfig.mockReturnValue(true);

    const { result } = renderHook(() => useMessageActions());

    await act(async () => {
      await result.current.speakMessage({ id: 11, content: 'Teste', role: 'assistant' } as never);
    });

    expect(ttsServiceMock.speakAsRole).toHaveBeenCalledWith('Teste', 'assistant');
  });

  it('não reproduz quando sem config de voz', async () => {
    ttsServiceMock.hasVoiceConfig.mockReturnValue(false);

    const { result } = renderHook(() => useMessageActions());

    await act(async () => {
      await result.current.speakMessage({ id: 12, content: 'Teste', role: 'assistant' } as never);
    });

    expect(messageAudioServiceMock.getAudioFromDB).not.toHaveBeenCalled();
    expect(ttsServiceMock.speakAsRole).not.toHaveBeenCalled();
  });
});
