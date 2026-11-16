import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetLogSettings, SaveLogSettings } from '@wailsjs/go/wailsapi/Settings';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { Input } from '../components/ui/Input';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import './LogSettingsPage.css';

interface LogSettings {
  enabled: boolean;
  path: string;
  maxMiB: number;
  backups: number;
  rotateHours: number;
}

export default function LogSettingsPage() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [settings, setSettings] = useState<LogSettings | null>(null);
  const [overridden, setOverridden] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  useContentPageLandmarks({ pageClass: 'log-settings-page' });

  useEffect(() => {
    let active = true;
    void GetLogSettings().then((result) => {
      if (active) {
        setSettings(result.settings);
        setOverridden(result.overridden);
        if (result.error) setError(t('logSettings.startupError', { detail: result.error }));
      }
    }).catch(() => { if (active) setError(t('logSettings.loadError')); });
    return () => { active = false; };
  }, [t]);

  const save = async () => {
    if (!settings) return;
    setSaving(true);
    setError('');
    try {
      await SaveLogSettings(settings);
      announce(t('logSettings.saved'));
    } catch (cause) {
      const detail = cause instanceof Error ? cause.message : String(cause);
      const message = t('logSettings.saveError', { detail });
      setError(message);
      announce(message, 'assertive');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="log-settings-page">
      <h1>{t('logSettings.title')}</h1>
      <p>{t('logSettings.description')}</p>
      {overridden && <p>{t('logSettings.overridden')}</p>}
      {error && <p role="alert">{error}</p>}
      {settings && <form onSubmit={(event) => { event.preventDefault(); void save(); }}>
        <Checkbox label={t('logSettings.enabled')} checked={settings.enabled}
          onChange={(event) => setSettings({ ...settings, enabled: event.target.checked })} />
        <Input label={t('logSettings.path')} value={settings.path} disabled={!settings.enabled}
          required={settings.enabled} onChange={(event) => setSettings({ ...settings, path: event.target.value })} />
        <Input label={t('logSettings.maxMiB')} type="number" min={1} max={10240}
          value={settings.maxMiB} onChange={(event) => setSettings({ ...settings, maxMiB: Number(event.target.value) })} />
        <Input label={t('logSettings.backups')} type="number" min={1} max={100}
          value={settings.backups} onChange={(event) => setSettings({ ...settings, backups: Number(event.target.value) })} />
        <Input label={t('logSettings.rotateHours')} type="number" min={0} max={8760}
          hint={t('logSettings.rotateHoursHint')} value={settings.rotateHours}
          onChange={(event) => setSettings({ ...settings, rotateHours: Number(event.target.value) })} />
        <Button type="submit" loading={saving}>{t('logSettings.save')}</Button>
      </form>}
    </div>
  );
}
