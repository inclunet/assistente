import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation();
  return (
    <>
      <div aria-live="assertive" aria-atomic="true">
        {regError && (
          <div className="channels-page__alert" role="alert">
            <strong>{t('channels.signalRegistration.error')}</strong> {regError}
          </div>
        )}
      </div>

      {regStep === 'idle' && (
        <div className="channels-page__fields">
          <Input
            label={t('channels.signalRegistration.verificationToken')}
            value={regCaptcha}
            onChange={(e) => onSetRegCaptcha(e.target.value)}
            placeholder={t('channels.signalRegistration.captchaPlaceholder')}
            fullWidth
          />
          <p className="channels-page__hint">
            Abra{' '}
            <a
              href="https://signalcaptchas.org/registration/generate.html"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('channels.signalRegistration.captchaLink')}
            </a>
            {t('channels.signalRegistration.captchaHint')}
          </p>
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={() => onRegister('sms')}
              disabled={!apiURL || !account || !regCaptcha}
            >
              {t('channels.signalRegistration.sendSMS')}
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
          {t('channels.signalRegistration.sending')}
        </p>
      )}

      {(regStep === 'awaiting_code' || regStep === 'verifying') && (
        <div className="channels-page__fields">
          <Input
            label={t('channels.signalRegistration.verificationCode')}
            value={regCode}
            onChange={(e) => onSetRegCode(e.target.value)}
            placeholder={t('channels.signalRegistration.codePlaceholder')}
            fullWidth
          />
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={onVerify}
              loading={regStep === 'verifying'}
              disabled={!regCode}
            >
              {t('channels.signalRegistration.verify')}
            </Button>
            <Button
              variant="outline"
              onClick={() => onRegister('voice')}
              disabled={!smsSent}
            >
              {t('channels.signalRegistration.resendCall')}
            </Button>
            <Button variant="ghost" onClick={onReset}>
              {t('common.cancel')}
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
            {account} {t('channels.signalRegistration.registeredSuccess')}
          </div>
          <Button variant="ghost" onClick={onReset}>
            {t('channels.signalRegistration.ok')}
          </Button>
        </div>
      )}
    </>
  );
}
