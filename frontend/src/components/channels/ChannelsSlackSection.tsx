import { useTranslation } from 'react-i18next';
import { Input } from '../index';
import {
  ChannelEnabledFields,
  ChannelLimitsProfileFields,
  ChannelStoredCredentialActions,
  ChannelVaultFields,
} from './ChannelCommonFields';
import type { SlackForm } from './channelTypes';

interface ChannelsSlackSectionProps {
  form: SlackForm;
  onChange: (form: SlackForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  botTokenStored: boolean;
  botTokenMasked: string;
  appTokenStored: boolean;
  appTokenMasked: string;
  onRemoveBotToken: () => void;
  onRemoveAppToken: () => void;
}

export function ChannelsSlackSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  botTokenStored,
  botTokenMasked,
  appTokenStored,
  appTokenMasked,
  onRemoveBotToken,
  onRemoveAppToken,
}: ChannelsSlackSectionProps) {
  const { t } = useTranslation();

  return (
    <ChannelEnabledFields
      form={form}
      onChange={onChange}
      enabledLabel={t('channels.slack.enabled')}
    >
      <Input
        label={t('channels.slack.botToken')}
        type="password"
        value={form.botToken}
        onChange={(e) => onChange({ ...form, botToken: e.target.value })}
        placeholder={t('channels.slack.botTokenPlaceholder')}
        fullWidth
      />
      <ChannelVaultFields
        label={t('channels.slack.saveVault')}
        checked={vaultEnabled}
        onToggle={onToggleVault}
        hint={t('channels.slack.vaultHint')}
        credentials={[
          {
            id: 'slack-bot-token',
            stored: botTokenStored,
            masked: botTokenMasked,
            storedLabel: t('channels.slack.botTokenStored'),
            removeLabel: t('channels.slack.removeVault'),
            onRemove: onRemoveBotToken,
          },
        ]}
      />
      <p className="channels-page__hint">{t('channels.slack.botTokenHint')}</p>
      <Input
        label={t('channels.slack.appToken')}
        type="password"
        value={form.appToken}
        onChange={(e) => onChange({ ...form, appToken: e.target.value })}
        placeholder={t('channels.slack.appTokenPlaceholder')}
        fullWidth
      />
      <ChannelStoredCredentialActions
        credentials={[
          {
            id: 'slack-app-token',
            stored: appTokenStored,
            masked: appTokenMasked,
            storedLabel: t('channels.slack.appTokenStored'),
            removeLabel: t('channels.slack.removeVault'),
            onRemove: onRemoveAppToken,
          },
        ]}
      />
      <p className="channels-page__hint">{t('channels.slack.appTokenHint')}</p>
      <ChannelLimitsProfileFields
        form={form}
        onChange={onChange}
        onAnnounce={onAnnounce}
        labels={{
          maxContacts: t('channels.slack.maxContacts'),
          maxContactsHint: t('channels.slack.maxContactsHint'),
          channelProfile: t('channels.slack.channelProfile'),
          channelProfileHint: t('channels.slack.channelProfileHint'),
          maxHistory: t('channels.slack.maxHistory'),
        }}
      />
    </ChannelEnabledFields>
  );
}
