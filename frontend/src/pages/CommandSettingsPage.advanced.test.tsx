import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { CommandSettingsSnapshot, CommandSettingsMutationRequest } from '../types/commandSettingsTypes';

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
  function deckConfiguration() {
    const snapshot = configuration();
    snapshot.commands[0].allowedSources.push('streamdeck.key');
    snapshot.bindings[0] = { ...snapshot.bindings[0], reviewStatus: 'active',
      triggerType: 'streamdeck.key', triggerSpec: '{"version":1,"device":"deck","key":0}',
      presentation: { version: 1, icon: 'settings', status_label_keys: { active: 'status.active' },
        title_by_locale: { 'pt-BR': 'Antes', en: 'Before', es: 'Antes' } } };
    return snapshot;
  }

  it('salva títulos normalizados, remove locale vazio e reabre após reload preservando metadados', async () => {
    const snapshot = deckConfiguration();
    bridge.get.mockImplementation(async () => structuredClone(snapshot));
    bridge.mutate.mockImplementation(async (request: CommandSettingsMutationRequest) => {
      snapshot.bindings[0] = { ...snapshot.bindings[0], ...request.binding };
      return { committed: true, published: true, id: snapshot.bindings[0].id };
    });
    const page = render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.icon.label'), { target: { value: 'folder' } });
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.locales.ptBR'), { target: { value: '  Meu título  ' } });
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.locales.en'), { target: { value: '   ' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(bridge.get).toHaveBeenCalledTimes(2));
    expect(bridge.mutate).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({
      presentation: { version: 1, icon: 'folder', status_label_keys: { active: 'status.active' },
        title_by_locale: { 'pt-BR': 'Meu título', es: 'Antes' } },
    }) }));
    page.unmount();
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    expect(screen.getByLabelText('commandSettings.presentation.icon.label')).toHaveValue('folder');
    expect(screen.getByLabelText('commandSettings.presentation.locales.ptBR')).toHaveValue('Meu título');
    expect(screen.getByLabelText('commandSettings.presentation.locales.en')).toHaveValue('');
    expect(screen.getByLabelText('commandSettings.presentation.locales.es')).toHaveValue('Antes');
  });

  it('envia image_upload em base64 sem prefixo, substitui image_ref e preserva título e ícone', async () => {
    const snapshot = deckConfiguration();
    snapshot.bindings[0].presentation = {
      ...snapshot.bindings[0].presentation,
      image_ref: 'sha256-somente-backend',
    };
    bridge.get.mockImplementation(async () => structuredClone(snapshot));
    bridge.mutate.mockImplementation(async (request: CommandSettingsMutationRequest) => {
      snapshot.bindings[0] = { ...snapshot.bindings[0], ...request.binding };
      return { committed: true, published: true, id: snapshot.bindings[0].id };
    });

    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), {
      target: { files: [new File(['new image'], 'do-not-persist.jpg', { type: 'image/jpeg' })] },
    });
    expect(await screen.findByText('commandSettings.presentation.image.selected')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));

    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledWith(expect.objectContaining({
      binding: expect.objectContaining({
        presentation: expect.objectContaining({
          icon: 'settings',
          title_by_locale: { 'pt-BR': 'Antes', en: 'Before', es: 'Antes' },
          image_upload: expect.stringMatching(/^(?!data:).+/),
        }),
      }),
    })));
    const request = bridge.mutate.mock.calls[0][0] as CommandSettingsMutationRequest;
    expect(request.binding?.presentation).not.toHaveProperty('image_ref');
    expect(request.binding?.presentation?.image_upload).not.toContain('do-not-persist.jpg');
  });

  it('remove o ícone desconhecido ao salvar e recarregar sem perder metadados', async () => {
    const snapshot = deckConfiguration();
    snapshot.bindings[0] = {
      ...snapshot.bindings[0],
      presentation: { version: 1, icon: 'legacy-deck-token', status_label_keys: { active: 'status.active' },
        title_by_locale: { 'pt-BR': 'Antes', en: 'Before', es: 'Antes' } },
    };
    bridge.get.mockImplementation(async () => structuredClone(snapshot));
    bridge.mutate.mockImplementation(async (request: CommandSettingsMutationRequest) => {
      snapshot.bindings[0] = { ...snapshot.bindings[0], ...request.binding };
      return { committed: true, published: true, id: snapshot.bindings[0].id };
    });

    const page = render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    const icon = screen.getByLabelText('commandSettings.presentation.icon.label');
    expect(icon).toHaveValue('legacy-deck-token');
    fireEvent.change(icon, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({
      presentation: { version: 1, status_label_keys: { active: 'status.active' },
        title_by_locale: { 'pt-BR': 'Antes', en: 'Before', es: 'Antes' } },
    }) })));

    page.unmount();
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    expect(screen.getByLabelText('commandSettings.presentation.icon.label')).toHaveValue('');
    expect(screen.getByLabelText('commandSettings.presentation.locales.ptBR')).toHaveValue('Antes');
  });

  it.each(['bad\0title', '😀'.repeat(257)])('bloqueia título inválido também após alternar origem (%#)', async title => {
    bridge.get.mockResolvedValue(deckConfiguration());
    render(<CommandSettingsPage />);
    await bindingAction('commandSettings.actions.editBinding');
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.locales.en'), { target: { value: title } });
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'palette' } });
    expect(screen.queryByLabelText('commandSettings.presentation.locales.en')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    expect(bridge.mutate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'streamdeck.key' } });
    expect(screen.getByLabelText('commandSettings.presentation.locales.en')).toHaveValue(title);
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.locales.en'), { target: { value: '😀'.repeat(256) } });
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'palette' } });
    expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(bridge.mutate).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({
      triggerType: 'palette', presentation: expect.objectContaining({ icon: 'settings',
        title_by_locale: { 'pt-BR': 'Antes', en: '😀'.repeat(256), es: 'Antes' } }),
    }) })));
  });

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
