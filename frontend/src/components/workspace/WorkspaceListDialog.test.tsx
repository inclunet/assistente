import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import i18next from 'i18next';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { WorkspaceListDialog } from './WorkspaceListDialog';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { captureCommandSettingsKeyboardContext, capturePagePresentationTarget } from '../../lib/commandPagePresentation';
import { createLocalCommandKeyboard, type LocalCommandKeyboardController } from '../../lib/commandLocalKeyboard';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { workspace } from '../../../wailsjs/go/models';

const api = vi.hoisted(() => ({ rename: vi.fn(), remove: vi.fn(), list: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Workspace', async importOriginal => ({
  ...await importOriginal<typeof import('@wailsjs/go/wailsapi/Workspace')>(),
  RenameWorkspace: api.rename, DeleteWorkspace: api.remove, ListWorkspaces: api.list,
}));
vi.mock('../../hooks/useAnnouncer', async importOriginal => {
  const actual = await importOriginal<typeof import('../../hooks/useAnnouncer')>();
  return { ...actual, announce: api.announce, useAnnouncer: () => ({ ...actual.useAnnouncer(), announce: api.announce }) };
});
let keyboard: LocalCommandKeyboardController | undefined;
const rows = [
  new workspace.WorkspaceInfo({ id: 'active', name: 'Atual', path: 'C:/active', tab_count: 1, is_active: true }),
  new workspace.WorkspaceInfo({ id: 'other', name: 'Projeto', path: 'C:/project', tab_count: 1, is_active: false }),
];
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  api.rename.mockResolvedValue(undefined);
  api.remove.mockResolvedValue(undefined);
  api.list.mockResolvedValue(rows);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session', role: 'user' } });
  useWorkspaceStore.setState({ workspaces: rows, workspace: {
    id: 'active', name: 'Atual', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'chat', title: 'Chat', position: 0 }],
  } });
});
afterEach(() => {
  keyboard?.dispose(); keyboard = undefined;
  useQuestionnaireUIStore.getState().cancel();
  unregisterOpenModal('foreign-modal');
  cleanup(); vi.restoreAllMocks(); vi.clearAllMocks();
});
async function openList(suppressed = false) {
  const activate = vi.fn();
  const onClose = vi.fn();
  render(<MemoryRouter><WorkspaceListDialog onActivate={id => { activate(id); onClose(); }}
    onClose={onClose} /></MemoryRouter>);
  const cell = await screen.findByRole('gridcell', { name: 'Projeto' });
  const first = screen.getByRole('gridcell', { name: 'Atual' });
  fireEvent.click(first);
  fireEvent.keyDown(first, { key: 'ArrowDown', code: 'ArrowDown' });
  await waitFor(() => expect(cell).toHaveAttribute('tabindex', '0'));
  act(() => cell.focus());
  keyboard = createLocalCommandKeyboard({
    target: window, reset: async () => {}, onUp: async () => {},
    blocked: id => !captureCommandSettingsKeyboardContext(() => '/')?.allowedCommandIds?.includes(id ?? ''),
    readContext: () => captureCommandSettingsKeyboardContext(() => '/'),
    loadMap: async () => ({ generation: 'test', bindings: [], contextualBindings: suppressed ? [] :
      [['F2', 'workspaces.rename.open'], ['Delete', 'workspaces.delete.open']].map(([code, commandId]) => ({
        shortcut: { version: 1 as const, code, modifiers: [] }, fallback: null,
        bySurface: { workspaces: { shortcut: { version: 1 as const, code, modifiers: [] }, commandId, handler: 'local_ui' as const } },
      })),
    }),
    onDown: async request => {
      const target = capturePagePresentationTarget(() => '/', request.commandId);
      try { target?.open(request.commandId); } finally { target?.dispose(); }
    },
  });
  await keyboard.refresh();
  return { cell, activate, onClose };
}
function press(cell: HTMLElement, code: string) {
  fireEvent.keyDown(cell, { code, key: code });
  fireEvent.keyUp(cell, { code, key: code });
}
it('F2 passa pelo mapa e renomeia o item selecionado sem mudar o ativo', async () => {
  const { cell } = await openList();
  press(cell, 'F2');
  await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
  await act(async () => { useQuestionnaireUIStore.getState().submit({ name: 'Renomeado' }); });
  expect(api.rename).toHaveBeenCalledWith('other', 'Renomeado');
  expect(useWorkspaceStore.getState().workspace?.name).toBe('Atual');
});
it('Delete pede confirmação e só então remove o workspace selecionado', async () => {
  const { cell } = await openList();
  press(cell, 'Delete');
  const dialog = await screen.findByRole('alertdialog');
  expect(dialog).toBeTruthy();
  expect(api.remove).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: /confirmar|confirm/i }));
  await waitFor(() => expect(api.remove).toHaveBeenCalledWith('other'));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
  expect(api.announce).toHaveBeenCalledExactlyOnceWith(i18next.t('workspace.announce.workspaceRemoved'));
});
it('não executa F2 ou Delete quando o binding foi suprimido', async () => {
  const { cell } = await openList(true);
  press(cell, 'F2'); press(cell, 'Delete');
  expect(useQuestionnaireUIStore.getState().active).toBeNull();
  expect(screen.queryByRole('alertdialog')).toBeNull();
});
it('recusa ações atrás de outro modal e exclusão do workspace ativo', async () => {
  const { cell } = await openList();
  registerOpenModal('foreign-modal');
  press(cell, 'F2'); press(cell, 'Delete');
  expect(useQuestionnaireUIStore.getState().active).toBeNull();
  unregisterOpenModal('foreign-modal');
  const active = screen.getByRole('gridcell', { name: 'Atual' });
  fireEvent.click(active); press(active, 'Delete');
  expect(screen.queryByRole('alertdialog')).toBeNull();
  expect(api.remove).not.toHaveBeenCalled();
});
it('Enter ativa a seleção e fecha a lista', async () => {
  const { cell, activate, onClose } = await openList();
  press(cell, 'Enter');
  expect(activate).toHaveBeenCalledOnce();
  expect(onClose).toHaveBeenCalledOnce();
});

it('excluir o último resultado filtrado devolve o foco para a busca', async () => {
  const { cell } = await openList();
  const search = screen.getByRole('textbox');
  fireEvent.change(search, { target: { value: 'Projeto' } });
  act(() => cell.focus());
  api.list.mockResolvedValue([rows[0]]);
  press(cell, 'Delete');
  await screen.findByRole('alertdialog');
  fireEvent.click(screen.getByRole('button', { name: /confirmar|confirm/i }));
  await waitFor(() => expect(search).toHaveFocus());
  expect(api.remove).toHaveBeenCalledWith('other');
});

it('falha ao renomear usa o anunciador global e preserva a lista', async () => {
 const { cell } = await openList();
 api.rename.mockRejectedValueOnce(new Error('disk error'));
 press(cell, 'F2');
 await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
 await act(async () => { useQuestionnaireUIStore.getState().submit({ name: 'Renomeado' }); });
 await waitFor(() => expect(api.announce).toHaveBeenCalledOnce());
 expect(screen.queryByRole('alert')).toBeNull();
 expect(screen.getByRole('gridcell', { name: 'Projeto' })).toBeInTheDocument();
});

it('restauração após renomear não rouba foco de outro modal', async () => {
 const { cell } = await openList();
 press(cell, 'F2');
 await waitFor(() => expect(useQuestionnaireUIStore.getState().active).not.toBeNull());
 registerOpenModal('foreign-modal');
 const nextForm = document.createElement('input');
 document.body.append(nextForm);
 nextForm.focus();
 try {
  await act(async () => {
   useQuestionnaireUIStore.getState().submit({ name: 'Renomeado' });
   await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
  expect(nextForm).toHaveFocus();
 } finally { nextForm.remove(); }
});
it('mostra o workspace aberto separado da seleção do grid', async () => {
 await openList();
 const active = screen.getByRole('gridcell', { name: 'Atual' }).closest('[role="row"]')!;
 const selected = screen.getByRole('gridcell', { name: 'Projeto' }).closest('[role="row"]')!;
 expect(within(active as HTMLElement).getByText('workspace.activeStatus')).toBeInTheDocument();
 expect(within(selected as HTMLElement).getByText('workspace.inactiveStatus')).toBeInTheDocument();
});

it('atualiza a lista ao abrir e ativa resultados novos pela mesma coleção', async () => {
  const activate = vi.fn();
  api.list.mockResolvedValue([new workspace.WorkspaceInfo({ id: 'new', name: 'Novo', path: 'C:/new', tab_count: 0 })]);
  render(<MemoryRouter><WorkspaceListDialog onActivate={activate} onClose={() => {}} /></MemoryRouter>);
  const cell = await screen.findByRole('gridcell', { name: 'Novo' });
  fireEvent.click(cell); press(cell, 'Enter');
  expect(activate).toHaveBeenCalledWith('new');
});
it('deriva a situação atual e impede Delete depois de trocar o workspace', async () => {
  const { cell } = await openList();
  act(() => useWorkspaceStore.setState({ workspace: { ...useWorkspaceStore.getState().workspace!, id: 'other' } }));
  expect(within(cell.closest('[role="row"]')!).getByText('workspace.activeStatus')).toBeTruthy();
  press(cell, 'Delete');
  expect(screen.queryByRole('alertdialog')).toBeNull();
});
it('Ctrl+F não retira foco da confirmação filha', async () => {
  const { cell } = await openList();
  press(cell, 'Delete');
  const dialog = await screen.findByRole('alertdialog');
  const button = within(dialog).getAllByRole('button')[0];
  act(() => button.focus());
  fireEvent.keyDown(button, { key: 'f', code: 'KeyF', ctrlKey: true });
  expect(button).toHaveFocus();
});

it('avisa quando não consegue atualizar a lista ao abrir', async () => {
  api.list.mockRejectedValue(new Error('index unavailable'));
  render(<MemoryRouter><WorkspaceListDialog onActivate={() => {}} onClose={() => {}} /></MemoryRouter>);
  await screen.findByText('commandPalette.executionFailed');
  expect(api.announce).toHaveBeenCalledWith('commandPalette.executionFailed');
});

it('falha ao remover anuncia somente o erro, sem confirmar sucesso', async () => {
  api.remove.mockRejectedValueOnce(new Error('write failed'));
  const { cell } = await openList();
  press(cell, 'Delete');
  await screen.findByRole('alertdialog');
  fireEvent.click(screen.getByRole('button', { name: /confirmar|confirm/i }));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
  expect(api.announce).toHaveBeenCalledExactlyOnceWith('commandPalette.executionFailed');
  expect(screen.getByRole('gridcell', { name: 'Projeto' })).toBeInTheDocument();
});
