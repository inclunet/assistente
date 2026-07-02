import { useTranslation } from 'react-i18next';
import { Input } from '../index';
import {
  ChannelEnabledFields,
  ChannelLimitsProfileFields,
  ChannelVaultFields,
} from './ChannelCommonFields';
import type { TelegramForm } from './channelTypes';

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
    <ChannelEnabledFields
      form={form}
      onChange={onChange}
      enabledLabel={t('channels.telegram.enabled')}
    >
      <Input
        label={t('channels.telegram.botToken')}
        type="password"
        value={form.botToken}
        onChange={(e) => onChange({ ...form, botToken: e.target.value })}
        placeholder={t('channels.telegram.botTokenPlaceholder')}
        fullWidth
      />
      <ChannelVaultFields
        label={t('channels.telegram.saveVault')}
        checked={vaultEnabled}
        onToggle={onToggleVault}
        hint={t('channels.telegram.vaultHint')}
        credentials={[
          {
            id: 'telegram-bot-token',
            stored: credentialStored,
            masked: credentialMasked,
            storedLabel: t('channels.telegram.tokenStored'),
            removeLabel: t('channels.telegram.removeVault'),
            onRemove: onRemoveCredential,
          },
        ]}
      />
      <p className="channels-page__hint">{t('channels.telegram.botFatherHint')}</p>
      <ChannelLimitsProfileFields
        form={form}
        onChange={onChange}
        onAnnounce={onAnnounce}
        labels={{
          maxContacts: t('channels.telegram.maxContacts'),
          maxContactsHint: t('channels.telegram.maxContactsHint'),
          channelProfile: t('channels.telegram.channelProfile'),
          channelProfileHint: t('channels.telegram.channelProfileHint'),
          maxHistory: t('channels.telegram.maxHistory'),
        }}
      />
    </ChannelEnabledFields>
  );
}
