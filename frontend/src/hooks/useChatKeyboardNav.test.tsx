/** @vitest-environment jsdom */
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useChatKeyboardNav } from './useChatKeyboardNav';

vi.mock('./useAnnouncer', () => ({
  announce: vi.fn(),
}));

describe('useChatKeyboardNav', () => {
  const input = document.createElement('textarea');
  const container = document.createElement('div');
  const messages: HTMLDivElement[] = [];

  beforeEach(() => {
    document.body.innerHTML = '';
    container.className = 'messages';

    for (let i = 0; i < 3; i += 1) {
      const node = document.createElement('div');
      node.className = 'message-node';
      node.tabIndex = 0;
      node.textContent = `msg-${i}`;
      node.scrollIntoView = () => {};
      container.appendChild(node);
      messages.push(node);
    }

    document.body.appendChild(container);
    document.body.appendChild(input);
  });

  afterEach(() => {
    messages.splice(0, messages.length);
    document.body.innerHTML = '';
    vi.clearAllMocks();
  });

  it('move foco do input para a ultima mensagem com ArrowUp', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    const { result } = renderHook(() =>
      useChatKeyboardNav({ inputRef, messagesContainerRef })
    );

    input.focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }));
    });

    expect(document.activeElement).toBe(messages[messages.length - 1]);
    expect(result.current.focusedMessageIndex.current).toBe(messages.length - 1);
  });

  it('Escape NÃO é tratado pelo hook (delegado ao sistema de landmarks)', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[1].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    });

    // Escape não move o foco — isso é responsabilidade do useLandmarkNavigation
    expect(document.activeElement).toBe(messages[1]);
  });

  it('type-ahead: move foco para o input ao digitar letra na mensagem', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[1].focus();
    expect(document.activeElement).toBe(messages[1]);

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }));
    });

    expect(document.activeElement).toBe(input);
  });

  it('type-ahead: move foco para o input ao digitar número', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '5' }));
    });

    expect(document.activeElement).toBe(input);
  });

  it('type-ahead: move foco com letra maiúscula (Shift+letra)', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'H', shiftKey: true }));
    });

    expect(document.activeElement).toBe(input);
  });

  it('type-ahead: NÃO ativa com Ctrl+letra', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true }));
    });

    expect(document.activeElement).toBe(messages[0]);
  });

  it('type-ahead: NÃO ativa com Alt+letra', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'a', altKey: true }));
    });

    expect(document.activeElement).toBe(messages[0]);
  });

  it('type-ahead: NÃO ativa quando foco já está no input', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    input.focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }));
    });

    expect(document.activeElement).toBe(input);
  });

  it('type-ahead: insere o caractere digitado no textarea', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    input.value = '';
    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'O' }));
    });

    expect(document.activeElement).toBe(input);
    expect(input.value).toBe('O');

    // Simula segunda tecla (foco já no input, não deve acionar type-ahead de novo)
    // Mas como foco já está no input e activeElement !== messageNode, não entra no type-ahead
  });

  it('type-ahead: insere caractere no meio do texto existente', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    input.value = 'abcd';
    input.selectionStart = 2;
    input.selectionEnd = 2;

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'X' }));
    });

    expect(document.activeElement).toBe(input);
    expect(input.value).toBe('abXcd');
    expect(input.selectionStart).toBe(3);
  });

  it('type-ahead: NÃO ativa para teclas não imprimíveis', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[0].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }));
    });
    expect(document.activeElement).toBe(messages[0]);

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Shift' }));
    });
    expect(document.activeElement).toBe(messages[0]);

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1' }));
    });
    expect(document.activeElement).toBe(messages[0]);
  });
});
