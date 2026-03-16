/** @vitest-environment jsdom */
import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useChatKeyboardNav } from './useChatKeyboardNav';

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

  it('restaura foco no input ao pressionar Escape', () => {
    const inputRef = { current: input } as React.RefObject<HTMLTextAreaElement>;
    const messagesContainerRef = { current: container } as React.RefObject<HTMLDivElement>;

    renderHook(() => useChatKeyboardNav({ inputRef, messagesContainerRef }));

    messages[1].focus();

    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    });

    expect(document.activeElement).toBe(input);
  });
});
