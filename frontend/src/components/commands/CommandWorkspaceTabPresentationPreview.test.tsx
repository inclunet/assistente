import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { axe } from 'vitest-axe';
import { CommandWorkspaceTabPresentationPreview } from './CommandWorkspaceTabPresentationPreview';
import type { WorkspaceData } from '../../store/workspaceStore';

const workspace = (tabs: WorkspaceData['tabs']): WorkspaceData => ({
  id: 'workspace-a', name: 'Trabalho', tabs, activeTabId: tabs[0]?.id ?? null,
});

describe('CommandWorkspaceTabPresentationPreview', () => {
  it('deriva título e tipo pelo ID, acompanhando renomeação, reordenação e fechamento', () => {
    const args = { workspace_id: 'workspace-a', target_mode: 'specific', tab_id: 'tab-b' };
    const tabs = [
      { id: 'tab-a', type: 'chat' as const, title: 'Chat', position: 0 },
      { id: 'tab-b', type: 'editor' as const, title: 'Notas', position: 1 },
    ];
    const { container, rerender } = render(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace(tabs)} />);
    expect(screen.getByText('workspace.tabType.editor: Notas')).toBeInTheDocument();
    expect(screen.getByText('commandSettings.automaticPresentation.help')).toBeInTheDocument();

    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace([
      { ...tabs[1], title: 'Rascunhos', position: 0 }, tabs[0],
    ])} />);
    expect(screen.getByText('workspace.tabType.editor: Rascunhos')).toBeInTheDocument();

    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace([tabs[0]])} />);
    expect(screen.getByText('commandSettings.automaticPresentation.unavailable')).toBeInTheDocument();
    expect(container.querySelector('svg')).toBeNull();
  });

  it('ordena a posição como o backend e recusa posições duplicadas e tipos desconhecidos', () => {
    const args = { workspace_id: 'workspace-a', target_mode: 'position', position: 1 };
    const tabs = [
      { id: 'tab-b', type: 'editor' as const, title: 'Segundo', position: 2 },
      { id: 'tab-a', type: 'chat' as const, title: 'Primeiro', position: 1 },
    ];
    const { rerender } = render(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace(tabs)} />);
    expect(screen.getByText('workspace.tabType.chat: Primeiro')).toBeInTheDocument();

    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace([
      tabs[0], { ...tabs[1], position: 2 },
    ])} />);
    expect(screen.getByText('commandSettings.automaticPresentation.unavailable')).toBeInTheDocument();

    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to" argumentsValue={args} workspace={workspace([
      { id: 'unknown', type: 'future' as unknown as 'chat', title: 'Futuro', position: 1 },
    ])} />);
    expect(screen.getByText('commandSettings.automaticPresentation.unavailable')).toBeInTheDocument();
  });

  it('usa a apresentação automática para posições fixas mesmo quando o destino é a aba ativa', () => {
    const tabs = [{ id: 'tab-a', type: 'tasklist' as const, title: 'Hoje', position: 0 }];
    render(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.first" workspace={workspace(tabs)} />);
    expect(screen.getByText('workspace.tabType.tasklist: Hoje')).toBeInTheDocument();
  });

  it('espelha a recusa para workspace divergente e ID específico ambíguo', () => {
    const tabs = [
      { id: 'tab-a', type: 'chat' as const, title: 'A', position: 0 },
      { id: 'tab-a', type: 'editor' as const, title: 'B', position: 1 },
    ];
    const { rerender } = render(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to"
      argumentsValue={{ workspace_id: 'workspace-other', target_mode: 'specific', tab_id: 'tab-a' }} workspace={workspace(tabs)} />);
    expect(screen.getByText('commandSettings.automaticPresentation.unavailable')).toBeInTheDocument();
    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to"
      argumentsValue={{ workspace_id: 'workspace-a', target_mode: 'specific', tab_id: 'tab-a' }} workspace={workspace(tabs)} />);
    expect(screen.getByText('commandSettings.automaticPresentation.unavailable')).toBeInTheDocument();
  });

  it('suporta posição acima de nove e não renderiza para comandos sem apresentação automática', async () => {
    const tabs = Array.from({ length: 32 }, (_, index) => ({
      id: `tab-${index + 1}`, type: 'terminal' as const, title: `Terminal ${index + 1}`, position: index,
    }));
    const { container, rerender } = render(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.go_to"
      argumentsValue={{ workspace_id: 'workspace-a', target_mode: 'position', position: 32 }} workspace={workspace(tabs)} />);
    expect(screen.getByText('workspace.tabType.terminal: Terminal 32')).toBeInTheDocument();
    expect(await axe(container, { rules: { 'color-contrast': { enabled: false } } })).toHaveNoViolations();

    rerender(<CommandWorkspaceTabPresentationPreview commandID="workspace.tab.next" workspace={workspace(tabs)} />);
    expect(container.querySelector('section')).toBeNull();
  });
});
