import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PreviewBuilder, SaveBuilder } from '@wailsjs/go/wailsapi/Plugins';
import { GetAllTaskLists } from '@wailsjs/go/wailsapi/Tasklist';
import { plugins, type database, type wailsapi } from '@wailsjs/go/models';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { Input } from '../components/ui/Input';

export default function PluginBuilder() {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [lists, setLists] = useState<database.TaskList[]>([]);
  const [name, setName] = useState('');
  const [version, setVersion] = useState('');
  const [selected, setSelected] = useState<Record<string, plugins.BuilderTaskList>>({});
  const [preview, setPreview] = useState<wailsapi.BuilderPreview | null>(null);
  const [status, setStatus] = useState('');
  const [busy, setBusy] = useState(true);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    void GetAllTaskLists().then(items => { if (mounted.current) setLists(items ?? []); })
      .catch(() => { if (mounted.current) setStatus('plugins.builderError'); })
      .finally(() => { if (mounted.current) setBusy(false); });
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => { if (status) announce(t(status)); }, [status, t, announce]);
  const invalidate = () => { setPreview(null); setStatus(''); };
  const change = (id: string, item?: plugins.BuilderTaskList) => {
    invalidate();
    setSelected(previous => {
      const next = {...previous};
      if (item) next[id] = item;
      else delete next[id];
      return next;
    });
  };
  const run = async () => {
    if (busy) return;
    setBusy(true);
    setStatus('');
    const request = new plugins.BuilderRequest({name, version, tasklists: Object.values(selected)});
    try {
      if (preview) {
        const saved = await SaveBuilder(request, preview.digest);
        if (mounted.current && saved) setStatus('plugins.builderSaved');
      } else {
        const next = await PreviewBuilder(request);
        if (mounted.current) { setPreview(next); setStatus('plugins.builderReady'); }
      }
    } catch { if (mounted.current) { setPreview(null); setStatus('plugins.builderError'); } }
    finally { if (mounted.current) setBusy(false); }
  };
  return <form onSubmit={event => {event.preventDefault(); void run();}}>
    <h2>{t('plugins.builder')}</h2>
    <Input label={t('plugins.packageName')} value={name} required disabled={busy} onChange={event => {invalidate(); setName(event.target.value);}} />
    <Input label={t('plugins.version')} value={version} disabled={busy} onChange={event => {invalidate(); setVersion(event.target.value);}} />
    <fieldset disabled={busy}>
      <legend>{t('plugins.builderLists')}</legend>
      {lists.map(list => {
        const item = selected[list.id];
        return <div key={list.id}>
          <Checkbox label={list.title} checked={Boolean(item)} onChange={event => change(list.id, event.target.checked ? new plugins.BuilderTaskList({id:list.id,key:list.slug || '',includeTasks:false}) : undefined)} />
          {item && <>
            <Input label={t('plugins.templateKey',{name:list.title})} required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={64} value={item.key} onChange={event => change(list.id,new plugins.BuilderTaskList({...item,key:event.target.value}))} />
            <Checkbox label={t('plugins.builderTasks',{name:list.title})} checked={item.includeTasks} onChange={event => change(list.id,new plugins.BuilderTaskList({...item,includeTasks:event.target.checked}))} />
          </>}
        </div>;
      })}
    </fieldset>
    {preview && <section aria-label={t('plugins.builderContents')}>
      <h3>{t('plugins.builderContents')}</h3>
      <p>{preview.name}</p><p>SHA-256 · {preview.digest}</p>
      <ul>{preview.files.map(file => <li key={file}>{file}</li>)}</ul>
    </section>}
    <Button type="submit" disabled={busy || Object.keys(selected).length === 0}>{t(preview ? 'plugins.builderSave' : 'plugins.builderReview')}</Button>
    {status && <p>{t(status)}</p>}
  </form>;
}
