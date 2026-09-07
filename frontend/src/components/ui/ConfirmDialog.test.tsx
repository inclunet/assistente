import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ConfirmDialog } from './ConfirmDialog';

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

describe('ConfirmDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('aciona confirmar e cancelar', async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDialog
        isOpen={true}
        title="Apagar"
        message="Tem certeza"
        confirmText="Confirmar"
        cancelText="Cancelar"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />,
    );

    expect(screen.getByText('Tem certeza')).toBeInTheDocument();
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));

    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).toHaveBeenCalled();
  });

  it('coloca Confirmar antes de Cancelar no DOM (AEP-0090)', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        title="Apagar"
        message="Tem certeza"
        confirmText="Sim, apagar"
        cancelText="Não"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const actions = document.querySelector('[data-dialog-actions]');
    expect(actions).not.toBeNull();
    const footerButtons = Array.from(actions!.querySelectorAll('button'));
    expect(footerButtons.map((b) => b.textContent)).toEqual(['Sim, apagar', 'Não']);
  });

  it('mapeia danger para severidade destrutiva com alertdialog', async () => {
    render(
      <ConfirmDialog
        isOpen
        title="Excluir"
        message="Irreversível"
        variant="danger"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    });
  });
});
