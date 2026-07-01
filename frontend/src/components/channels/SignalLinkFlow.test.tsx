import { beforeEach, describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SignalLinkFlow } from './SignalLinkFlow';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

describe('SignalLinkFlow', () => {
  const mockOnLink = vi.fn();
  const mockOnReset = vi.fn();

  const defaultProps = {
    apiURL: 'http://localhost:8080',
    linkQR: '',
    linking: false,
    onLink: mockOnLink,
    onReset: mockOnReset,
  };

  beforeEach(() => {
    announceMock.mockReset();
    mockOnLink.mockReset();
    mockOnReset.mockReset();
  });

  it('mostra botão de gerar QR code', () => {
    render(<SignalLinkFlow {...defaultProps} />);

    expect(screen.getByText('channels.signalLink.generateQr')).toBeInTheDocument();
  });

  it('desabilita botão quando não há API URL', () => {
    render(<SignalLinkFlow {...defaultProps} apiURL="" />);

    const button = screen.getByText('channels.signalLink.generateQr');
    expect(button).toBeDisabled();
  });

  it('mostra estado de loading quando linking é true', () => {
    const { container } = render(
      <SignalLinkFlow {...defaultProps} linking={true} />
    );

    const button = container.querySelector('button');
    expect(button).toBeDisabled();
  });

  it('mostra mensagem de gerando quando linking mas sem QR', () => {
    render(<SignalLinkFlow {...defaultProps} linking={true} />);

    expect(screen.getByText('channels.signalLink.generating')).toBeInTheDocument();
  });

  it('reanuncia progresso ao reiniciar vinculação', () => {
    const { rerender } = render(<SignalLinkFlow {...defaultProps} linking={true} />);

    expect(announceMock).toHaveBeenCalledWith('channels.signalLink.generating');

    rerender(<SignalLinkFlow {...defaultProps} linking={false} />);
    rerender(<SignalLinkFlow {...defaultProps} linking={true} />);

    expect(announceMock).toHaveBeenCalledTimes(2);
  });

  it('mostra QR code quando linkQR está presente', () => {
    render(
      <SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />
    );

    const img = screen.getByAltText('channels.signalLink.qrAlt');
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute('src', 'data:image/png;base64,...');
  });

  it('mostra mensagem de aguardando vinculação quando linking', () => {
    render(
      <SignalLinkFlow
        {...defaultProps}
        linkQR="data:image/png;base64,..."
        linking={true}
      />
    );

    expect(screen.getByText('channels.signalLink.waiting')).toBeInTheDocument();
  });

  it('mostra botão de cancelar quando há QR ou linking', () => {
    render(<SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />);

    expect(screen.getByText('common.cancel')).toBeInTheDocument();
  });

  it('chama onReset ao clicar em Cancelar', async () => {
    const user = userEvent.setup();
    render(<SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />);

    await user.click(screen.getByText('common.cancel'));
    expect(mockOnReset).toHaveBeenCalled();
  });
});
