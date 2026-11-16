import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../test/a11yAxe';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getSettings, saveSettings, announce, translate } = vi.hoisted(() => ({
  getSettings: vi.fn(), saveSettings: vi.fn(), announce: vi.fn(), translate: (key: string) => key,
}));
vi.mock('@wailsjs/go/wailsapi/Settings', () => ({ GetLogSettings: getSettings, SaveLogSettings: saveSettings }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: translate }) }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));
vi.mock('../hooks/useContentPageLandmarks', () => ({ useContentPageLandmarks: vi.fn() }));

import LogSettingsPage from './LogSettingsPage';
import { ActivePanelContext } from '../hooks/useActivePanelShortcut';

describe('LogSettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSettings.mockResolvedValue({ settings: { enabled: false, path: '', maxMiB: 50, backups: 5, rotateHours: 24 }, overridden: false });
    saveSettings.mockResolvedValue(undefined);
  });

  it('salva o arquivo e os limites escolhidos', async () => {
    const user = userEvent.setup();
    render(<LogSettingsPage />);
    await user.click(await screen.findByLabelText('logSettings.enabled'));
    await user.type(screen.getByLabelText(/logSettings\.path/), 'C:\\logs\\app.log');
    await user.clear(screen.getByLabelText('logSettings.backups'));
    await user.type(screen.getByLabelText('logSettings.backups'), '3');
    await user.click(screen.getByRole('button', { name: 'logSettings.save' }));
    await waitFor(() => expect(saveSettings).toHaveBeenCalledWith({ enabled: true, path: 'C:\\logs\\app.log', maxMiB: 50, backups: 3, rotateHours: 24 }));
    expect(announce).toHaveBeenCalledWith('logSettings.saved');
  });

  it('informa quando a flag prevalece e oferece controles acessíveis', async () => {
    getSettings.mockResolvedValue({ settings: { enabled: false, path: '', maxMiB: 50, backups: 5, rotateHours: 24 }, overridden: true });
    const { container } = render(<LogSettingsPage />);
    await screen.findByText('logSettings.overridden');
    expect(await axe(container)).toHaveNoViolations();
  });

  it('informa indisponibilidade síncrona da ponte', async () => {
    getSettings.mockImplementation(() => { throw new Error('ponte indisponível'); });
    render(<LogSettingsPage />);
    expect(await screen.findByText('logSettings.loadError')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('logSettings.loadError', 'assertive');
  });

  it('atualiza diagnóstico ao reabrir a aba sem apagar alterações', async () => {
    const user = userEvent.setup();
    const initial = { enabled: true, path: 'C:\\logs\\original.log', maxMiB: 50, backups: 5, rotateHours: 24 };
    getSettings.mockResolvedValueOnce({ settings: initial, overridden: false, error: '' })
      .mockResolvedValue({ settings: initial, overridden: false, error: 'disco cheio' });
    const view = render(<ActivePanelContext.Provider value><LogSettingsPage /></ActivePanelContext.Provider>);
    const path = await screen.findByLabelText(/logSettings\.path/);
    await user.clear(path);
    await user.type(path, 'C:\\logs\\editado.log');
    view.rerender(<ActivePanelContext.Provider value={false}><LogSettingsPage /></ActivePanelContext.Provider>);
    view.rerender(<ActivePanelContext.Provider value><LogSettingsPage /></ActivePanelContext.Provider>);
    expect(await screen.findByText('logSettings.startupError')).toBeInTheDocument();
    expect(screen.getByLabelText(/logSettings\.path/)).toHaveValue('C:\\logs\\editado.log');
  });

  it('mantém erro do destino efetivo após salvar preferências', async () => {
    const user = userEvent.setup();
    const initial = { enabled: false, path: '', maxMiB: 50, backups: 5, rotateHours: 24 };
    getSettings.mockResolvedValueOnce({ settings: initial, overridden: true, error: '' })
      .mockResolvedValue({ settings: initial, overridden: true, error: 'disco cheio' });
    render(<LogSettingsPage />);
    await user.click(await screen.findByRole('button', { name: 'logSettings.save' }));
    expect(await screen.findByText('logSettings.startupError')).toBeInTheDocument();
  });

  it('atualiza diagnóstico enquanto a aba permanece aberta', async () => {
    vi.useFakeTimers();
    try {
      const initial = { enabled: true, path: 'C:\\logs\\original.log', maxMiB: 50, backups: 5, rotateHours: 24 };
      getSettings.mockResolvedValueOnce({ settings: initial, overridden: false, error: '' })
        .mockResolvedValue({ settings: initial, overridden: false, error: 'disco cheio' });
      render(<LogSettingsPage />);
      await act(async () => { await Promise.resolve(); });
      fireEvent.change(screen.getByLabelText(/logSettings\.path/), { target: { value: 'C:\\logs\\editado.log' } });
      await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
      expect(screen.getByText('logSettings.startupError')).toBeInTheDocument();
      expect(screen.getByLabelText(/logSettings\.path/)).toHaveValue('C:\\logs\\editado.log');
    } finally {
      vi.useRealTimers();
    }
  });
});
