import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { axe } from '../../test/a11yAxe';
import { describe, expect, it, vi } from 'vitest';

import { EditorExternalChangeDialog } from './EditorExternalChangeDialog';

const announceRequest = vi.fn();
const playSound = vi.fn();
vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
  useAnnouncer: () => ({ announceRequest }),
}));
vi.mock('../../services/audioFeedback', () => ({
  SOUND_TYPES: { ALERT: 'alert' },
  playSound: (...args: unknown[]) => playSound(...args),
}));
vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: unknown) => unknown) =>
    selector({ config: { decisionAlertSound: true } }),
}));
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const decision = {
  id: 'decision-1',
  title: 'Arquivo modificado',
  description: 'Escolha como resolver.',
  filePath: 'C:/tmp/doc.md',
  diffPreview: '- antigo\n+ novo',
  diskPreview: 'novo',
  localPreview: 'local',
  diskReadFailed: false,
  labels: {
    file: 'Arquivo',
    diff: 'Diff',
    disk: 'Disco',
    local: 'Local',
    useDisk: 'Usar disco',
    resolveMerge: 'Resolver conflitos',
    useMine: 'Usar minha versão',
    saveAs: 'Salvar como',
    notNow: 'Agora não',
  },
} as const;

describe('EditorExternalChangeDialog', () => {
  it('usa alertdialog, botões diretos e começa pela primeira ilha', async () => {
    const onAction = vi.fn();
    const visibleRects = Object.assign([{} as DOMRect], {
      item: (index: number) => (index === 0 ? ({} as DOMRect) : null),
    }) as DOMRectList;
    const rects = vi
      .spyOn(HTMLElement.prototype, 'getClientRects')
      .mockReturnValue(visibleRects);
    render(<EditorExternalChangeDialog decision={decision} onAction={onAction} />);

    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    expect(screen.queryByRole('radio')).not.toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Arquivo' })).toHaveTextContent('C:/tmp/doc.md');
    expect(screen.getByRole('group', { name: 'Diff' }).textContent).toBe('- antigo\n+ novo');
    expect(screen.getByRole('group', { name: 'Disco' })).toHaveTextContent('novo');
    expect(screen.getByRole('group', { name: 'Local' })).toHaveTextContent('local');
    expect(screen.queryByRole('document')).toBeNull();
    const footer = screen.getByRole('alertdialog').querySelector('[data-dialog-actions]');
    expect(
      Array.from(footer?.querySelectorAll('button') ?? []).map(
        (button) => button.getAttribute('aria-label'),
      ),
    ).toEqual([
      'Usar disco',
      'Resolver conflitos',
      'Usar minha versão',
      'Salvar como',
      'Agora não',
    ]);
    await waitFor(() =>
      expect(screen.getByRole('document', { name: 'Arquivo' })).toHaveFocus(),
    );
    expect(screen.getByRole('button', { name: 'Usar disco' })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Enter'),
    );
    expect(screen.getByRole('button', { name: 'Agora não' })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Backspace'),
    );
    expect(screen.getByRole('button', { name: 'Resolver conflitos' }))
      .toHaveAttribute('aria-keyshortcuts', expect.stringMatching(/Alt\+[A-Z]/));
    rects.mockRestore();
  });

  it('anuncia novamente com Ctrl+Shift+R e não tem violações axe', async () => {
    const { container } = render(
      <EditorExternalChangeDialog decision={decision} onAction={vi.fn()} />,
    );
    await waitFor(() => expect(announceRequest).toHaveBeenCalled());
    announceRequest.mockClear();

    fireEvent.keyDown(document, { key: 'R', ctrlKey: true, shiftKey: true });

    expect(announceRequest).toHaveBeenCalledWith(
      expect.objectContaining({ announcePriority: 'assertive' }),
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('anuncia e toca alerta novamente ao avançar a fila com o mesmo texto', async () => {
    const { rerender } = render(
      <EditorExternalChangeDialog decision={decision} onAction={vi.fn()} />,
    );
    await waitFor(() => expect(announceRequest).toHaveBeenCalledTimes(1));
    announceRequest.mockClear();
    playSound.mockClear();

    rerender(
      <EditorExternalChangeDialog
        decision={{ ...decision, id: 'decision-2' }}
        onAction={vi.fn()}
      />,
    );

    await waitFor(() => expect(announceRequest).toHaveBeenCalledTimes(1));
    expect(playSound).toHaveBeenCalledWith('alert');
  });
});
