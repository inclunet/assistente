import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CommandWorkspaceTabTargetFields } from './CommandWorkspaceTabTargetFields';

const { announceMock } = vi.hoisted(() => ({ announceMock: vi.fn() }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: announceMock }) }));

const tabs = [
  { id: 'tab-a', type: 'chat' as const, title: 'Conversa', position: 0 },
  { id: 'tab-b', type: 'editor' as const, title: 'Editor', position: 1 },
];

describe('CommandWorkspaceTabTargetFields', () => {
  beforeEach(() => announceMock.mockClear());

  it('persiste tab_id selecionado pelo nome e preserva referência fechada sem torná-la válida', () => {
    const onChange = vi.fn();
    const onValidityChange = vi.fn();
    const { rerender } = render(<CommandWorkspaceTabTargetFields workspaceID="workspace-a" workspaceName="Trabalho"
      tabs={tabs} value={{}} onChange={onChange} onValidityChange={onValidityChange} />);
    fireEvent.change(screen.getByLabelText('commandSettings.tabTarget.mode'), { target: { value: 'specific' } });
    expect(onChange).toHaveBeenLastCalledWith({ workspace_id: 'workspace-a', target_mode: 'specific' });
    rerender(<CommandWorkspaceTabTargetFields workspaceID="workspace-a" workspaceName="Trabalho"
      tabs={tabs} value={{ workspace_id: 'workspace-a', target_mode: 'specific', tab_id: 'tab-b' }}
      onChange={onChange} onValidityChange={onValidityChange} />);
    expect(screen.getByRole('option', { name: 'Editor' })).toBeInTheDocument();
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    rerender(<CommandWorkspaceTabTargetFields workspaceID="workspace-a" workspaceName="Trabalho"
      tabs={[tabs[0]]} value={{ workspace_id: 'workspace-a', target_mode: 'specific', tab_id: 'tab-b' }}
      onChange={onChange} onValidityChange={onValidityChange} />);
    expect(screen.getByRole('option', { name: 'commandSettings.tabTarget.unavailableTab' })).toBeDisabled();
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(onChange).not.toHaveBeenCalledWith(expect.objectContaining({ tab_id: 'tab-a' }));
  });

  it('aceita posição inteira positiva sem teto e invalida outros workspaces', () => {
    const onChange = vi.fn();
    const onValidityChange = vi.fn();
    const { rerender } = render(<CommandWorkspaceTabTargetFields workspaceID="workspace-a" workspaceName="Trabalho"
      tabs={[]} value={{ workspace_id: 'workspace-a', target_mode: 'position', position: 128 }}
      onChange={onChange} onValidityChange={onValidityChange} />);
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    fireEvent.change(screen.getByLabelText('commandSettings.tabTarget.position'), { target: { value: '300' } });
    expect(onChange).toHaveBeenLastCalledWith({ workspace_id: 'workspace-a', target_mode: 'position', position: 300 });
    rerender(<CommandWorkspaceTabTargetFields workspaceID="workspace-b" workspaceName="Outro"
      tabs={[]} value={{ workspace_id: 'workspace-a', target_mode: 'position', position: 128 }}
      onChange={onChange} onValidityChange={onValidityChange} />);
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(announceMock).toHaveBeenCalledTimes(1);
    expect(announceMock).toHaveBeenCalledWith('commandSettings.tabTarget.otherWorkspace');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
