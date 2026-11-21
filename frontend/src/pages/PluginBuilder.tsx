import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BuilderJobNames, BuilderProfileNames, BuilderSkillNames, PreviewBuilder, SaveBuilder } from '@wailsjs/go/wailsapi/Plugins';
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
  const [jobNames, setJobNames] = useState<string[]>([]);
  const [jobSelection, setJobSelection] = useState<string[]>([]);
  const [profileOptions, setProfileOptions] = useState<string[]>([]);
  const [profileSelection, setProfileSelection] = useState<string[]>([]);
  const [skillNames, setSkillNames] = useState<string[]>([]);
  const [skillSelection, setSkillSelection] = useState<string[]>([]);
  const [name, setName] = useState('');
  const [version, setVersion] = useState('');
  const [selected, setSelected] = useState<Record<string, plugins.BuilderTaskList>>({});
  const [preview, setPreview] = useState<wailsapi.BuilderPreview | null>(null);
  const [status, setStatus] = useState('');
  const [unavailableJob, setUnavailableJob] = useState('');
  const [busy, setBusy] = useState(true);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    void Promise.allSettled([GetAllTaskLists(), BuilderSkillNames(), BuilderProfileNames(), BuilderJobNames()]).then(([items, names, profileList, jobList]) => {
      if (!mounted.current) return;
      if (items.status === 'fulfilled') setLists(items.value ?? []);
      if (names.status === 'fulfilled') setSkillNames(names.value ?? []);
      if (profileList.status === 'fulfilled') setProfileOptions(profileList.value ?? []);
      if (jobList.status === 'fulfilled') setJobNames(jobList.value ?? []);
      if (jobList.status === 'rejected' || profileList.status === 'rejected' || items.status === 'rejected' || names.status === 'rejected') setStatus('plugins.builderSourcesError');
      setBusy(false);
    });
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => { if (status) announce(t(status, {name:unavailableJob})); }, [status, unavailableJob, t, announce]);
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
    const request = new plugins.BuilderRequest({name, version, jobs:jobSelection, skills:skillSelection, profiles:profileSelection, tasklists: Object.values(selected)});
    try {
      if (preview) {
        const saved = await SaveBuilder(request, preview.digest);
        if (mounted.current && saved) setStatus('plugins.builderSaved');
      } else {
        const next = await PreviewBuilder(request);
        if (mounted.current) { setPreview(next); setStatus('plugins.builderReady'); }
      }
    } catch (cause) {
      if (mounted.current) {
        setPreview(null);
        const failedJob = jobSelection.find(key => String(cause).endsWith('PLUGIN_BUILDER_JOB_UNAVAILABLE: ' + key));
        setUnavailableJob(failedJob ?? '');
        setStatus(/PLUGIN_BUILDER_SKILL_UNAVAILABLE|SKILL_BUILDER_SUPPORTING_FILES/.test(String(cause)) ? 'plugins.builderSkillError' : String(cause).includes('PLUGIN_BUILDER_JOB_UNAVAILABLE') ? 'plugins.builderJobError' : 'plugins.builderError');
      }
    }
    finally { if (mounted.current) setBusy(false); }
  };
  return <form onSubmit={event => {event.preventDefault(); void run();}}>
    <h2>{t('plugins.builder')}</h2>
    <Input label={t('plugins.packageName')} value={name} required disabled={busy} onChange={event => {invalidate(); setName(event.target.value);}} />
    <Input label={t('plugins.version')} value={version} disabled={busy} onChange={event => {invalidate(); setVersion(event.target.value);}} />
    <fieldset disabled={busy}>
      <legend>{t('plugins.builderJobs')}</legend>
      <p>{t('plugins.builderJobInputs')}</p>
      {jobNames.map(slug => <Checkbox key={slug} label={slug} checked={jobSelection.includes(slug)} onChange={event => {
        invalidate();
        const checked = event.target.checked;
        setJobSelection(previous => checked ? [...previous, slug] : previous.filter(value => value !== slug));
      }} />)}
    </fieldset>
    <fieldset disabled={busy}>
      <legend>{t('plugins.builderProfiles')}</legend>
      <p>{t('plugins.builderProfileFields')}</p>
      {profileOptions.map(slug => <Checkbox key={slug} label={slug} checked={profileSelection.includes(slug)} onChange={event => {
        invalidate();
        const checked = event.target.checked;
        setProfileSelection(previous => checked ? [...previous, slug] : previous.filter(value => value !== slug));
      }} />)}
    </fieldset>
    <fieldset disabled={busy}>
      <legend>{t('plugins.builderSkills')}</legend>
      {skillNames.map(slug => <Checkbox key={slug} label={slug} checked={skillSelection.includes(slug)} onChange={event => {
        invalidate();
        setSkillSelection(previous => event.target.checked ? [...previous, slug] : previous.filter(value => value !== slug));
      }} />)}
    </fieldset>
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
      <ul>{preview.files.map(file => <li key={file}><details><summary>{file}</summary><pre>{preview.contents[file]}</pre></details></li>)}</ul>
    </section>}
    <Button type="submit" disabled={busy || (Object.keys(selected).length === 0 && skillSelection.length === 0 && profileSelection.length === 0 && jobSelection.length === 0)}>{t(preview ? 'plugins.builderSave' : 'plugins.builderReview')}</Button>
    {status && <p>{t(status, {name:unavailableJob})}</p>}
  </form>;
}
