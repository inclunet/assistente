import type { ReactNode } from 'react';
import { Button, Checkbox, Input } from '../index';
import { ProfilePicker } from '../pickers/ProfilePicker';

export interface ChannelCommonForm {
  enabled: boolean;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface ChannelEnabledFieldsProps<TForm extends { enabled: boolean }> {
  form: TForm;
  onChange: (form: TForm) => void;
  enabledLabel: string;
  children: ReactNode;
}

interface ChannelVaultCredential {
  id: string;
  stored: boolean;
  masked: string;
  storedLabel: string;
  removeLabel: string;
  onRemove: () => void;
}

interface ChannelVaultFieldsProps {
  label: string;
  checked: boolean;
  onToggle: (value: boolean) => void;
  hint: string;
  credentials: ChannelVaultCredential[];
}

interface ChannelLimitsProfileLabels {
  maxContacts: string;
  maxContactsHint: string;
  channelProfile: string;
  channelProfileHint: string;
  maxHistory: string;
}

interface ChannelLimitsProfileFieldsProps<TForm extends ChannelCommonForm> {
  form: TForm;
  onChange: (form: TForm) => void;
  onAnnounce: (message: string) => void;
  labels: ChannelLimitsProfileLabels;
}

export function ChannelEnabledFields<TForm extends { enabled: boolean }>({
  form,
  onChange,
  enabledLabel,
  children,
}: ChannelEnabledFieldsProps<TForm>) {
  return (
    <>
      <Checkbox
        label={enabledLabel}
        checked={form.enabled}
        onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
      />
      {form.enabled && children}
    </>
  );
}

export function ChannelVaultFields({
  label,
  checked,
  onToggle,
  hint,
  credentials,
}: ChannelVaultFieldsProps) {
  return (
    <>
      <Checkbox label={label} checked={checked} onChange={(e) => onToggle(e.target.checked)} />
      <p className="channels-page__hint">{hint}</p>
      <ChannelStoredCredentialActions credentials={credentials} />
    </>
  );
}

export function ChannelStoredCredentialActions({
  credentials,
}: {
  credentials: ChannelVaultCredential[];
}) {
  return (
    <>
      {credentials.map((credential) =>
        credential.stored ? (
          <div className="channels-page__vault-actions" key={credential.id}>
            <span className="channels-page__hint">
              {credential.masked
                ? `${credential.storedLabel} (${credential.masked}).`
                : `${credential.storedLabel}.`}
            </span>
            <Button variant="ghost" size="sm" onClick={credential.onRemove}>
              {credential.removeLabel}
            </Button>
          </div>
        ) : null
      )}
    </>
  );
}

export function ChannelLimitsProfileFields<TForm extends ChannelCommonForm>({
  form,
  onChange,
  onAnnounce,
  labels,
}: ChannelLimitsProfileFieldsProps<TForm>) {
  return (
    <>
      <Input
        label={labels.maxContacts}
        type="number"
        min="-1"
        max="100"
        value={String(form.maxContacts)}
        onChange={(e) => {
          const parsed = parseInt(e.target.value, 10);
          onChange({ ...form, maxContacts: Number.isNaN(parsed) ? 1 : parsed });
        }}
        fullWidth
      />
      <p className="channels-page__hint">{labels.maxContactsHint}</p>
      <ProfilePicker
        value={form.profile}
        onChange={(slug) => onChange({ ...form, profile: slug })}
        label={labels.channelProfile}
        maxWidth="100%"
        onAnnounce={onAnnounce}
      />
      <p className="channels-page__hint">{labels.channelProfileHint}</p>
      <Input
        label={labels.maxHistory}
        type="number"
        min="1"
        max="200"
        value={form.maxHistory}
        onChange={(e) => onChange({ ...form, maxHistory: parseInt(e.target.value, 10) || 50 })}
        fullWidth
      />
    </>
  );
}
