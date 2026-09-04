import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../test/a11yAxe';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const updateConfig = vi.fn();
const announce = vi.fn();
let externalChange: 'autoReload' | 'prompt' = 'autoReload';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock('../lib/i18n', () => ({
  default: { language: 'pt-BR', changeLanguage: vi.fn() },
  LANGUAGES: [{ id: 'pt-BR', label: 'Português', nativeLabel: 'Português' }],
}));
vi.mock('../hooks/useTheme', () => ({
  useTheme: () => ({ theme: 'assistente', setTheme: vi.fn() }),
  THEMES: [{ id: 'assistente', label: 'Assistente', description: 'Tema' }],
}));
vi.mock('../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: unknown) => unknown) =>
    selector({
      config: {
        decisionAlertSound: true,
        preventScreenLock: true,
        editor: { externalChange },
      },
      updateConfig,
    }),
}));
vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce }),
}));
vi.mock('../hooks/useContentPageLandmarks', () => ({
  useContentPageLandmarks: vi.fn(),
}));
vi.mock('../hooks/useRadioGroup', () => ({
  useRadioGroup: () => ({ current: null }),
}));

import AppearancePage from './AppearancePage';

describe('AppearancePage editor.externalChange', () => {
  beforeEach(() => {
    externalChange = 'autoReload';
    updateConfig.mockReset();
    announce.mockReset();
  });

  it('exibe o default e salva a opção prompt', async () => {
    const user = userEvent.setup();
    render(<AppearancePage />);

    const select = screen.getByLabelText('appearance.editorExternalChange');
    expect(select).toHaveValue('autoReload');

    await user.selectOptions(select, 'prompt');

    expect(updateConfig).toHaveBeenCalledWith({
      editor: { externalChange: 'prompt' },
    });
    expect(announce).toHaveBeenCalledWith(
      'appearance.announce.editorExternalChangePrompt',
    );
  });

  it('não tem violações de acessibilidade', async () => {
    const { container } = render(<AppearancePage />);
    expect(await axe(container)).toHaveNoViolations();
  });
});
