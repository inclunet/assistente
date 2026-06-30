import { beforeEach, describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SignalRegistrationFlow } from './SignalRegistrationFlow';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'channels.signalRegistration.error': 'Erro:',
        'channels.signalRegistration.verificationToken': 'Token de Verificação',
        'channels.signalRegistration.captchaPlaceholder':
          'signalcaptcha://signal-hcaptcha.abcdef...',
        'channels.signalRegistration.captchaLink': 'esta página',
        'channels.signalRegistration.captchaHint':
          ', complete o desafio, clique direito em "Open Signal" e copie o link.',
        'channels.signalRegistration.sendSMS': 'Enviar código por SMS',
        'channels.signalRegistration.sending': 'Enviando código...',
        'channels.signalRegistration.verificationCode': 'Código de Verificação',
        'channels.signalRegistration.codePlaceholder': '123-456',
        'channels.signalRegistration.verify': 'Verificar',
        'channels.signalRegistration.resendCall': 'Reenviar por Ligação',
        'channels.signalRegistration.registeredSuccess': 'registrado com sucesso!',
        'channels.signalRegistration.ok': 'OK',
        'common.cancel': 'Cancelar',
      };
      return translations[key] ?? key;
    },
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

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

  beforeEach(() => {
    announceMock.mockReset();
    mockOnSetRegCode.mockReset();
    mockOnSetRegCaptcha.mockReset();
    mockOnRegister.mockReset();
    mockOnVerify.mockReset();
    mockOnReset.mockReset();
  });

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

  it('reanuncia progresso ao reiniciar registro', () => {
    const { rerender } = render(
      <SignalRegistrationFlow {...defaultProps} regStep="registering" />
    );

    expect(announceMock).toHaveBeenCalledWith('Enviando código...');

    rerender(<SignalRegistrationFlow {...defaultProps} regStep="idle" />);
    rerender(<SignalRegistrationFlow {...defaultProps} regStep="registering" />);

    expect(announceMock).toHaveBeenCalledTimes(2);
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
      screen.getByText(/registrado com sucesso!/)
    ).toBeInTheDocument();
  });

  it('mostra erro quando regError está presente', () => {
    render(
      <SignalRegistrationFlow {...defaultProps} regError="Erro ao registrar" />
    );

    expect(screen.getByText(/Erro ao registrar/)).toHaveClass('channels-page__alert');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
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
