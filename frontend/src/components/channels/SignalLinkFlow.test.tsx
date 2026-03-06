import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SignalLinkFlow } from './SignalLinkFlow';

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

  it('mostra botão de gerar QR code', () => {
    render(<SignalLinkFlow {...defaultProps} />);

    expect(screen.getByText('Gerar QR Code')).toBeInTheDocument();
  });

  it('desabilita botão quando não há API URL', () => {
    render(<SignalLinkFlow {...defaultProps} apiURL="" />);

    const button = screen.getByText('Gerar QR Code');
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

    expect(screen.getByText('Gerando QR Code...')).toBeInTheDocument();
  });

  it('mostra QR code quando linkQR está presente', () => {
    render(
      <SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />
    );

    const img = screen.getByAltText('QR Code para vincular dispositivo Signal');
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

    expect(screen.getByText('Aguardando vinculação...')).toBeInTheDocument();
  });

  it('mostra botão de cancelar quando há QR ou linking', () => {
    render(<SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />);

    expect(screen.getByText('Cancelar')).toBeInTheDocument();
  });

  it('chama onReset ao clicar em Cancelar', async () => {
    const user = userEvent.setup();
    render(<SignalLinkFlow {...defaultProps} linkQR="data:image/png;base64,..." />);

    await user.click(screen.getByText('Cancelar'));
    expect(mockOnReset).toHaveBeenCalled();
  });
});
