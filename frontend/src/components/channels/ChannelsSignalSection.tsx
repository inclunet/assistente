import { useTranslation } from 'react-i18next';
import { Input, Button } from '../index';
import {
  ChannelEnabledFields,
  ChannelLimitsProfileFields,
  ChannelVaultFields,
} from './ChannelCommonFields';
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
    <ChannelEnabledFields
      form={form}
      onChange={onChange}
      enabledLabel={t('channels.signal.enabled')}
    >
      <Input
        label={t('channels.signal.apiUrl')}
        value={form.apiURL}
        onChange={(e) => {
          onChange({ ...form, apiURL: e.target.value });
          onSetApiReady(false);
          onSetApiInfo('');
          onSetAccounts([]);
          resetRegistration();
          resetLink();
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
      <ChannelVaultFields
        label={t('channels.signal.saveVault')}
        checked={vaultEnabled}
        onToggle={onToggleVault}
        hint={t('channels.signal.vaultHint')}
        credentials={[
          {
            id: 'signal-api-token',
            stored: tokenStored,
            masked: tokenMasked,
            storedLabel: t('channels.signal.tokenStored'),
            removeLabel: t('channels.signal.removeVault'),
            onRemove: onRemoveToken,
          },
        ]}
      />
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
          <h4>{t('channels.signal.connectAccount')}</h4>
          <p className="channels-page__hint">{t('channels.signal.connectHint')}</p>

          <Input
            label={t('channels.signal.phoneNumber')}
            value={form.account}
            onChange={(e) => onChange({ ...form, account: e.target.value })}
            placeholder={t('channels.signal.phonePlaceholder')}
            fullWidth
          />

          <div className="channels-page__row">
            <Button
              variant={connectionMode === 'register' ? 'primary' : 'outline'}
              onClick={() => {
                onSetConnectionMode('register');
                resetRegistration();
                resetLink();
              }}
            >
              {t('channels.signal.registerNumber')}
            </Button>
            <Button
              variant={connectionMode === 'link' ? 'primary' : 'outline'}
              onClick={() => {
                onSetConnectionMode('link');
                resetRegistration();
              }}
            >
              {t('channels.signal.connectExisting')}
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

      <ChannelLimitsProfileFields
        form={form}
        onChange={onChange}
        onAnnounce={onAnnounce}
        labels={{
          maxContacts: t('channels.signal.maxContacts'),
          maxContactsHint: t('channels.signal.maxContactsHint'),
          channelProfile: t('channels.signal.channelProfile'),
          channelProfileHint: t('channels.signal.channelProfileHint'),
          maxHistory: t('channels.signal.maxHistory'),
        }}
      />
    </ChannelEnabledFields>
  );
}
