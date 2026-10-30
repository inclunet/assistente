import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { llmcatalog } from '@wailsjs/go/models';
import { FormField, Select } from '../';
import { BasePicker } from '../pickers/BasePicker';

interface Props {
  aliases: Record<string, string>;
  remoteModels: string[];
  catalogModels: llmcatalog.Model[];
  onChange: (aliases: Record<string, string>) => void;
}

/** Edits connection metadata; selecting an alias never changes the default model. */
export function ProviderModelAliases({ aliases, remoteModels, catalogModels, onChange }: Props) {
  const { t } = useTranslation();
  const [remoteID, setRemoteID] = useState('');
  const canonicalID = Object.prototype.hasOwnProperty.call(aliases, remoteID) ? aliases[remoteID] : '';
  const missing = canonicalID && !catalogModels.some(model => model.id === canonicalID);
  const items = [...new Set([...Object.keys(aliases), ...remoteModels])].map(id => ({
    value: id,
    label: id,
    sublabel: Object.prototype.hasOwnProperty.call(aliases, id) ? aliases[id] : undefined,
  }));
  return (
    <fieldset>
      <legend>{t('providerForm.aliases.title')}</legend>
      <BasePicker
        label={t('providerForm.aliases.remote')}
        helpText={t('providerForm.aliases.description')}
        items={items}
        selected={remoteID}
        onSelect={setRemoteID}
        allowFreeInput
        showEmptyState={false}
      />
      <FormField label={t('providerForm.aliases.canonical')}>
        <Select
          aria-label={t('providerForm.aliases.canonical')}
          value={canonicalID}
          disabled={!remoteID.trim()}
          fullWidth
          options={[
            { value: '', label: t('providerForm.aliases.none') },
            ...(missing ? [{ value: canonicalID, label: t('providerForm.aliases.missing', { id: canonicalID }) }] : []),
            ...catalogModels.map(model => ({ value: model.id, label: `${model.name} (${model.id})` })),
          ]}
          onChange={event => {
            if (event.target.value) {
              onChange({ ...aliases, [remoteID]: event.target.value });
            } else {
              const next = { ...aliases };
              delete next[remoteID];
              onChange(next);
            }
          }}
        />
      </FormField>
    </fieldset>
  );
}
