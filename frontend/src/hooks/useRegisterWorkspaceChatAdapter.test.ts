import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import {
  useRegisterWorkspaceChatAdapter,
} from './useRegisterWorkspaceChatAdapter';
import type { WorkspaceChatModalAdapter } from '../store/workspaceChatModalStore';

const registerAdapter = vi.fn();

vi.mock('../store/workspaceChatModalStore', () => ({
  registerWorkspaceChatModalAdapter: (...args: unknown[]) => registerAdapter(...args),
}));

function adapter(prepare: WorkspaceChatModalAdapter['prepare']): WorkspaceChatModalAdapter {
  return {
    prepare,
    send: vi.fn(),
  };
}

describe('useRegisterWorkspaceChatAdapter', () => {
  beforeEach(() => {
    registerAdapter.mockReset();
  });

  it('recusa o resultado quando o adapter real muda durante prepare', async () => {
    let resolveFirst!: (result: Awaited<ReturnType<WorkspaceChatModalAdapter['prepare']>>) => void;
    const first = adapter(() => new Promise((resolve) => { resolveFirst = resolve; }));
    const second = adapter(vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'new', meta: null }));

    const { rerender } = renderHook(
      ({ current }) => useRegisterWorkspaceChatAdapter('tab-editor', current),
      { initialProps: { current: first } },
    );
    await waitFor(() => expect(registerAdapter).toHaveBeenCalledTimes(1));
    const wrapper = registerAdapter.mock.calls[0][1] as WorkspaceChatModalAdapter;
    const pending = wrapper.prepare();

    rerender({ current: second });
    resolveFirst({ ok: true, contextDisplay: 'old', meta: null });

    await expect(pending).resolves.toEqual({
      ok: false,
      message: 'workspace.chatModal.panelLoading',
    });
    expect(second.prepare).not.toHaveBeenCalled();
  });

  it('mantém send dinâmico após a preparação concluída', async () => {
    const firstSend = vi.fn().mockResolvedValue(null);
    const secondSend = vi.fn().mockResolvedValue(null);
    const first = { ...adapter(vi.fn().mockResolvedValue({ ok: true, contextDisplay: 'ctx', meta: null })), send: firstSend };
    const second = { ...adapter(vi.fn()), send: secondSend };

    const { rerender } = renderHook(
      ({ current }) => useRegisterWorkspaceChatAdapter('tab-editor', current),
      { initialProps: { current: first } },
    );
    await waitFor(() => expect(registerAdapter).toHaveBeenCalledTimes(1));
    const wrapper = registerAdapter.mock.calls[registerAdapter.mock.calls.length - 1]?.[1] as WorkspaceChatModalAdapter;
    await wrapper.prepare();

    rerender({ current: second });
    await wrapper.send('instruction', undefined, null, { tabId: 'tab-editor', conversationId: '01900000-0000-7000-8000-000000000001' });

    expect(firstSend).not.toHaveBeenCalled();
    expect(secondSend).toHaveBeenCalledTimes(1);
  });
});
