import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BuilderProviderSlugs,
  BuilderJobNames,
  BuilderProfileNames,
  BuilderSkillNames,
  PreviewBuilder,
  SaveBuilder,
} from '@wailsjs/go/wailsapi/Plugins';
import { GetAllTaskLists } from '@wailsjs/go/wailsapi/Tasklist';
import { plugins, type database, type wailsapi } from '@wailsjs/go/models';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { DialogActions } from '../components/ui/DialogActions';
import { Tabs, Tab, TabList, TabPanel } from '../components/ui/tabs/Tabs';
import { Input } from '../components/ui/Input';

interface EditorProps {
  initial?: plugins.BuilderRequest;
  saveProject: (request: plugins.BuilderRequest) => Promise<void>;
  onCancel: () => void;
  onDirty: () => void;
  onBusy: (busy: boolean) => void;
}
export default function PluginBuilder({ initial, saveProject, onCancel, onDirty, onBusy }: EditorProps) {
  const [tab, setTab] = useState('general');
  const previewHeading = useRef<HTMLHeadingElement>(null);
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [lists, setLists] = useState<database.TaskList[]>([]);
  const [providerOptions, setProviderOptions] = useState<string[]>([]);
  const [providerSelection, setProviderSelection] = useState<string[]>(initial?.providers ?? []);
  const [jobNames, setJobNames] = useState<string[]>([]);
  const [jobSelection, setJobSelection] = useState<string[]>(initial?.jobs ?? []);
  const [profileOptions, setProfileOptions] = useState<string[]>([]);
  const [profileSelection, setProfileSelection] = useState<string[]>(initial?.profiles ?? []);
  const [skillNames, setSkillNames] = useState<string[]>([]);
  const [skillSelection, setSkillSelection] = useState<string[]>(initial?.skills ?? []);
  const [name, setName] = useState(initial?.name ?? '');
  const [version, setVersion] = useState(initial?.version ?? '');
  const [selected, setSelected] = useState<Record<string, plugins.BuilderTaskList>>(() =>
    Object.fromEntries((initial?.tasklists ?? []).map((item) => [item.id, item]))
  );
  const [preview, setPreview] = useState<wailsapi.BuilderPreview | null>(null);
  const [failedResource, setFailedResource] = useState('');
  const [sourcesIncomplete, setSourcesIncomplete] = useState(false);
  const [status, setStatus] = useState('');
  const [busy, setBusy] = useState(true);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    void Promise.allSettled([
      GetAllTaskLists(),
      BuilderSkillNames(),
      BuilderProfileNames(),
      BuilderJobNames(),
      BuilderProviderSlugs(),
    ]).then(([items, names, profileList, jobList, providerList]) => {
      if (!mounted.current) return;
      if (items.status === 'fulfilled') setLists(items.value ?? []);
      if (profileList.status === 'fulfilled') setProfileOptions(profileList.value.names ?? []);
      if (names.status === 'fulfilled') setSkillNames(names.value.names ?? []);
      if (jobList.status === 'fulfilled') setJobNames(jobList.value ?? []);
      if (providerList.status === 'fulfilled') setProviderOptions(providerList.value ?? []);
      setSourcesIncomplete(
        providerList.status === 'rejected' ||
          jobList.status === 'rejected' ||
          profileList.status === 'rejected' ||
          profileList.value.partial ||
          items.status === 'rejected' ||
          names.status === 'rejected' ||
          names.value.partial
      );
      setBusy(false);
    });
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    if (sourcesIncomplete) announce(t('plugins.builderSourcesError'));
  }, [sourcesIncomplete, announce, t]);
  useEffect(() => {
    if (status) announce(t(status, { name: failedResource }));
  }, [status, failedResource, t, announce]);
  useEffect(() => {
    if (preview) previewHeading.current?.focus();
  }, [preview]);
  const invalidate = () => {
    onDirty();
    setPreview(null);
    setStatus('');
  };
  const change = (id: string, item?: plugins.BuilderTaskList) => {
    invalidate();
    setSelected((previous) => {
      const next = { ...previous };
      if (item) next[id] = item;
      else delete next[id];
      return next;
    });
  };
  const run = async (action: 'save' | 'preview' | 'export') => {
    if (busy) return;
    setBusy(true);
    onBusy(true);
    setStatus('');
    const request = new plugins.BuilderRequest({
      name,
      version,
      providers: providerSelection,
      jobs: jobSelection,
      skills: skillSelection,
      profiles: profileSelection,
      tasklists: Object.values(selected),
    });
    try {
      if (action === 'save') {
        await saveProject(request);
      } else if (action === 'export' && preview) {
        const saved = await SaveBuilder(request, preview.digest);
        if (mounted.current && saved) setStatus('plugins.builderSaved');
      } else {
        const next = await PreviewBuilder(request);
        if (mounted.current) {
          setTab('contents');
          setPreview(next);
          setStatus('plugins.builderReady');
        }
      }
    } catch (cause) {
      if (!mounted.current) return;
      const message = String(cause);
      if (action === 'save') {
        setStatus(
          message.includes('PLUGIN_PROJECT_CHANGED') ? 'plugins.projectChanged' : 'plugins.projectSaveError'
        );
      } else if (message.includes('PLUGIN_BUILDER_DESTINATION')) {
        setStatus('plugins.builderDestination');
      } else {
        setPreview(null);
        if (message.includes('PLUGIN_BUILDER_CHANGED')) setStatus('plugins.builderChanged');
        else if (message.includes('PLUGIN_BUILDER_KEY')) setStatus('plugins.builderInvalidKey');
        else if (message.includes('PLUGIN_BUILDER_PROVIDER_UNAVAILABLE:')) {
          setFailedResource(
            providerSelection.find((slug) => message.endsWith('PLUGIN_BUILDER_PROVIDER_UNAVAILABLE: ' + slug)) ??
              ''
          );
          setStatus('plugins.builderProviderError');
        } else if (message.includes('PLUGIN_BUILDER_PROFILE_UNAVAILABLE:')) {
          setFailedResource(/PLUGIN_BUILDER_PROFILE_UNAVAILABLE:([a-z0-9-]+)/.exec(message)?.[1] ?? '');
          setStatus('plugins.builderNamedProfileError');
        } else if (message.includes('PLUGIN_BUILDER_SKILL_UNAVAILABLE')) {
          const key = /PLUGIN_BUILDER_SKILL_UNAVAILABLE:([a-z0-9-]+)/.exec(message)?.[1];
          setFailedResource(key ?? '');
          setStatus(key ? 'plugins.builderNamedSkillError' : 'plugins.builderSkillError');
        } else if (message.includes('PLUGIN_BUILDER_JOB_UNAVAILABLE:')) {
          setFailedResource(
            jobSelection.find((key) => message.endsWith('PLUGIN_BUILDER_JOB_UNAVAILABLE: ' + key)) ?? ''
          );
          setStatus('plugins.builderJobError');
        } else setStatus('plugins.builderError');
      }
    } finally {
      if (mounted.current) {
        setBusy(false);
        onBusy(false);
      }
    }
  };
  const titles = new Set<string>();
  const duplicates = new Set<string>();
  for (const list of lists) {
    if (titles.has(list.title)) duplicates.add(list.title);
    titles.add(list.title);
  }
  const resources = [
    'builderProviders',
    'builderJobs',
    'builderProfiles',
    'builderSkills',
    'builderLists',
  ] as const;
  const count =
    Object.keys(selected).length +
    skillSelection.length +
    profileSelection.length +
    providerSelection.length +
    jobSelection.length;
  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (name.trim()) void run('save');
      }}
    >
      {sourcesIncomplete && <p>{t('plugins.builderSourcesError')}</p>}
      <Tabs value={tab} onValueChange={setTab}>
        <TabList ariaLabel={t('plugins.projectEditor')}>
          <Tab value="general">{t('plugins.projectGeneral')}</Tab>
          {resources.map((key) => (
            <Tab key={key} value={key}>
              {t('plugins.' + key)}
            </Tab>
          ))}
          <Tab value="contents">{t('plugins.builderContents')}</Tab>
        </TabList>
        <TabPanel value="general">
          <Input
            label={t('plugins.packageName')}
            value={name}
            required
            disabled={busy}
            onChange={(event) => {
              invalidate();
              setName(event.target.value);
            }}
          />
          <Input
            label={t('plugins.version')}
            value={version}
            disabled={busy}
            onChange={(event) => {
              invalidate();
              setVersion(event.target.value);
            }}
          />
        </TabPanel>
        <TabPanel value="builderProviders">
          <fieldset disabled={busy}>
            <legend>{t('plugins.builderProviders')}</legend>
            {Array.from(new Set([...providerOptions, ...providerSelection])).map((slug) => (
              <Checkbox
                key={slug}
                label={slug}
                checked={providerSelection.includes(slug)}
                onChange={(event) => {
                  invalidate();
                  const checked = event.target.checked;
                  setProviderSelection((previous) =>
                    checked ? [...previous, slug] : previous.filter((value) => value !== slug)
                  );
                }}
              />
            ))}
          </fieldset>
        </TabPanel>
        <TabPanel value="builderJobs">
          <fieldset disabled={busy}>
            <legend>{t('plugins.builderJobs')}</legend>
            <p>{t('plugins.builderJobInputs')}</p>
            {Array.from(new Set([...jobNames, ...jobSelection])).map((slug) => (
              <Checkbox
                key={slug}
                label={slug}
                checked={jobSelection.includes(slug)}
                onChange={(event) => {
                  invalidate();
                  const checked = event.target.checked;
                  setJobSelection((previous) =>
                    checked ? [...previous, slug] : previous.filter((value) => value !== slug)
                  );
                }}
              />
            ))}
          </fieldset>
        </TabPanel>
        <TabPanel value="builderProfiles">
          <fieldset disabled={busy}>
            <legend>{t('plugins.builderProfiles')}</legend>
            <p>{t('plugins.builderProfileFields')}</p>
            {Array.from(new Set([...profileOptions, ...profileSelection])).map((slug) => (
              <Checkbox
                key={slug}
                label={slug}
                checked={profileSelection.includes(slug)}
                onChange={(event) => {
                  invalidate();
                  const checked = event.target.checked;
                  setProfileSelection((previous) =>
                    checked ? [...previous, slug] : previous.filter((value) => value !== slug)
                  );
                }}
              />
            ))}
          </fieldset>
        </TabPanel>
        <TabPanel value="builderSkills">
          <fieldset disabled={busy}>
            <legend>{t('plugins.builderSkills')}</legend>
            {Array.from(new Set([...skillNames, ...skillSelection])).map((slug) => (
              <Checkbox
                key={slug}
                label={slug}
                checked={skillSelection.includes(slug)}
                onChange={(event) => {
                  invalidate();
                  setSkillSelection((previous) =>
                    event.target.checked ? [...previous, slug] : previous.filter((value) => value !== slug)
                  );
                }}
              />
            ))}
          </fieldset>
        </TabPanel>
        <TabPanel value="builderLists">
          <fieldset disabled={busy}>
            <legend>{t('plugins.builderLists')}</legend>
            {[
              ...lists,
              ...Object.values(selected)
                .filter((item) => !lists.some((list) => list.id === item.id))
                .map((item) => ({ id: item.id, title: item.key || item.id, slug: item.key, description: '' })),
            ].map((list, index) => {
              const item = selected[list.id];
              const label = duplicates.has(list.title)
                ? [
                    t('plugins.builderListPosition', { name: list.title, position: index + 1 }),
                    list.slug,
                    list.description,
                  ]
                    .filter(Boolean)
                    .join(' · ')
                : list.title;
              return (
                <div key={list.id}>
                  <Checkbox
                    label={label}
                    checked={Boolean(item)}
                    onChange={(event) =>
                      change(
                        list.id,
                        event.target.checked
                          ? new plugins.BuilderTaskList({ id: list.id, key: list.slug || '', includeTasks: false })
                          : undefined
                      )
                    }
                  />
                  {item && (
                    <>
                      <Input
                        label={t('plugins.templateKey', { name: label })}
                        required
                        pattern="[a-z0-9]+(-[a-z0-9]+)*"
                        maxLength={64}
                        value={item.key}
                        onChange={(event) =>
                          change(list.id, new plugins.BuilderTaskList({ ...item, key: event.target.value }))
                        }
                      />
                      <Checkbox
                        label={t('plugins.builderTasks', { name: label })}
                        checked={item.includeTasks}
                        onChange={(event) =>
                          change(
                            list.id,
                            new plugins.BuilderTaskList({ ...item, includeTasks: event.target.checked })
                          )
                        }
                      />
                    </>
                  )}
                </div>
              );
            })}
          </fieldset>
        </TabPanel>
        <TabPanel value="contents">
          <Button type="button" onClick={() => void run('preview')} disabled={busy || !name.trim() || !count}>
            {t('plugins.builderReview')}
          </Button>
          {preview && (
            <section aria-label={t('plugins.builderContents')}>
              <h3 ref={previewHeading} tabIndex={-1}>
                {t('plugins.builderContents')}
              </h3>
              <p>{preview.name}</p>
              <p>SHA-256 · {preview.digest}</p>
              <ul>
                {preview.files.map((file) => (
                  <li key={file}>
                    <details>
                      <summary>{file}</summary>
                      <pre>{preview.contents[file]}</pre>
                    </details>
                  </li>
                ))}
              </ul>
              <Button type="button" onClick={() => void run('export')} disabled={busy}>
                {t('plugins.builderSave')}
              </Button>
            </section>
          )}
        </TabPanel>
      </Tabs>
      <DialogActions
        primary={
          <Button type="submit" disabled={busy || !name.trim()}>
            {t('plugins.saveProject')}
          </Button>
        }
        secondary={
          <Button type="button" variant="secondary" disabled={busy} onClick={onCancel}>
            {t('common.cancel')}
          </Button>
        }
      />
      {status && <p>{t(status, { name: failedResource })}</p>}
    </form>
  );
}
