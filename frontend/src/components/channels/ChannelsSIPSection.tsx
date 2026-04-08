import { useTranslation } from 'react-i18next';
import { Input, Checkbox, Button } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';

interface SIPForm {
  enabled: boolean;
  sipServer: string;
  sipPort: number;
  sipUser: string;
  sipPassword: string;
  sipDisplayName: string;
  sipTransport: string;
  sipLocalIP: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface ChannelsSIPSectionProps {
  form: SIPForm;
  onChange: (form: SIPForm) => void;
  onAnnounce: (message: string) => void;
  vaultEnabled: boolean;
  onToggleVault: (value: boolean) => void;
  credentialStored: boolean;
  credentialMasked: string;
  onRemoveCredential: () => void;
}

export function ChannelsSIPSection({
  form,
  onChange,
  onAnnounce,
  vaultEnabled,
  onToggleVault,
  credentialStored,
  credentialMasked,
  onRemoveCredential,
}: ChannelsSIPSectionProps) {
  const { t } = useTranslation();

  return (
    <>
      <Checkbox
        label={t('channels.sip.enabled')}
        checked={form.enabled}
        onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
      />
      {form.enabled && (
        <>
          <Input
            label={t('channels.sip.server')}
            type="text"
            value={form.sipServer}
            onChange={(e) => onChange({ ...form, sipServer: e.target.value })}
            placeholder={t('channels.sip.serverPlaceholder')}
            fullWidth
          />
          <Input
            label={t('channels.sip.port')}
            type="number"
            min="1"
            max="65535"
            value={form.sipPort}
            onChange={(e) =>
              onChange({ ...form, sipPort: parseInt(e.target.value) || 5060 })
            }
            fullWidth
          />
          <Input
            label={t('channels.sip.user')}
            type="text"
            value={form.sipUser}
            onChange={(e) => onChange({ ...form, sipUser: e.target.value })}
            placeholder={t('channels.sip.userPlaceholder')}
            fullWidth
          />
          <Input
            label={t('channels.sip.password')}
            type="password"
            value={form.sipPassword}
            onChange={(e) => onChange({ ...form, sipPassword: e.target.value })}
            fullWidth
          />
          <Checkbox
            label={t('channels.sip.saveVault')}
            checked={vaultEnabled}
            onChange={(e) => onToggleVault(e.target.checked)}
          />
          <p className="channels-page__hint">
            {t('channels.sip.vaultHint')}
          </p>
          {credentialStored && (
            <div className="channels-page__vault-actions">
              <span className="channels-page__hint">
                {t('channels.sip.passwordStored')} {credentialMasked ? `(${credentialMasked})` : ''}.
              </span>
              <Button variant="ghost" size="sm" onClick={onRemoveCredential}>
                {t('channels.sip.removeVault')}
              </Button>
            </div>
          )}
          <Input
            label={t('channels.sip.displayName')}
            type="text"
            value={form.sipDisplayName}
            onChange={(e) => onChange({ ...form, sipDisplayName: e.target.value })}
            placeholder={t('channels.sip.displayNamePlaceholder')}
            fullWidth
          />
          <Input
            label={t('channels.sip.transport')}
            type="text"
            value={form.sipTransport}
            onChange={(e) => onChange({ ...form, sipTransport: e.target.value })}
            placeholder="udp"
            fullWidth
          />
          <p className="channels-page__hint">
            {t('channels.sip.transportHint')}
          </p>
          <Input
            label={t('channels.sip.localIP')}
            type="text"
            value={form.sipLocalIP}
            onChange={(e) => onChange({ ...form, sipLocalIP: e.target.value })}
            placeholder={t('channels.sip.localIPPlaceholder')}
            fullWidth
          />
          <p className="channels-page__hint">
            {t('channels.sip.localIPHint')}
          </p>
          <Input
            label={t('channels.sip.maxContacts')}
            type="number"
            min="0"
            max="100"
            value={form.maxContacts}
            onChange={(e) =>
              onChange({ ...form, maxContacts: parseInt(e.target.value) || 0 })
            }
            fullWidth
          />
          <p className="channels-page__hint">
            {t('channels.sip.maxContactsHint')}
          </p>
          <ProfilePicker
            value={form.profile}
            onChange={(slug) => onChange({ ...form, profile: slug })}
            label={t('channels.sip.channelProfile')}
            maxWidth="100%"
            onAnnounce={onAnnounce}
          />
          <p className="channels-page__hint">
            {t('channels.sip.channelProfileHint')}
          </p>
          <Input
            label={t('channels.sip.maxHistory')}
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
