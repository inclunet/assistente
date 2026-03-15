import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const handlers: Record<string, (payload: unknown) => void> = {};
const mockStartUpdate = vi.fn();
const mockAddToast = vi.fn();
const mockNavigate = vi.fn();

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (eventName: string, handler: (payload: unknown) => void) => {
    handlers[eventName] = handler;
    return () => {
      delete handlers[eventName];
    };
  },
}));

vi.mock('@wailsjs/go/main/App', () => ({
  StartUpdate: () => mockStartUpdate(),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: mockAddToast,
  }),
}));

import UpdatePage from './UpdatePage';

describe('UpdatePage', () => {
  beforeEach(() => {
    mockStartUpdate.mockReset();
    mockAddToast.mockReset();
    mockNavigate.mockReset();
    Object.keys(handlers).forEach((key) => delete handlers[key]);
  });

  it('mostra estado de sucesso e permite voltar', async () => {
    const user = userEvent.setup();
    render(<UpdatePage />);

    await act(async () => {
      handlers['update:completed']?.({ message: 'Atualizado!' });
    });

    await waitFor(() => {
      expect(screen.getByText('update.successTitle')).toBeInTheDocument();
    });

    const backButton = screen.getByRole('button', { name: 'update.buttons.backToChat' });
    await user.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/');
  });

  it('mostra erro e permite tentar novamente', async () => {
    const user = userEvent.setup();
    render(<UpdatePage />);

    await act(async () => {
      handlers['update:error']?.({ error: 'Falha' });
    });

    await waitFor(() => {
      expect(screen.getByText('update.errorTitle')).toBeInTheDocument();
    });

    const retryButton = screen.getByRole('button', { name: 'update.buttons.retry' });
    await user.click(retryButton);

    expect(mockStartUpdate).toHaveBeenCalled();
  });
});
