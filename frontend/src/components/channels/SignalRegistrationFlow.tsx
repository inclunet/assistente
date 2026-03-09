import { Input, Button } from '../index';

type SignalRegisterStep = 'idle' | 'registering' | 'awaiting_code' | 'verifying' | 'done';

interface SignalRegistrationFlowProps {
  account: string;
  apiURL: string;
  regStep: SignalRegisterStep;
  regCode: string;
  regCaptcha: string;
  smsSent: boolean;
  regError: string;
  onSetRegCode: (code: string) => void;
  onSetRegCaptcha: (captcha: string) => void;
  onRegister: (mode: 'sms' | 'voice') => Promise<void>;
  onVerify: () => Promise<void>;
  onReset: () => void;
}

export function SignalRegistrationFlow({
  account,
  apiURL,
  regStep,
  regCode,
  regCaptcha,
  smsSent,
  regError,
  onSetRegCode,
  onSetRegCaptcha,
  onRegister,
  onVerify,
  onReset,
}: SignalRegistrationFlowProps) {
  return (
    <>
      <div aria-live="assertive" aria-atomic="true">
        {regError && (
          <div className="channels-page__alert" role="alert">
            <strong>Erro:</strong> {regError}
          </div>
        )}
      </div>

      {regStep === 'idle' && (
        <div className="channels-page__fields">
          <Input
            label="Token de Verificação"
            value={regCaptcha}
            onChange={(e) => onSetRegCaptcha(e.target.value)}
            placeholder="signalcaptcha://signal-hcaptcha.abcdef..."
            fullWidth
          />
          <p className="channels-page__hint">
            Abra{' '}
            <a
              href="https://signalcaptchas.org/registration/generate.html"
              target="_blank"
              rel="noopener noreferrer"
            >
              esta página
            </a>
            , complete o desafio, clique direito em "Open Signal" e copie o
            link.
          </p>
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={() => onRegister('sms')}
              disabled={!apiURL || !account || !regCaptcha}
            >
              Enviar código por SMS
            </Button>
          </div>
        </div>
      )}

      {regStep === 'registering' && (
        <p
          className="channels-page__hint"
          role="status"
          aria-live="polite"
        >
          Enviando código...
        </p>
      )}

      {(regStep === 'awaiting_code' || regStep === 'verifying') && (
        <div className="channels-page__fields">
          <Input
            label="Código de Verificação"
            value={regCode}
            onChange={(e) => onSetRegCode(e.target.value)}
            placeholder="123-456"
            fullWidth
          />
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={onVerify}
              loading={regStep === 'verifying'}
              disabled={!regCode}
            >
              Verificar
            </Button>
            <Button
              variant="outline"
              onClick={() => onRegister('voice')}
              disabled={!smsSent}
            >
              Reenviar por Ligação
            </Button>
            <Button variant="ghost" onClick={onReset}>
              Cancelar
            </Button>
          </div>
        </div>
      )}

      {regStep === 'done' && (
        <div className="channels-page__fields">
          <div
            className="channels-page__success"
            role="status"
            aria-live="polite"
          >
            Número {account} registrado com sucesso!
          </div>
          <Button variant="ghost" onClick={onReset}>
            OK
          </Button>
        </div>
      )}
    </>
  );
}
