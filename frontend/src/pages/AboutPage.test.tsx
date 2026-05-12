import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetVersion = vi.fn();
const mockCheckForUpdates = vi.fn();
const mockStartUpdate = vi.fn();
const mockAddToast = vi.fn();

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetAppVersion: () => mockGetVersion(),
  CheckForUpdates: () => mockCheckForUpdates(),
  StartUpdate: () => mockStartUpdate(),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));

import AboutPage from './AboutPage';

describe('AboutPage', () => {
  beforeEach(() => {
    mockGetVersion.mockResolvedValue('1.0.0');
    mockCheckForUpdates.mockReset();
    mockStartUpdate.mockReset();
    mockAddToast.mockReset();
  });

  it('carrega versao atual', async () => {
    render(<AboutPage />);

    await waitFor(() => {
      expect(screen.getByText('1.0.0')).toBeInTheDocument();
    });
  });

  it('mostra update disponivel e inicia download', async () => {
    const user = userEvent.setup();
    mockCheckForUpdates.mockResolvedValue({
      available: true,
      currentVersion: '1.0.0',
      latestVersion: '1.1.0',
      releaseNotes: 'Notas',
    });

    render(<AboutPage />);

    const checkButton = await screen.findByRole('button', { name: 'about.buttons.checkUpdates' });
    await user.click(checkButton);

    await waitFor(() => {
      expect(screen.getByText('1.1.0')).toBeInTheDocument();
    });

    const updateButton = screen.getByRole('button', { name: 'about.buttons.updateNow' });
    await user.click(updateButton);

    expect(mockStartUpdate).toHaveBeenCalled();
  });

  it('mostra estado atualizado quando nao ha update', async () => {
    const user = userEvent.setup();
    mockCheckForUpdates.mockResolvedValue({
      available: false,
      currentVersion: '1.0.0',
      latestVersion: '1.0.0',
    });

    render(<AboutPage />);

    const checkButton = await screen.findByRole('button', { name: 'about.buttons.checkUpdates' });
    await user.click(checkButton);

    await waitFor(() => {
      expect(screen.getByText('about.upToDateTitle')).toBeInTheDocument();
    });
  });
});
