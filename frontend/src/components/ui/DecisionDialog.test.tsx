import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { DecisionDialog } from './DecisionDialog';
import { axe } from '../../test/a11yAxe';

const announceRequest = vi.fn();
const playSound = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      const map: Record<string, string> = {
        'ui.decisionDialog.bodyHint': 'Há conteúdo adicional no diálogo para leitura.',
        'ui.modal.close': 'Fechar',
      };
      return map[key] ?? key;
    },
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
    announceRequest,
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playSound: (...args: unknown[]) => playSound(...args),
  SOUND_TYPES: { ALERT: 'alert' },
}));

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (s: { config: { decisionAlertSound: boolean } }) => unknown) =>
    selector({ config: { decisionAlertSound: true } }),
}));

describe('DecisionDialog', () => {
  beforeEach(() => {
    announceRequest.mockClear();
    playSound.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('usa alertdialog, anuncia e toca alerta na abertura', async () => {
    render(
      <DecisionDialog
        isOpen
        title="Apagar item"
        description="Tem certeza?"
        actions={[
          { id: 'confirm', label: 'Confirmar', primary: true, variant: 'danger' },
          { id: 'cancel', label: 'Cancelar', variant: 'outline' },
        ]}
        severity="destructive"
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    await waitFor(() => {
      expect(announceRequest).toHaveBeenCalledWith(
        expect.objectContaining({
          message: 'Apagar item. Tem certeza?',
          announcePriority: 'assertive',
        }),
      );
    });
    expect(playSound).toHaveBeenCalledWith('alert');
  });

  it('coloca ações primárias antes de cancelar (AEP-0090)', () => {
    render(
      <DecisionDialog
        isOpen
        title="Apagar"
        description="Tem certeza"
        actions={[
          { id: 'confirm', label: 'Sim, apagar', primary: true, variant: 'danger' },
          { id: 'cancel', label: 'Não', variant: 'outline' },
        ]}
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const actions = document.querySelector('[data-dialog-actions]');
    const footerButtons = Array.from(actions!.querySelectorAll('button'));
    expect(footerButtons.map((b) => b.textContent)).toEqual(['Sim, apagar', 'Não']);
  });

  it('Alt+mnemônico dispara a ação', () => {
    const onAction = vi.fn();
    render(
      <DecisionDialog
        isOpen
        title="Confirmar"
        description="Prosseguir?"
        actions={[
          { id: 'yes', label: '&Sim', primary: true },
          { id: 'no', label: '&Não', variant: 'outline' },
        ]}
        onAction={onAction}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.keyDown(document, { key: 's', altKey: true });
    expect(onAction).toHaveBeenCalledWith('yes');
  });

  it('envia rejectReason no extras ao rejeitar e na ordem AEP-0090', () => {
    const onAction = vi.fn();
    render(
      <DecisionDialog
        isOpen
        title="Editar"
        description="Aplicar?"
        rejectReason={{
          id: 'reject_reason',
          label: 'Motivo (opcional)',
          placeholder: 'Explique',
        }}
        actions={[
          { id: 'apply', label: 'Aplicar', primary: true, variant: 'primary' },
          { id: 'reject', label: 'Rejeitar', variant: 'outline' },
        ]}
        onAction={onAction}
        onCancel={vi.fn()}
      />,
    );

    const actions = document.querySelector('[data-dialog-actions]');
    const children = Array.from(actions!.children);
    expect(children[0].getAttribute('data-decision-action')).toBe('apply');
    expect(children[1].classList.contains('decision-dialog__reject-reason')).toBe(true);
    expect(children[2].getAttribute('data-decision-action')).toBe('reject');

    fireEvent.change(screen.getByLabelText('Motivo (opcional)'), {
      target: { value: '  Quero outro tom  ' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Rejeitar/i }));
    expect(onAction).toHaveBeenCalledWith('reject', { reject_reason: 'Quero outro tom' });
  });

  it('não envia extras ao aplicar mesmo com motivo preenchido', () => {
    const onAction = vi.fn();
    render(
      <DecisionDialog
        isOpen
        title="Editar"
        description="Aplicar?"
        rejectReason={{ id: 'reject_reason', label: 'Motivo' }}
        actions={[
          { id: 'apply', label: 'Aplicar', primary: true },
          { id: 'reject', label: 'Rejeitar', variant: 'outline' },
        ]}
        onAction={onAction}
        onCancel={vi.fn()}
      />,
    );

    fireEvent.change(screen.getByLabelText('Motivo'), {
      target: { value: 'ignorar isto no apply' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Aplicar/i }));
    expect(onAction).toHaveBeenCalledWith('apply');
    expect(onAction.mock.calls[0]).toHaveLength(1);
  });

  it('passa motivo no onCancel quando ESC com texto', () => {
    const onCancel = vi.fn();
    render(
      <DecisionDialog
        isOpen
        title="Editar"
        description="Aplicar?"
        rejectReason={{ id: 'reject_reason', label: 'Motivo' }}
        actions={[
          { id: 'apply', label: 'Aplicar', primary: true },
          { id: 'reject', label: 'Rejeitar', variant: 'outline' },
        ]}
        onAction={vi.fn()}
        onCancel={onCancel}
      />,
    );

    fireEvent.change(screen.getByLabelText('Motivo'), {
      target: { value: 'via ESC' },
    });
    fireEvent.keyDown(screen.getByRole('alertdialog'), { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledWith({ reject_reason: 'via ESC' });
  });

  it('Ctrl+Shift+R repete o anúncio', async () => {
    render(
      <DecisionDialog
        isOpen
        title="Título"
        description="Pergunta"
        body={<code>ls -la</code>}
        actions={[
          { id: 'ok', label: 'OK', primary: true },
          { id: 'cancel', label: 'Cancelar', variant: 'outline' },
        ]}
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    await waitFor(() => expect(announceRequest).toHaveBeenCalled());
    expect(announceRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('conteúdo adicional'),
      }),
    );
    announceRequest.mockClear();

    fireEvent.keyDown(document, { key: 'R', ctrlKey: true, shiftKey: true });
    expect(announceRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        message: expect.stringContaining('conteúdo adicional'),
        announcePriority: 'assertive',
      }),
    );
  });

  it('Ctrl+Shift+R nao intercepta em campo editavel', async () => {
    render(
      <DecisionDialog
        isOpen
        title="Título"
        description="Pergunta"
        body={<textarea aria-label="motivo" defaultValue="" />}
        actions={[
          { id: 'ok', label: 'OK', primary: true },
          { id: 'cancel', label: 'Cancelar', variant: 'outline' },
        ]}
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    await waitFor(() => expect(announceRequest).toHaveBeenCalled());
    announceRequest.mockClear();

    const field = screen.getByLabelText('motivo');
    field.focus();
    fireEvent.keyDown(field, { key: 'R', ctrlKey: true, shiftKey: true });
    expect(announceRequest).not.toHaveBeenCalled();
  });

  it('não tem violações axe', async () => {
    render(
      <DecisionDialog
        isOpen
        title="Apagar"
        description="Tem certeza que deseja apagar?"
        actions={[
          { id: 'confirm', label: 'Apagar', primary: true, variant: 'danger' },
          { id: 'cancel', label: 'Cancelar', variant: 'outline' },
        ]}
        severity="destructive"
        onAction={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    const dialog = screen.getByRole('alertdialog');
    expect(await axe(dialog)).toHaveNoViolations();
  });
});
