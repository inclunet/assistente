import { render, screen, waitFor } from '@testing-library/react';
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
});
