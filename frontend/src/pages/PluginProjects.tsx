import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BuilderProjects, SaveBuilderProject } from '@wailsjs/go/wailsapi/Plugins';
import { plugins } from '@wailsjs/go/models';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid } from '../components/ui/DataGrid';
import { Modal } from '../components/ui/Modal';
import { Button } from '../components/ui/Button';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { usePagePresentationCommands } from '../lib/commandPagePresentation';
import PluginBuilder from './PluginBuilder';

export default function PluginProjects({ session }: { session: string }) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [rows, setRows] = useState<plugins.BuilderProject[]>([]);
  const [query, setQuery] = useState('');
  const [load, setLoad] = useState(0);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [editor, setEditor] = useState<plugins.BuilderProject | 'new' | null>(null);
  const [busy, setBusy] = useState(false);
  const dirty = useRef(false);
  const root = useRef<HTMLDivElement>(null);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    let active = true;
    setLoading(true);
    setFailed(false);
    BuilderProjects(session)
      .then((items) => {
        if (active) setRows(items ?? []);
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      mounted.current = false;
    };
  }, [session, load]);
  useEffect(() => {
    if (failed) announce(t('plugins.projectListError'));
  }, [failed, announce, t]);

  const open = (project: plugins.BuilderProject | 'new') => {
    dirty.current = false;
    setBusy(false);
    setEditor(project);
  };
  const { request } = usePagePresentationCommands({
    root,
    pathname: '/plugins',
    allowedCommands: ['plugins.create.open'],
    readTarget: () => session,
    isCurrent: () => mounted.current && !editor,
    canOpen: (id) => id === 'plugins.create.open' && !editor && !loading && !failed,
    open: (id) => {
      if (id !== 'plugins.create.open' || editor || loading || failed) return false;
      open('new');
      return true;
    },
  });
  const close = async () => {
    if (busy) return;
    if (
      dirty.current &&
      !(await confirm({
        title: t('plugins.discardProject'),
        message: t('plugins.discardProjectChanges'),
        confirmText: t('plugins.discardChanges'),
        cancelText: t('common.cancel'),
      }))
    )
      return;
    if (mounted.current) setEditor(null);
  };
  const save = async (selection: plugins.BuilderRequest) => {
    if (!editor) return;
    const project = new plugins.BuilderProject({
      id: editor === 'new' ? '' : editor.id,
      revision: editor === 'new' ? 0 : editor.revision,
      selection,
    });
    const saved = await SaveBuilderProject(session, project).catch((error) => {
      if (mounted.current && String(error).includes('PLUGIN_PROJECT_CHANGED')) setLoad((value) => value + 1);
      throw error;
    });
    if (!mounted.current) return;
    setRows((previous) =>
      previous.some((row) => row.id === saved.id)
        ? previous.map((row) => (row.id === saved.id ? saved : row))
        : [...previous, saved]
    );
    setQuery('');
    setEditor(null);
    announce(t('plugins.projectSaved', { name: saved.selection.name }));
  };
  const filtered = rows.filter((row) =>
    row.selection.name.toLocaleLowerCase().includes(query.toLocaleLowerCase())
  );
  return (
    <div ref={root}>
      <Toolbar
        ariaLabel={t('plugins.builder')}
        left={<h2>{t('plugins.builder')}</h2>}
        searchValue={query}
        onSearchChange={setQuery}
        searchPlaceholder={t('plugins.searchProjects')}
        actions={[
          {
            key: 'new-plugin',
            label: t('plugins.newProject'),
            variant: 'primary',
            disabled: loading || failed,
            onClick: () => request('plugins.create.open'),
          },
        ]}
      />
      {failed ? (
        <>
          <p>{t('plugins.projectListError')}</p>
          <Button onClick={() => setLoad((value) => value + 1)}>{t('plugins.reloadList')}</Button>
        </>
      ) : loading ? (
        <p>{t('common.loading')}</p>
      ) : (
        <DataGrid
          items={filtered}
          getItemId={(row) => row.id}
          onActivate={open}
          label={t('plugins.builder')}
          columns={[
            { key: 'name', label: t('plugins.packageName'), format: (_, row) => row.selection.name },
            { key: 'version', label: t('plugins.version'), format: (_, row) => row.selection.version },
          ]}
        />
      )}
      <Modal
        isOpen={editor !== null}
        title={editor === 'new' ? t('plugins.newProject') : (editor?.selection.name ?? '')}
        size="xl"
        allowClose={!busy}
        onClose={() => void close()}
      >
        {editor && (
          <PluginBuilder
            initial={editor === 'new' ? undefined : editor.selection}
            saveProject={save}
            onCancel={() => void close()}
            onDirty={() => {
              dirty.current = true;
            }}
            onBusy={setBusy}
          />
        )}
      </Modal>
    </div>
  );
}
