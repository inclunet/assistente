import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useWorkspaceChatBridge } from './useWorkspaceChatBridge';
const mocks = vi.hoisted(() => ({
  load: vi.fn(), error: vi.fn(),
  workspace: { activeTabId: 'editor', tabs: [{ id: 'editor', type: 'editor', conversationId: 'chat' }, { id: 'other', type: 'terminal', conversationId: '' }] },
}));
vi.mock('../utils/logger', () => ({ logger: { error: mocks.error } }));
vi.mock('../store/chatStore', () => ({ useChatStore: { getState: () => ({ loadConversationSession: mocks.load }) } }));
vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign((selector: (state: unknown) => unknown) => selector({ isInitialized: true, workspace: mocks.workspace }), { getState: () => ({ workspace: mocks.workspace }) }),
  registerTabRenameHandler: () => () => {},
}));
vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({ RenameConversation: vi.fn() }));
vi.mock('../lib/workspaceConversation', () => ({ ensureWorkspaceTabConversationId: vi.fn() }));
beforeEach(() => { vi.clearAllMocks(); mocks.workspace.activeTabId = 'editor'; });
describe('useWorkspaceChatBridge', () => {
  it('trata falha do painel auxiliar e tenta novamente ao reativar', async () => {
    mocks.load.mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined);
    const view = renderHook(() => useWorkspaceChatBridge());
    await waitFor(() => expect(mocks.error).toHaveBeenCalledTimes(1));
    act(() => { mocks.workspace.activeTabId = 'other'; view.rerender(); });
    act(() => { mocks.workspace.activeTabId = 'editor'; view.rerender(); });
    await waitFor(() => expect(mocks.load).toHaveBeenCalledTimes(2));
    expect(mocks.load).toHaveBeenLastCalledWith('chat');
    view.unmount();
  });
});
