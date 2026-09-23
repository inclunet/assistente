import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CommandSettingsBindingUnavailableError, deleteCommandBinding, getCommandSettings, getCommandSettingsForScope, mutateCommandSettings, prepareManualCommandLayer, prepareManualCommandLayerForScope, saveCommandBinding, saveCommandLayer, setCommandLayerActive, setCommandLayerActiveForScope, setDefaultCommandSuppressed } from './commandSettings';

const bridge = vi.hoisted(() => ({
  GetCommandSettings: vi.fn(),
  SaveCommandLayer: vi.fn(),
  SaveCommandBinding: vi.fn(),
  DeleteCommandBinding: vi.fn(),
  SetDefaultCommandSuppressed: vi.fn(),
  PrepareManualCommandLayer: vi.fn(),
  SetCommandLayerActive: vi.fn(),
  GetCommandSettingsForScope: vi.fn(),
  MutateCommandSettings: vi.fn(),
  PrepareManualCommandLayerForScope: undefined,
  SetCommandLayerActiveForScope: undefined,
}));

// Substitui somente a fronteira Wails nos testes; não cria exports de produção.
vi.mock('@wailsjs/go/app/App', () => bridge);

describe('commandSettings bridge', () => {
  beforeEach(() => vi.resetAllMocks());

  it('envia locale sem inventar identidade ou workspace', async () => {
    const snapshot = { layers: [], bindings: [], commands: [], keyboardOperational: false };
    bridge.GetCommandSettings.mockResolvedValue(snapshot);
    await expect(getCommandSettings('es')).resolves.toBe(snapshot);
    expect(bridge.GetCommandSettings).toHaveBeenCalledExactlyOnceWith('es');
  });

  it('normaliza ID de criação para a assinatura Go', async () => {
    const layer = { name: 'Pessoal', description: '', enabled: true };
    await saveCommandLayer(layer);
    expect(bridge.SaveCommandLayer).toHaveBeenCalledExactlyOnceWith({ ...layer, id: '' });
    const binding = { layerId: 'layer-id', commandId: 'workspace.list', triggerType: 'palette' as const, triggerSpec: '{"version":1,"selection":"workspace.list"}', enabled: true };
    await saveCommandBinding(binding);
    expect(bridge.SaveCommandBinding).toHaveBeenCalledExactlyOnceWith({ ...binding, id: '' });
  });

  it('preserva commit sem publicação e nunca repete a gravação', async () => {
    const result = { committed: true, published: false, id: 'existing-layer' };
    bridge.SaveCommandLayer.mockResolvedValue(result);
    const request = { id: result.id, name: 'Atualizada', description: '', enabled: true };
    await expect(saveCommandLayer(request)).resolves.toBe(result);
    expect(bridge.SaveCommandLayer).toHaveBeenCalledExactlyOnceWith(request);
  });

  it('separa exclusão de binding e supressão/restauração de padrão', async () => {
    await deleteCommandBinding('personal-binding');
    await setDefaultCommandSuppressed('builtin.palette.workspace.list', true);
    await setDefaultCommandSuppressed('builtin.palette.workspace.list', false);
    expect(bridge.DeleteCommandBinding).toHaveBeenCalledExactlyOnceWith('personal-binding');
    expect(bridge.SetDefaultCommandSuppressed.mock.calls).toEqual([
      ['builtin.palette.workspace.list', true],
      ['builtin.palette.workspace.list', false],
    ]);
  });

  it('encaminha preparação e ativação manual sem identidade de cliente', async () => {
    const prepared = { committed: true, published: true, id: 'layer-id' };
    const activated = { committed: true, published: false, id: 'layer-id' };
    bridge.PrepareManualCommandLayer.mockResolvedValue(prepared);
    bridge.SetCommandLayerActive.mockResolvedValue(activated);

    await expect(prepareManualCommandLayer('layer-id')).resolves.toBe(prepared);
    await expect(setCommandLayerActive('layer-id', true)).resolves.toBe(activated);
    expect(bridge.PrepareManualCommandLayer).toHaveBeenCalledExactlyOnceWith('layer-id');
    expect(bridge.SetCommandLayerActive).toHaveBeenCalledExactlyOnceWith('layer-id', true);
  });

  it('propaga recusa sem retry', async () => {
    const failure = new Error('denied');
    bridge.DeleteCommandBinding.mockRejectedValue(failure);
    await expect(deleteCommandBinding('binding')).rejects.toBe(failure);
    expect(bridge.DeleteCommandBinding).toHaveBeenCalledTimes(1);
  });

  it('falha explicitamente quando os bindings de escopo ainda não foram gerados', async () => {
    const mockModule = bridge as unknown as Record<string, unknown>;
    const getScope = mockModule.GetCommandSettingsForScope;
    const mutate = mockModule.MutateCommandSettings;
    mockModule.GetCommandSettingsForScope = undefined;
    mockModule.MutateCommandSettings = undefined;
    const request = {
      locale: 'pt-BR',
      scope: 'global' as const,
      operation: 'config_restore' as const,
      id: '',
      expectedRevision: 3,
      expectedFingerprint: 'fp-current',
    };

    await expect(getCommandSettingsForScope('pt-BR', 'global')).rejects.toBeInstanceOf(
      CommandSettingsBindingUnavailableError
    );
    await expect(mutateCommandSettings(request)).rejects.toBeInstanceOf(
      CommandSettingsBindingUnavailableError
    );
    await expect(prepareManualCommandLayerForScope('global', 'layer')).rejects.toBeInstanceOf(
      CommandSettingsBindingUnavailableError
    );
    await expect(setCommandLayerActiveForScope('global', 'layer', true)).rejects.toBeInstanceOf(
      CommandSettingsBindingUnavailableError
    );
    expect(bridge.GetCommandSettings).not.toHaveBeenCalled();
    expect(bridge.SaveCommandLayer).not.toHaveBeenCalled();
    mockModule.GetCommandSettingsForScope = getScope;
    mockModule.MutateCommandSettings = mutate;
  });

  it('codifica a mutação wire e decodifica documentos do snapshot sem expor JSON à UI', async () => {
    bridge.GetCommandSettingsForScope.mockResolvedValue({
      scope: 'global',
      revision: 4,
      fingerprint: 'fp',
      layers: [],
      bindings: [{
        id: 'binding',
        layerId: 'layer',
        commandId: 'workspace.list',
        triggerType: 'keyboard.local',
        triggerSpec: '{}',
        enabled: true,
        customized: true,
        readOnly: false,
        defaultId: '',
        reviewStatus: 'active',
        arguments: '{"selection":"current"}',
        condition: '{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}',
      }],
      rules: [],
      commands: [],
      keyboardOperational: true,
    });
    bridge.MutateCommandSettings.mockResolvedValue({ committed: true, published: true, id: 'binding' });

    const snapshot = await getCommandSettingsForScope('en', 'global');
    expect(snapshot.bindings[0].arguments).toEqual({ selection: 'current' });
    expect(snapshot.bindings[0].condition).toEqual({
      version: 1,
      clauses: [{ field: 'app.focused', value: true }],
    });

    await mutateCommandSettings({
      locale: 'en',
      scope: 'global',
      operation: 'binding_update',
      id: 'binding',
      expectedRevision: 4,
      expectedFingerprint: 'fp',
      binding: {
        id: 'binding',
        layerId: 'layer',
        commandId: 'workspace.list',
        triggerType: 'keyboard.local',
        triggerSpec: '{}',
        arguments: { selection: 'current' },
        condition: { version: 1, clauses: [{ field: 'app.focused', value: true }] },
        effect: 'execute',
        enabled: true,
        resolutionPriority: 1,
      },
    });
    expect(bridge.MutateCommandSettings).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      locale: 'en',
      binding: expect.objectContaining({
        arguments: { selection: 'current' },
        condition: { version: 1, clauses: [{ field: 'app.focused', op: 'eq', value: true }] },
        presentation: undefined,
      }),
    }));
  });

  it('rejeita o snapshot quando uma cláusula de binding é inválida', async () => {
    bridge.GetCommandSettingsForScope.mockResolvedValue({
      layers: [],
      bindings: [{
        id: 'invalid', layerId: 'layer', commandId: 'cmd', triggerType: 'keyboard.local', triggerSpec: '{}',
        enabled: true, customized: true, readOnly: false, defaultId: '', reviewStatus: 'needs_review',
        condition: '{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true},{"field":"surface.type","value":"editor"}]}',
      }],
      commands: [],
      keyboardOperational: true,
    });

    await expect(getCommandSettingsForScope('pt-BR', 'global')).rejects.toThrow('Invalid command settings snapshot binding:invalid');
  });

  it('rejeita condição inválida de regra em vez de abrir a regra', async () => {
    bridge.GetCommandSettingsForScope.mockResolvedValue({
      layers: [],
      bindings: [],
      rules: [{
        id: 'rule', layerId: 'layer', mode: 'manual', condition: '{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":"true"}]}',
        lifecycle: 'persistent', enabled: true, reviewStatus: 'active',
      }],
      commands: [],
      keyboardOperational: true,
    });

    await expect(getCommandSettingsForScope('pt-BR', 'global')).rejects.toThrow('Invalid command settings snapshot rule:rule');
  });
});
