import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SignalRegistrationFlow } from './SignalRegistrationFlow';

describe('SignalRegistrationFlow', () => {
  const mockOnSetRegCode = vi.fn();
  const mockOnSetRegCaptcha = vi.fn();
  const mockOnRegister = vi.fn();
  const mockOnVerify = vi.fn();
  const mockOnReset = vi.fn();

  const defaultProps = {
    account: '+5511999999999',
    apiURL: 'http://localhost:8080',
    regStep: 'idle' as const,
    regCode: '',
    regCaptcha: '',
    smsSent: false,
    regError: '',
    onSetRegCode: mockOnSetRegCode,
    onSetRegCaptcha: mockOnSetRegCaptcha,
    onRegister: mockOnRegister,
    onVerify: mockOnVerify,
    onReset: mockOnReset,
  };

  it('mostra campo de captcha no estado idle', () => {
    render(<SignalRegistrationFlow {...defaultProps} />);

    expect(screen.getByText('Token de Verificação')).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText('signalcaptcha://signal-hcaptcha.abcdef...')
    ).toBeInTheDocument();
    expect(screen.getByText('Enviar código por SMS')).toBeInTheDocument();
  });

  it('mostra mensagem de enviando no estado registering', () => {
    render(<SignalRegistrationFlow {...defaultProps} regStep="registering" />);

    expect(screen.getByText('Enviando código...')).toBeInTheDocument();
  });

  it('mostra campo de código no estado awaiting_code', () => {
    render(<SignalRegistrationFlow {...defaultProps} regStep="awaiting_code" />);

    expect(screen.getByText('Código de Verificação')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('123-456')).toBeInTheDocument();
    expect(screen.getByText('Verificar')).toBeInTheDocument();
    expect(screen.getByText('Reenviar por Ligação')).toBeInTheDocument();
  });

  it('mostra mensagem de sucesso no estado done', () => {
    render(<SignalRegistrationFlow {...defaultProps} regStep="done" />);

    expect(
      screen.getByText(/Número .* registrado com sucesso!/)
    ).toBeInTheDocument();
  });

  it('mostra erro quando regError está presente', () => {
    render(
      <SignalRegistrationFlow {...defaultProps} regError="Erro ao registrar" />
    );

    expect(screen.getByRole('alert')).toHaveTextContent('Erro: Erro ao registrar');
  });

  it('desabilita botão SMS quando faltam dados', () => {
    render(<SignalRegistrationFlow {...defaultProps} apiURL="" />);

    const button = screen.getByText('Enviar código por SMS');
    expect(button).toBeDisabled();
  });

  it('desabilita botão de ligação até SMS ser enviado', () => {
    render(
      <SignalRegistrationFlow
        {...defaultProps}
        regStep="awaiting_code"
        smsSent={false}
      />
    );

    const button = screen.getByText('Reenviar por Ligação');
    expect(button).toBeDisabled();
  });

  it('chama onReset ao clicar em Cancelar', async () => {
    const user = userEvent.setup();
    render(<SignalRegistrationFlow {...defaultProps} regStep="awaiting_code" />);

    await user.click(screen.getByText('Cancelar'));
    expect(mockOnReset).toHaveBeenCalled();
  });
});
