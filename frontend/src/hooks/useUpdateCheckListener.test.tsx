/** @vitest-environment jsdom */
import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { UPDATE_CHECK_ERROR_EVENT, useUpdateCheckListener } from './useUpdateCheckListener';

let subscribedEvent = '';
let handler: (() => void) | undefined;
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, callback: () => void) => {
    subscribedEvent = event;
    handler = callback;
    return vi.fn();
  },
}));

const addToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof addToast }) => unknown) =>
    selector({ addToast }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('useUpdateCheckListener', () => {
  beforeEach(() => {
    subscribedEvent = '';
    handler = undefined;
    addToast.mockClear();
  });

  it('mostra feedback acessível sem detalhes internos', () => {
    renderHook(() => useUpdateCheckListener());
    expect(subscribedEvent).toBe(UPDATE_CHECK_ERROR_EVENT);

    act(() => handler?.());

    expect(addToast).toHaveBeenCalledWith('app.updater.checkError', 'warning', 10000);
  });
});
