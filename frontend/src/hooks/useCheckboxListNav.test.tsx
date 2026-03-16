/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useCheckboxListNav } from './useCheckboxListNav';

const playBumpSoundMock = vi.fn();

vi.mock('../services/audioFeedback', () => ({
  playBumpSound: () => playBumpSoundMock(),
}));

describe('useCheckboxListNav', () => {
  let container: HTMLDivElement;
  let checkboxes: HTMLInputElement[];

  beforeEach(() => {
    container = document.createElement('div');
    checkboxes = [];

    for (let i = 0; i < 3; i += 1) {
      const input = document.createElement('input');
      input.type = 'checkbox';
      container.appendChild(input);
      checkboxes.push(input);
    }

    document.body.appendChild(container);
  });

  afterEach(() => {
    playBumpSoundMock.mockClear();
    document.body.innerHTML = '';
  });

  it('define tabindex e navega com setas', () => {
    const { result } = renderHook(() => useCheckboxListNav());

    act(() => {
      result.current(container);
    });

    expect(checkboxes[0].getAttribute('tabindex')).toBe('0');
    expect(checkboxes[1].getAttribute('tabindex')).toBe('-1');

    checkboxes[0].focus();
    act(() => {
      checkboxes[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    });

    expect(document.activeElement).toBe(checkboxes[1]);
    expect(checkboxes[1].getAttribute('tabindex')).toBe('0');
  });

  it('toca bump ao tentar navegar alem do limite', () => {
    const { result } = renderHook(() => useCheckboxListNav());

    act(() => {
      result.current(container);
    });

    checkboxes[0].focus();
    act(() => {
      checkboxes[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true }));
    });

    expect(playBumpSoundMock).toHaveBeenCalled();
  });
});
