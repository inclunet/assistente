import { useTranslation } from 'react-i18next';
import { Input, Button, Checkbox } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';
import { SignalRegistrationFlow } from './SignalRegistrationFlow';
import { SignalLinkFlow } from './SignalLinkFlow';
import { SignalAccountManagement } from './SignalAccountManagement';

interface SignalForm {
  enabled: boolean;
  apiURL: string;
  account: string;
  apiToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

type SignalRegisterStep = 'idle' | 'registering' | 'awaiting_code' | 'verifying' | 'done';

interface ChannelsSignalSectionProps {
  form: SignalForm;
  onChange: (form: SignalForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  tokenStored: boolean;
  tokenMasked: string;
  onRemoveToken: () => void;
  apiReady: boolean;
  apiInfo: string;
  regError: string;
  regStep: SignalRegisterStep;
  regCode: string;
  regCaptcha: string;
  smsSent: boolean;
  accounts: string[];
  connectionMode: 'register' | 'link';
  linkQR: string;
  linking: boolean;
  unregistering: string | null;
  checkingAPI: boolean;
  onSetApiReady: (ready: boolean) => void;
  onSetApiInfo: (info: string) => void;
  onSetRegError: (error: string) => void;
  onSetRegStep: (step: SignalRegisterStep) => void;
  onSetRegCode: (code: string) => void;
  onSetRegCaptcha: (captcha: string) => void;
  onSetSmsSent: (sent: boolean) => void;
  onSetAccounts: (accounts: string[]) => void;
  onSetConnectionMode: (mode: 'register' | 'link') => void;
  onSetLinkQR: (qr: string) => void;
  onSetLinking: (linking: boolean) => void;
  onCheckAPI: () => Promise<void>;
  onRegister: (mode: 'sms' | 'voice') => Promise<void>;
  onVerify: () => Promise<void>;
  onLink: () => Promise<void>;
  onUnregister: (account: string) => Promise<void>;
  onStopLinkPolling: () => void;
}

export function ChannelsSignalSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  tokenStored,
  tokenMasked,
  onRemoveToken,
  apiReady,
  apiInfo,
  regError,
  regStep,
  regCode,
  regCaptcha,
  smsSent,
  accounts,
  connectionMode,
  linkQR,
  linking,
  unregistering,
  checkingAPI,
  onSetApiReady,
  onSetApiInfo,
  onSetRegError,
  onSetRegStep,
  onSetRegCode,
  onSetRegCaptcha,
  onSetSmsSent,
  onSetAccounts,
  onSetConnectionMode,
  onSetLinkQR,
  onSetLinking,
  onCheckAPI,
  onRegister,
  onVerify,
  onLink,
  onUnregister,
  onStopLinkPolling,
}: ChannelsSignalSectionProps) {
  const { t } = useTranslation();
  const resetRegistration = () => {
    onSetRegStep('idle');
    onSetRegCode('');
    onSetRegCaptcha('');
    onSetRegError('');
    onSetSmsSent(false);
  };

  const resetLink = () => {
    onSetLinkQR('');
    onSetLinking(false);
    onStopLinkPolling();
  };

  return (
    <>
      <Checkbox
        label={t('channels.signal.enabled')}
        checked={form.enabled}
        onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
      />
      {form.enabled && (
        <>
          <Input
            label={t('channels.signal.apiUrl')}
            value={form.apiURL}
            onChange={(e) => {
              onChange({ ...form, apiURL: e.target.value });
              onSetApiReady(false);
              onSetApiInfo('');
              onSetRegError('');
              onSetAccounts([]);
            }}
            placeholder={t('channels.signal.apiUrlPlaceholder')}
            fullWidth
          />
          <Input
            label={t('channels.signal.apiToken')}
            type="password"
            value={form.apiToken}
            onChange={(e) => onChange({ ...form, apiToken: e.target.value })}
            placeholder={t('channels.signal.apiTokenPlaceholder')}
            fullWidth
          />
          <Checkbox
            label={t('channels.signal.saveVault')}
            checked={vaultEnabled}
            onChange={(e) => onToggleVault(e.target.checked)}
          />
          <p className="channels-page__hint">
            Use apenas se sua instância exigir autenticação. O token fica criptografado no cofre.
          </p>
          {tokenStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                Token salvo no cofre {tokenMasked ? `(${tokenMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveToken}>
                Remover do cofre
              </Button>
            </div>
          )}
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={onCheckAPI}
              loading={checkingAPI}
              disabled={!form.apiURL}
            >
              {t('channels.signal.testConnection')}
            </Button>
          </div>
          <div>
            {apiInfo && (
              <p className="channels-page__hint">
                {apiInfo}
              </p>
            )}
          </div>

          {!apiReady && (
            <p className="channels-page__hint">
              {t('channels.signal.testHint')}
            </p>
          )}

          <SignalAccountManagement
            accounts={accounts}
            unregistering={unregistering}
            onUnregister={onUnregister}
          />

          {apiReady && accounts.length === 0 && (
            <div className="channels-page__subsection">
              <h4>Conectar Conta</h4>
              <p className="channels-page__hint">
                Cadastre um novo número ou conecte uma conta existente via QR
                Code.
              </p>

              <Input
                label="Novo número de telefone"
                value={form.account}
                onChange={(e) => onChange({ ...form, account: e.target.value })}
                placeholder="+5511999999999"
                fullWidth
              />

              <div className="channels-page__row">
                <Button
                  variant={
                    connectionMode === 'register' ? 'primary' : 'outline'
                  }
                  onClick={() => {
                    onSetConnectionMode('register');
                    resetRegistration();
                    onSetLinkQR('');
                    onSetLinking(false);
                  }}
                >
                  Cadastrar número
                </Button>
                <Button
                  variant={connectionMode === 'link' ? 'primary' : 'outline'}
                  onClick={() => {
                    onSetConnectionMode('link');
                    resetRegistration();
                  }}
                >
                  Conectar conta existente
                </Button>
              </div>

              {connectionMode === 'register' && (
                <SignalRegistrationFlow
                  account={form.account}
                  apiURL={form.apiURL}
                  regStep={regStep}
                  regCode={regCode}
                  regCaptcha={regCaptcha}
                  smsSent={smsSent}
                  regError={regError}
                  onSetRegCode={onSetRegCode}
                  onSetRegCaptcha={onSetRegCaptcha}
                  onRegister={onRegister}
                  onVerify={onVerify}
                  onReset={resetRegistration}
                />
              )}

              {connectionMode === 'link' && (
                <SignalLinkFlow
                  apiURL={form.apiURL}
                  linkQR={linkQR}
                  linking={linking}
                  onLink={onLink}
                  onReset={resetLink}
                />
              )}
            </div>
          )}

          <Input
            label="Max. contatos autorizados"
            type="number"
            value={String(form.maxContacts)}
            onChange={(e) =>
              onChange({
                ...form,
                maxContacts: parseInt(e.target.value) || 1,
              })
            }
            fullWidth
          />
          <p className="channels-page__hint">
            Ao atingir o limite, novos contatos são ignorados silenciosamente.
          </p>
          <ProfilePicker
            value={form.profile}
            onChange={(slug) => onChange({ ...form, profile: slug })}
            label="Perfil do Canal"
            maxWidth="100%"
            onAnnounce={onAnnounce}
          />
          <p className="channels-page__hint">
            Perfil usado para conversas deste canal. Define modelo, voz, STT e
            comportamento. Vazio usa o perfil ativo global.
          </p>
          <Input
            label="Máximo de Histórico"
            type="number"
            min="1"
            max="200"
            value={form.maxHistory}
            onChange={(e) =>
              onChange({
                ...form,
                maxHistory: parseInt(e.target.value) || 50,
              })
            }
            fullWidth
          />
        </>
      )}
    </>
  );
}
