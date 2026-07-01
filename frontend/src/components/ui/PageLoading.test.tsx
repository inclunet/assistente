import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PageLoading } from './PageLoading';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import type { WorkspaceTab } from '../../store/workspaceStore';

const announceRequestMock = vi.hoisted(() => vi.fn(() => true));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announceRequest: announceRequestMock,
  }),
}));

describe('PageLoading', () => {
  beforeEach(() => {
    announceRequestMock.mockClear();
    announceRequestMock.mockReturnValue(true);
  });

  it('anuncia carregamento como progresso pelo broker global sem live region local', () => {
    render(<PageLoading message="Carregando dados..." />);

    expect(screen.getByText('Carregando dados...')).toBeInTheDocument();
    expect(announceRequestMock).toHaveBeenCalledWith({
      message: 'Carregando dados...',
      origin: undefined,
      eventType: 'progress',
    });
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('inclui origem do painel quando renderizado dentro de workspace', () => {
    const tab: WorkspaceTab = {
      id: 'tab-mcp',
      type: 'editor',
      title: 'Editor',
      position: 0,
    };

    render(
      <WorkspacePanelProvider value={{ tab, isActive: false }}>
        <PageLoading message="Carregando painel..." />
      </WorkspacePanelProvider>
    );

    expect(announceRequestMock).toHaveBeenCalledWith({
      message: 'Carregando painel...',
      origin: {
        tabId: 'tab-mcp',
        surfaceId: 'tab-mcp',
        conversationId: undefined,
        surfaceType: 'editor',
        profileSlug: null,
        title: 'Editor',
      },
      eventType: 'progress',
    });
  });
});
