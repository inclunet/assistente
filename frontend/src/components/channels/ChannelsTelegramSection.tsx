import { useTranslation } from 'react-i18next';
import { Input, Checkbox, Button } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';

interface TelegramForm {
  enabled: boolean;
  botToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface ChannelsTelegramSectionProps {
  form: TelegramForm;
  onChange: (form: TelegramForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  credentialStored: boolean;
  credentialMasked: string;
  onRemoveCredential: () => void;
}

export function ChannelsTelegramSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  credentialStored,
  credentialMasked,
  onRemoveCredential,
}: ChannelsTelegramSectionProps) {
  const { t } = useTranslation();
  return (
    <>
      <Checkbox
        label={t('channels.telegram.enabled')}
        checked={form.enabled}
        onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
      />
      {form.enabled && (
        <>
          <Input
            label={t('channels.telegram.botToken')}
            type="password"
            value={form.botToken}
            onChange={(e) => onChange({ ...form, botToken: e.target.value })}
            placeholder={t('channels.telegram.botTokenPlaceholder')}
            fullWidth
          />
          <Checkbox
            label={t('channels.telegram.saveVault')}
            checked={vaultEnabled}
            onChange={(e) => onToggleVault(e.target.checked)}
          />
          <p className="channels-page__hint">
            {t('channels.telegram.vaultHint')}
          </p>
          {credentialStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                {t('channels.telegram.tokenStored')} {credentialMasked ? `(${credentialMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveCredential}>
                {t('channels.telegram.removeVault')}
              </Button>
            </div>
          )}
          <p className="channels-page__hint">
            {t('channels.telegram.botFatherHint')}
          </p>
          <Input
            label={t('channels.telegram.maxContacts')}
            type="number"
            min="1"
            max="100"
            value={form.maxContacts}
            onChange={(e) =>
              onChange({ ...form, maxContacts: parseInt(e.target.value) || 1 })
            }
            fullWidth
          />
          <p className="channels-page__hint">
            {t('channels.telegram.maxContactsHint')}
          </p>
          <ProfilePicker
            value={form.profile}
            onChange={(slug) => onChange({ ...form, profile: slug })}
            label={t('channels.telegram.channelProfile')}
            maxWidth="100%"
            onAnnounce={onAnnounce}
          />
          <p className="channels-page__hint">
            {t('channels.telegram.channelProfileHint')}
          </p>
          <Input
            label={t('channels.telegram.maxHistory')}
            type="number"
            min="1"
            max="200"
            value={form.maxHistory}
            onChange={(e) =>
              onChange({ ...form, maxHistory: parseInt(e.target.value) || 50 })
            }
            fullWidth
          />
        </>
      )}
    </>
  );
}
