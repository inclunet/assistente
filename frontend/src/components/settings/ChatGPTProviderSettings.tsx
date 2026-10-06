import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetLLMProvider, SetDefaultProvider, UpdateLLMProvider } from '@wailsjs/go/wailsapi/LLMProviders';
import { GetModelsByProvider } from '@wailsjs/go/wailsapi/LLMModels';
import { Input } from '../ui/Input';
import { Select } from '../ui/Select';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';

interface Props {
  providerId: string;
  connected: boolean;
  disabled: boolean;
  onBusyChange: (busy: boolean) => void;
  onChanged: () => void;
}

export function ChatGPTProviderSettings({ providerId, connected, disabled, onBusyChange, onChanged }: Props) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [name, setName] = useState('');
  const [model, setModel] = useState('');
  const [models, setModels] = useState<string[]>([]);
  const [isDefault, setIsDefault] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  const presentation = useRef({ t, announce });
  presentation.current = { t, announce };
  useEffect(() => {
    const current = ++generation.current;
    setLoading(true); setLoadFailed(false); setError('');
    void GetLLMProvider(providerId).then(provider => {
      if (current !== generation.current) return;
      setName(provider.name); setModel(provider.default_model || ''); setIsDefault(!!provider.is_default);
    }).catch(() => {
      if (current !== generation.current) return;
      setLoadFailed(true); setError('chatgpt.preferencesLoadError'); presentation.current.announce(presentation.current.t('chatgpt.preferencesLoadError'), 'assertive');
    }).finally(() => { if (current === generation.current) setLoading(false); });
    return () => { generation.current++; };
  }, [providerId]);
  useEffect(() => {
    let active = true;
    setModels([]);
    if (!connected) { setModelsLoading(false); return; }
    setModelsLoading(true);
    void GetModelsByProvider(providerId).then(values => { if (active) setModels(values || []); })
      .catch(() => { if (active) { setError('chatgpt.modelsLoadError'); presentation.current.announce(presentation.current.t('chatgpt.modelsLoadError'), 'assertive'); } })
      .finally(() => { if (active) setModelsLoading(false); });
    return () => { active = false; };
  }, [providerId, connected]);

  const save = async (makeDefault: boolean) => {
    const current = generation.current;
    setBusy(true); onBusyChange(true); setError('');
    try {
      if (makeDefault) {
        await SetDefaultProvider(providerId);
        if (current !== generation.current) return;
        setIsDefault(true);
      } else {
        await UpdateLLMProvider(providerId, { name: name.trim(), default_model: model || undefined });
        if (current !== generation.current) return;
      }
      announce(t(makeDefault ? 'chatgpt.defaultSaved' : 'chatgpt.preferencesSaved'));
      onChanged();
    } catch {
      if (current === generation.current) { setError('chatgpt.preferencesSaveError'); announce(t('chatgpt.preferencesSaveError'), 'assertive'); }
    } finally {
      if (current === generation.current) { setBusy(false); onBusyChange(false); }
    }
  };
  const unavailable = disabled || busy || loading || loadFailed || !connected;
  const options = [...new Set([...models, ...(model ? [model] : [])])].map(value => ({ value, label: value }));
  return <fieldset disabled={unavailable}>
    <legend>{t('chatgpt.providerSettings')}</legend>
    {loading && <p>{t('common.loading')}</p>}
    {error && <p>{t(error)}</p>}
    <Input label={t('chatgpt.providerName')} value={name} maxLength={100} onChange={event => setName(event.target.value)} />
    <Select label={t('providerForm.defaultModel')} value={model} options={[{ value: '', label: t('chatgpt.chooseModel'), disabled: true }, ...options]}
      disabled={unavailable || !connected || modelsLoading} onChange={event => setModel(event.target.value)} />
    {modelsLoading && <p>{t('chatgpt.loadingModels')}</p>}
    <Button disabled={unavailable || !name.trim()} onClick={() => void save(false)}>{t('chatgpt.savePreferences')}</Button>
    {isDefault ? <p>{t('providers.badge.default')}</p> :
      <Button disabled={unavailable} onClick={() => void save(true)}>{t('providers.actions.setDefault')}</Button>}
  </fieldset>;
}
