import { useEffect, useId, useMemo, useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useAuthStore, type AuthStatus } from '../../store/authStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './AuthGate.css';

interface AuthGateProps {
  children: ReactNode;
}

type AuthStep = 'setup' | 'unlock' | 'createAdmin' | 'signIn';

function deriveStep(status: AuthStatus): AuthStep {
  if (!status.vaultConfigured) return 'setup';
  if (!status.vaultUnlocked) return 'unlock';
  if (!status.hasUsers) return 'createAdmin';
  return 'signIn';
}

export function AuthGate({ children }: AuthGateProps) {
  const { t } = useTranslation();
  const status = useAuthStore((s) => s.status);
  const isLoading = useAuthStore((s) => s.isLoading);
  const error = useAuthStore((s) => s.error);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const loadStatus = useAuthStore((s) => s.loadStatus);
  const setupVault = useAuthStore((s) => s.setupVault);
  const unlockVault = useAuthStore((s) => s.unlockVault);
  const createAdmin = useAuthStore((s) => s.createAdmin);
  const login = useAuthStore((s) => s.login);
  const { announce } = useAnnouncer();

  const [secret, setSecret] = useState('');
  const [confirmSecret, setConfirmSecret] = useState('');
  const [username, setUsername] = useState('');
  const [recoveryKey, setRecoveryKey] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);

  const headingId = useId();
  const descriptionId = useId();
  const errorId = useId();
  const passwordHintId = useId();
  const confirmHintId = useId();

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  // Anuncia erros do servidor para leitores de tela quando aparecerem.
  // Em vez de role="alert" agressivo no markup (M30 do review), usamos a
  // live region global do ScreenReaderAnnouncer; usuário com leitor de
  // tela ouve a mensagem sem perder o contexto do formulário.
  useEffect(() => {
    if (error) {
      announce(t(error.i18nKey, error.detail ?? ''), 'assertive');
    }
  }, [error, announce, t]);

  useEffect(() => {
    if (recoveryKey) {
      announce(t('auth.a11y.recoveryAnnouncement'), 'assertive');
    }
  }, [recoveryKey, announce, t]);

  useEffect(() => {
    if (isAuthenticated) {
      announce(t('auth.a11y.loginSuccess'), 'polite');
    }
  }, [isAuthenticated, announce, t]);

  const localizedError = useMemo(() => {
    if (validationError) return validationError;
    if (!error) return null;
    return t(error.i18nKey, t('auth.errors.unknown'));
  }, [error, validationError, t]);

  if (isAuthenticated) {
    return <>{children}</>;
  }

  if (!status && error) {
    return (
      <main className="auth-gate__shell" aria-busy={isLoading}>
        <section
          className="auth-gate__card"
          aria-labelledby={headingId}
          aria-describedby={descriptionId}
        >
          <h1 id={headingId} className="auth-gate__title">
            {t('auth.titles.unavailable')}
          </h1>
          <p id={descriptionId} className="auth-gate__description">
            {t('auth.descriptions.unavailable')}
          </p>
          {localizedError && (
            <p id={errorId} className="auth-gate__error" role="alert">
              {localizedError}
            </p>
          )}
          <button
            type="button"
            disabled={isLoading}
            className="auth-gate__button"
            onClick={() => void loadStatus()}
          >
            {isLoading ? t('auth.buttons.loading') : t('auth.buttons.retry')}
          </button>
        </section>
      </main>
    );
  }

  if (!status) {
    return (
      <main className="auth-gate__shell" aria-busy="true">
        <section
          className="auth-gate__card"
          aria-labelledby={headingId}
          aria-describedby={descriptionId}
        >
          <h1 id={headingId} className="auth-gate__title">
            {t('auth.titles.loading')}
          </h1>
          <p id={descriptionId} className="auth-gate__description">
            {t('auth.descriptions.loading')}
          </p>
        </section>
      </main>
    );
  }

  const step = deriveStep(status);
  const title = t(`auth.titles.${step === 'signIn' ? 'signIn' : step}`);
  const description = t(`auth.descriptions.${step === 'signIn' ? 'signIn' : step}`);
  const passwordLabel =
    step === 'setup' || step === 'unlock'
      ? t('auth.labels.masterPassword')
      : step === 'createAdmin'
        ? t('auth.labels.adminPassword')
        : t('auth.labels.password');
  const passwordHint =
    step === 'setup' || step === 'unlock'
      ? t('auth.hints.masterPassword')
      : t('auth.hints.passwordMin');
  const requiresUsername = step === 'createAdmin' || step === 'signIn';
  const requiresConfirmation = step === 'setup' || step === 'createAdmin';

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setValidationError(null);

    if (requiresConfirmation && secret !== confirmSecret) {
      const message = t('auth.validation.passwordsDoNotMatch');
      setValidationError(message);
      announce(message, 'assertive');
      return;
    }

    try {
      if (step === 'setup') {
        const recovery = await setupVault(secret);
        setRecoveryKey(recovery);
        setSecret('');
        setConfirmSecret('');
        return;
      }
      if (step === 'unlock') {
        await unlockVault('master', secret);
        setSecret('');
        return;
      }
      if (step === 'createAdmin') {
        await createAdmin(username, secret);
        setSecret('');
        setConfirmSecret('');
        return;
      }
      await login(username, secret);
      setSecret('');
    } catch {
      // O store já registra o erro via `error`. Não precisamos rethrowar.
    }
  };

  return (
    <main
      className="auth-gate__shell"
      aria-busy={isLoading}
      aria-label={isLoading ? t('auth.a11y.formBusy') : undefined}
    >
      <form
        className="auth-gate__card"
        onSubmit={(event) => void submit(event)}
        aria-labelledby={headingId}
        aria-describedby={`${descriptionId} ${localizedError ? errorId : ''}`.trim()}
      >
        <h1 id={headingId} className="auth-gate__title">
          {title}
        </h1>
        <p id={descriptionId} className="auth-gate__description">
          {description}
        </p>

        {recoveryKey && (
          <div className="auth-gate__recovery" role="status">
            <strong>{t('auth.recovery.title')}</strong>
            <code className="auth-gate__code">{recoveryKey}</code>
            <span>{t('auth.recovery.instructions')}</span>
          </div>
        )}

        {requiresUsername && (
          <UsernameField
            value={username}
            onChange={setUsername}
            disabled={isLoading}
            label={t('auth.labels.username')}
          />
        )}

        <PasswordField
          id={`${descriptionId}-password`}
          label={passwordLabel}
          hint={passwordHint}
          hintId={passwordHintId}
          value={secret}
          onChange={setSecret}
          autoComplete={step === 'signIn' ? 'current-password' : 'new-password'}
          disabled={isLoading}
        />

        {requiresConfirmation && (
          <PasswordField
            id={`${descriptionId}-confirm`}
            label={t('auth.labels.confirmPassword')}
            hint={t('auth.hints.confirmPassword')}
            hintId={confirmHintId}
            value={confirmSecret}
            onChange={setConfirmSecret}
            autoComplete="new-password"
            disabled={isLoading}
          />
        )}

        {localizedError && (
          <p id={errorId} className="auth-gate__error" role="alert">
            {localizedError}
          </p>
        )}

        {/* Não desabilitamos o botão por mismatch de senha: o usuário
         * com leitor de tela precisa poder submeter para ouvir a
         * mensagem de validação anunciada. Controle de mismatch fica
         * em `submit()`. */}
        <button type="submit" disabled={isLoading} className="auth-gate__button">
          {isLoading ? t('auth.buttons.loading') : t('auth.buttons.continue')}
        </button>
      </form>
    </main>
  );
}

interface UsernameFieldProps {
  value: string;
  onChange: (next: string) => void;
  disabled: boolean;
  label: string;
}

function UsernameField({ value, onChange, disabled, label }: UsernameFieldProps) {
  const id = useId();
  return (
    <div className="auth-gate__field">
      <label htmlFor={id} className="auth-gate__label">
        {label}
      </label>
      <input
        id={id}
        className="auth-gate__input"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required
        autoComplete="username"
        aria-required="true"
        aria-disabled={disabled || undefined}
        disabled={disabled}
      />
    </div>
  );
}

interface PasswordFieldProps {
  id: string;
  label: string;
  hint: string;
  hintId: string;
  value: string;
  onChange: (next: string) => void;
  autoComplete: 'new-password' | 'current-password';
  disabled: boolean;
}

function PasswordField({
  id,
  label,
  hint,
  hintId,
  value,
  onChange,
  autoComplete,
  disabled,
}: PasswordFieldProps) {
  return (
    <div className="auth-gate__field">
      <label htmlFor={id} className="auth-gate__label">
        {label}
      </label>
      <input
        id={id}
        className="auth-gate__input"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required
        type="password"
        autoComplete={autoComplete}
        aria-required="true"
        aria-describedby={hintId}
        aria-disabled={disabled || undefined}
        disabled={disabled}
      />
      <span id={hintId} className="auth-gate__hint">
        {hint}
      </span>
    </div>
  );
}
