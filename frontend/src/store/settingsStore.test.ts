import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import {
  defaultConfig,
  sanitizeConfig,
  useSettingsStore,
} from './settingsStore';

describe('settingsStore editor.externalChange', () => {
  beforeEach(() => {
    localStorage.clear();
    useSettingsStore.setState({
      config: {
        ...defaultConfig,
        editor: { ...defaultConfig.editor },
      },
    });
  });

  afterEach(() => {
    localStorage.clear();
    useSettingsStore.setState({
      config: {
        ...defaultConfig,
        editor: { ...defaultConfig.editor },
      },
    });
  });

  it('usa autoReload como padrão para configuração antiga', () => {
    expect(sanitizeConfig({ theme: 'light' }).editor.externalChange).toBe('autoReload');
  });

  it('preserva apenas valores válidos ao migrar', () => {
    expect(
      sanitizeConfig({ editor: { externalChange: 'prompt' } }).editor.externalChange,
    ).toBe('prompt');
    expect(
      sanitizeConfig({
        editor: { externalChange: 'inválido' as 'prompt' },
      }).editor.externalChange,
    ).toBe('autoReload');
  });

  it('persiste a preferência prompt sem perder as demais configurações', () => {
    useSettingsStore.getState().updateConfig({
      editor: { externalChange: 'prompt' },
    });

    expect(useSettingsStore.getState().config.editor.externalChange).toBe('prompt');
    expect(useSettingsStore.getState().config.decisionAlertSound).toBe(true);

    const persisted = JSON.parse(
      localStorage.getItem('assistente-settings') ?? '{}',
    ) as { state?: { config?: typeof defaultConfig } };
    expect(persisted.state?.config?.editor.externalChange).toBe('prompt');
  });
});
