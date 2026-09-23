import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CommandSettingsSnapshot } from '../types/commandSettingsTypes';

const bridge = vi.hoisted(() => ({
  get: vi.fn(), mutate: vi.fn(), prepare: vi.fn(), activate: vi.fn(),
}));
vi.mock('../services/commandSettings', () => ({
  getCommandSettingsForScope: bridge.get,
  mutateCommandSettings: bridge.mutate,
  prepareManualCommandLayerForScope: bridge.prepare,
  setCommandLayerActiveForScope: bridge.activate,
}));
vi.mock('../services/commandDeckCapture', () => ({ beginCommandDeckCapture: vi.fn(), cancelCommandDeckCapture: vi.fn() }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: vi.fn() }) }));
vi.mock('../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: vi.fn() }) }));
vi.mock('../store/authStore', () => ({ useAuthStore: (select: (state: unknown) => unknown) => select({ user: { userId: 'owner', sessionId: 'session' }, status: { vaultUnlocked: true } }) }));
vi.mock('../store/workspaceStore', () => ({ useWorkspaceStore: (select: (state: unknown) => unknown) => select({ workspace: { id: 'workspace-1' } }) }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => vi.fn() }));
import CommandSettingsPage from './CommandSettingsPage';

function configuration(): CommandSettingsSnapshot {
  return {
    scope: 'global', revision: 7, fingerprint: 'snapshot-fingerprint', keyboardOperational: true,
    layers: [{ id: 'builtin', name: 'Padrões', description: '', builtin: true, enabled: true, active: true, manualReady: false, manualActive: false }],
    bindings: [{ id: 'persisted-delta', defaultId: 'builtin.default', layerId: 'builtin', commandId: 'workspace.tab.next', triggerType: 'keyboard.local', triggerSpec: '{"version":1,"code":"KeyY","modifiers":["Control"]}', enabled: true, customized: true, readOnly: false, reviewStatus: 'needs_review', effect: 'execute', arguments: {}, condition: { version: 1, clauses: [] }, replacesDefaultVersion: '1', replacesDefaultFingerprint: 'old', currentDefaultVersion: '2', currentDefaultFingerprint: 'current' }],
    commands: [{ id: 'workspace.tab.next', name: 'Próxima aba', description: '', allowedSources: ['keyboard.local', 'palette'] }], rules: [],
  };
}

async function bindingAction(name: string) {
  const grid = await screen.findByRole('grid', { name: 'commandSettings.commands' });
  fireEvent.click(within(grid).getByRole('button', { name: 'common.actions' }));
  const item = await screen.findByRole('menuitem', { name });
  await act(async () => { fireEvent.click(item); });
}

describe('CommandSettingsPage contrato avançado', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    bridge.get.mockResolvedValue(configuration());
    bridge.mutate.mockResolvedValue({ committed: true, published: true, id: 'mutation' });
    bridge.prepare.mockResolvedValue({ committed: true, published: true, id: 'rule' });
    bridge.activate.mockResolvedValue({ committed: true, published: true, id: 'rule' });
  });

  it('não salva argumentos inválidos usando silenciosamente o valor anterior', async () => {
    const snapshot = configuration();
    snapshot.bindings[0] = { ...snapshot.bindings[0], reviewStatus: 'active', arguments: { amount: 3 } };
    bridge.get.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    fireEvent.change(screen.getByLabelText('commandSettings.argumentEditor.key 1'), { target: { value: '' } });
    await waitFor(() => expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled());
    expect(bridge.mutate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('commandSettings.argumentEditor.key 1'), { target: { value: 'amount' } });
    await waitFor(() => expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled());
  });

  it('distingue a aba tasklist da página tasklists ao editar uma condição', async () => {
    const snapshot = configuration();
    snapshot.bindings[0] = { ...snapshot.bindings[0], reviewStatus: 'active',
      condition: { version: 1, clauses: [{ field: 'surface.type', value: 'tasklist' }] } };
    bridge.get.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    const value = screen.getByLabelText('commandSettings.conditions.value');
    expect(value).toHaveValue('tasklist');
    expect(within(value).getByRole('option', { name: 'commandSettings.tasklistSurface' })).toHaveValue('tasklist');
    expect(within(value).getByRole('option', { name: 'commandSettings.conditionValues.tasklists' })).toHaveValue('tasklists');
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledWith(expect.objectContaining({
      binding: expect.objectContaining({ condition: { version: 1, clauses: [{ field: 'surface.type', value: 'tasklist' }] } }),
    })));
  });

  it('restaura pelo ID persistido do delta e vincula ao snapshot exibido', async () => {
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.restore');
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      operation: 'binding_restore', id: 'persisted-delta', scope: 'global',
      expectedRevision: 7, expectedFingerprint: 'snapshot-fingerprint',
    })));
  });

  it('revisa a personalização contra o default atual, não o fingerprint antigo', async () => {
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.rebaseDefault');
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      operation: 'default_rebase',
      default: expect.objectContaining({ bindingId: 'persisted-delta', default: { id: 'builtin.default', version: '2', fingerprint: 'current' } }),
    })));
  });

  it('restauração total usa somente o escopo selecionado', async () => {
    bridge.get.mockImplementation((_locale: string, scope: string) => Promise.resolve({ ...configuration(), scope, revision: scope === 'workspace' ? 8 : 7, fingerprint: scope === 'workspace' ? 'workspace-fingerprint' : 'snapshot-fingerprint' }));
    render(<CommandSettingsPage />);
    await screen.findByRole('heading', { name: 'Padrões' });
    await act(async () => { fireEvent.change(screen.getByLabelText('commandSettings.scope.label'), { target: { value: 'workspace' } }); });
    await waitFor(() => expect(bridge.get).toHaveBeenLastCalledWith(expect.any(String), 'workspace'));
    const restore = await screen.findByRole('button', { name: 'commandSettings.actions.restoreAll' });
    await act(async () => { fireEvent.click(restore); });
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ operation: 'config_restore', scope: 'workspace', expectedRevision: 8, expectedFingerprint: 'workspace-fingerprint' })));
  });

  it('ativação manual escopada não é substituída por rule_enable', async () => {
    const snapshot = configuration();
    snapshot.bindings = [];
    snapshot.layers = [{ id: 'local-layer', name: 'Camada local', description: '', builtin: false, enabled: true, active: false, manualReady: true, manualActive: false, workspaceId: 'workspace-1' }];
    bridge.get.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await screen.findByRole('heading', { name: 'Camada local' });
    await act(async () => { fireEvent.change(screen.getByLabelText('commandSettings.scope.label'), { target: { value: 'workspace' } }); });
    await waitFor(() => expect(bridge.get).toHaveBeenLastCalledWith(expect.any(String), 'workspace'));
    const grid = screen.getByRole('grid', { name: 'commandSettings.layers' });
    fireEvent.click(within(grid).getByRole('button', { name: 'common.actions' }));
    const activate = await screen.findByRole('menuitem', { name: 'commandSettings.activateManual' });
    await act(async () => { fireEvent.click(activate); });
    await waitFor(() => expect(bridge.activate).toHaveBeenCalledExactlyOnceWith('workspace', 'local-layer', true));
    expect(bridge.mutate).not.toHaveBeenCalled();
  });

  it('não oferece mutações para uma personalização global herdada no workspace', async () => {
    const snapshot = configuration();
    snapshot.bindings[0] = { ...snapshot.bindings[0], inherited: true, readOnly: true };
    bridge.get.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await screen.findByRole('heading', { name: 'Padrões' });
    await act(async () => { fireEvent.change(screen.getByLabelText('commandSettings.scope.label'), { target: { value: 'workspace' } }); });
    const grid = await screen.findByRole('grid', { name: 'commandSettings.commands' });
    const actions = within(grid).queryByRole('button', { name: 'common.actions' });
    if (actions) {
      fireEvent.click(actions);
    }
    expect(screen.queryByRole('menuitem', { name: 'commandSettings.actions.restore' })).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'commandSettings.actions.editBinding' })).not.toBeInTheDocument();
    expect(bridge.mutate).not.toHaveBeenCalled();
  });

  it('editar prioridade preserva argumentos tipados e apresentação da personalização', async () => {
    const snapshot = configuration();
    const args = { amount: 3, confirm: false, nested: { names: ['one', 'two'] } };
    snapshot.bindings[0] = { ...snapshot.bindings[0], reviewStatus: 'active', arguments: args, presentation: { version: 1, title: 'Próxima' } };
    bridge.get.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '12' } });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'common.save' })); });
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      operation: 'binding_update', id: 'persisted-delta',
      binding: expect.objectContaining({ id: 'persisted-delta', arguments: args, presentation: { version: 1, title: 'Próxima' }, resolutionPriority: 12, replacesDefaultId: 'builtin.default' }),
    })));
  });
});
