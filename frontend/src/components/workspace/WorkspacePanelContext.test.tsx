import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { WorkspacePanelProvider, useWorkspacePanel } from './WorkspacePanelContext';

const panelTab = {
  id: 'tab-1',
  type: 'chat' as const,
  title: 'Chat',
  position: 0,
};

describe('WorkspacePanelContext', () => {
  it('retorna a identidade explícita do painel', () => {
    const { result } = renderHook(() => useWorkspacePanel(), {
      wrapper: ({ children }) => (
        <WorkspacePanelProvider value={{ tab: panelTab, isActive: true }}>
          {children}
        </WorkspacePanelProvider>
      ),
    });

    expect(result.current).toEqual({ tab: panelTab, isActive: true });
  });

  it('falha cedo fora do provider para evitar fallback global acidental', () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);

    expect(() => renderHook(() => useWorkspacePanel())).toThrow(
      'useWorkspacePanel must be used within WorkspacePanelProvider',
    );

    consoleErrorSpy.mockRestore();
  });
});
