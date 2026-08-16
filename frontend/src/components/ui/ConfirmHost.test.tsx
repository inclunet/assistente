import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ConfirmHost } from './ConfirmHost';

const confirmSpy = vi.fn();
const cancelSpy = vi.fn();

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
    announceRequest: vi.fn(),
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playSound: vi.fn(),
  SOUND_TYPES: { ALERT: 'alert' },
}));

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (s: { config: { decisionAlertSound: boolean } }) => unknown) =>
    selector({ config: { decisionAlertSound: false } }),
}));

vi.mock('../../store/confirmStore', () => ({
  useConfirmStore: (selector: (state: {
    active: { title: string; message: string; confirmText?: string; cancelText?: string } | null;
    confirm: () => void;
    cancel: () => void;
  }) => unknown) =>
    selector({
      active: { title: 'Apagar', message: 'Tem certeza', confirmText: 'Confirmar', cancelText: 'Cancelar' },
      confirm: confirmSpy,
      cancel: cancelSpy,
    }),
}));

describe('ConfirmHost', () => {
  it('renderiza dialogo e aciona callbacks', () => {
    render(<ConfirmHost />);

    expect(screen.getByText('Tem certeza')).toBeInTheDocument();
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();

    const actions = document.querySelector('[data-dialog-actions]');
    expect(actions).not.toBeNull();
    const footerButtons = Array.from(actions!.querySelectorAll('button'));
    expect(footerButtons.map((b) => b.textContent)).toEqual(['Confirmar', 'Cancelar']);

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));

    expect(cancelSpy).toHaveBeenCalled();
    expect(confirmSpy).toHaveBeenCalled();
  });
});
