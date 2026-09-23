import { afterEach, describe, expect, it, vi } from 'vitest';
import * as generated from '@wailsjs/go/app/App';
import { app } from '@wailsjs/go/models';
import {
  getCommandSettingsForScope, mutateCommandSettings,
  prepareManualCommandLayerForScope, setCommandLayerActiveForScope,
} from './commandSettings';

// Não mockar App.js: este contrato atravessa os wrappers gerados pelo Wails.
describe('configuração pelos bindings oficiais', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('expõe as quatro operações escopadas e preserva o estado contextual', async () => {
    const snapshot = new app.CommandSettingsSnapshot({
      scope: 'workspace', revision: 7, fingerprint: 'snapshot', keyboardOperational: true,
      layers: [{ id: 'layer', name: 'Chat', description: '', builtin: false, enabled: true,
        active: false, activeKnown: false, manualReady: false, manualActive: false,
        activationModes: ['condition'], resolutionPriority: 0 }],
      bindings: [], rules: [], commands: [],
    });
    const api = {
      GetCommandSettingsForScope: vi.fn().mockResolvedValue(snapshot),
      MutateCommandSettings: vi.fn().mockResolvedValue({ committed: true, published: false, id: 'layer' }),
      PrepareManualCommandLayerForScope: vi.fn().mockResolvedValue({ committed: true, published: true, id: 'layer' }),
      SetCommandLayerActiveForScope: vi.fn().mockResolvedValue({ committed: true, published: true, id: 'layer' }),
    } satisfies Record<keyof Pick<typeof generated, 'GetCommandSettingsForScope' | 'MutateCommandSettings' | 'PrepareManualCommandLayerForScope' | 'SetCommandLayerActiveForScope'>, unknown>;
    vi.stubGlobal('go', { app: { App: api } });

    const result = await getCommandSettingsForScope('pt-BR', 'workspace');
    expect(result.layers[0].activeKnown).toBe(false);
    expect(result.layers[0].activationModes).toEqual(['condition']);
    expect(api.GetCommandSettingsForScope).toHaveBeenCalledExactlyOnceWith('pt-BR', 'workspace');
    await expect(mutateCommandSettings({ scope: 'workspace', locale: 'pt-BR', operation: 'layer_restore',
      id: 'layer', expectedRevision: 7, expectedFingerprint: 'snapshot' })).resolves.toEqual({ committed: true, published: false, id: 'layer' });
    expect(api.MutateCommandSettings).toHaveBeenCalledTimes(1);
    expect(api.MutateCommandSettings).toHaveBeenCalledWith(expect.objectContaining({ scope: 'workspace', expectedRevision: 7, expectedFingerprint: 'snapshot' }));
    await prepareManualCommandLayerForScope('workspace', 'layer');
    await setCommandLayerActiveForScope('workspace', 'layer', true);
    expect(api.PrepareManualCommandLayerForScope).toHaveBeenCalledExactlyOnceWith('workspace', 'layer');
    expect(api.SetCommandLayerActiveForScope).toHaveBeenCalledExactlyOnceWith('workspace', 'layer', true);
  });

  it('transporta condições e argumentos tipados sem perder false, zero ou estruturas', async () => {
    const mutate = vi.fn().mockResolvedValue({ committed: true, published: true, id: 'binding' });
    vi.stubGlobal('go', { app: { App: { MutateCommandSettings: mutate } } });
    await mutateCommandSettings({ scope: 'global', locale: 'pt-BR', operation: 'binding_create',
      expectedRevision: 4, expectedFingerprint: 'fp', binding: {
        layerId: 'layer', commandId: 'command', triggerType: 'keyboard.local', triggerSpec: '{}',
        arguments: { enabled: false, amount: 0, nested: { values: [1, 'two'] } },
        condition: { version: 1, clauses: [{ field: 'app.focused', value: true }] },
        enabled: true, effect: 'execute', resolutionPriority: 0,
      } });
    const wire = JSON.parse(JSON.stringify(mutate.mock.calls[0][0]));
    const request = new app.CommandSettingsMutationRequest(wire);
    expect(request.binding?.arguments).toEqual({ enabled: false, amount: 0, nested: { values: [1, 'two'] } });
    expect(request.binding?.condition?.clauses[0]).toEqual({ field: 'app.focused', op: 'eq', value: true });
    expect(request.expectedFingerprint).toBe('fp');
    expect(mutate).toHaveBeenCalledTimes(1);
  });
});
