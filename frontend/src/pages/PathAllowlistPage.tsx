import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DeleteOutlined, ReloadOutlined } from '@ant-design/icons';
import {
  AddPathDenyEntry,
  GetPathAllowlist,
  RemovePathAllowlistEntry,
} from '@wailsjs/go/wailsapi/FSTrust';

import { logger } from '../utils/logger';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { PageLoading } from '../components/ui/PageLoading';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Select } from '../components/ui/Select';
import './PathAllowlistPage.css';

interface PathAllowlistRow {
  id: string;
  path: string;
  kind: string;
  operation: string;
  effect: string;
  scope: string;
  createdBy: string;
  createdAt: string;
  reason: string;
  [key: string]: unknown;
}

interface DenyFormState {
  path: string;
  kind: string;
  operation: string;
  scope: string;
  reason: string;
}

const EMPTY_DENY_FORM: DenyFormState = {
  path: '',
  kind: 'file',
  operation: '',
  scope: 'workspace',
  reason: '',
};

const KNOWN_SCOPES = new Set(['session', 'workspace', 'profile', 'global']);
const KNOWN_KINDS = new Set(['file', 'dir']);
const KNOWN_EFFECTS = new Set(['allow', 'deny']);
const PERSISTENT_SCOPES = ['workspace', 'profile', 'global'] as const;

/** Data no idioma de quem lê; o que não for data válida aparece como veio. */
function formatCreatedAt(value: string, language: string): string {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language);
}

export default function PathAllowlistPage() {
  const { t, i18n } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'path-allowlist-page' });
  const requestConfirm = useConfirm();

  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<PathAllowlistRow[]>([]);
  const [loadFailed, setLoadFailed] = useState(false);
  const [focused, setFocused] = useState<PathAllowlistRow | null>(null);
  const [denyForm, setDenyForm] = useState<DenyFormState>(EMPTY_DENY_FORM);
  const [pathError, setPathError] = useState<string | undefined>();
  const [operationError, setOperationError] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  const scopeName = useCallback(
    (scope: string) =>
      KNOWN_SCOPES.has(scope)
        ? t(`pathAllowlist.scope.${scope}`)
        : t('pathAllowlist.scope.unknown'),
    [t],
  );

  const kindName = useCallback(
    (kind: string) =>
      KNOWN_KINDS.has(kind) ? t(`pathAllowlist.kind.${kind}`) : t('pathAllowlist.kind.unknown'),
    [t],
  );

  const effectName = useCallback(
    (effect: string) =>
      KNOWN_EFFECTS.has(effect)
        ? t(`pathAllowlist.effect.${effect}`)
        : t('pathAllowlist.effect.unknown'),
    [t],
  );

  const mapEntries = useCallback(
    (entries: Array<{
      path: string;
      kind: string;
      operation: string;
      effect: string;
      scope: string;
      createdBy?: string;
      createdAt: string;
      reason?: string;
    }>) =>
      entries.map((entry) => ({
        id: `${entry.scope}:${entry.effect}:${entry.kind}:${entry.operation}:${entry.path}`,
        path: entry.path,
        kind: entry.kind,
        operation: entry.operation,
        effect: entry.effect,
        scope: entry.scope,
        createdBy: entry.createdBy ?? '',
        createdAt: entry.createdAt,
        reason: entry.reason ?? '',
      })),
    [],
  );

  const load = useCallback(async () => {
    setLoading(true);
    // A entrada sob o foco pode não existir depois desta carga; manter a marca
    // deixaria a barra oferecendo "revogar" sobre algo que saiu da lista.
    setFocused(null);
    try {
      const entries = (await GetPathAllowlist()) ?? [];
      setLoadFailed(false);
      setRows(mapEntries(entries));
    } catch (error) {
      logger.error('Erro ao carregar allowlist de path:', error);
      addToast(t('pathAllowlist.error.loadFailed'), 'error');
      setLoadFailed(true);
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [addToast, mapEntries, t]);

  useEffect(() => {
    load();
  }, [load]);

  const remove = useCallback(
    async (row: PathAllowlistRow) => {
      const scope = scopeName(row.scope);
      const confirmed = await requestConfirm({
        title: t('pathAllowlist.confirm.title'),
        message: t('pathAllowlist.confirm.message', { path: row.path, scope }),
        confirmText: t('pathAllowlist.actions.remove'),
        cancelText: t('common.cancel'),
        variant: 'danger',
      });
      if (!confirmed) {
        return;
      }

      try {
        await RemovePathAllowlistEntry(row.scope, row.path, row.kind, row.operation, row.effect);
        // Remoção já valeu no backend: tira a linha localmente antes de
        // sincronizar. Se a sincronização falhar, a grade otimista permanece
        // (não cair em loadFailedBody, que contradiria o toast de sucesso).
        setRows((current) => current.filter((item) => item.id !== row.id));
        setFocused((current) => (current?.id === row.id ? null : current));
        setLoadFailed(false);
        addToast(t('pathAllowlist.toast.removed'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('pathAllowlist.announce.removed', { path: row.path, scope }));
        try {
          const entries = (await GetPathAllowlist()) ?? [];
          setRows(mapEntries(entries));
        } catch (error) {
          logger.error('Erro ao sincronizar allowlist de path após remoção:', error);
          addToast(t('pathAllowlist.error.reloadAfterRemoveFailed'), 'warning');
        }
      } catch {
        addToast(t('pathAllowlist.error.removeFailed'), 'error');
      }
    },
    [addToast, announce, mapEntries, requestConfirm, scopeName, t],
  );

  const submitDeny = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      const path = denyForm.path.trim();
      const operation = denyForm.operation.trim();
      let hasError = false;
      if (!path) {
        setPathError(t('pathAllowlist.form.pathRequired'));
        hasError = true;
      } else {
        setPathError(undefined);
      }
      if (!operation) {
        setOperationError(t('pathAllowlist.form.operationRequired'));
        hasError = true;
      } else {
        setOperationError(undefined);
      }
      if (hasError) {
        return;
      }

      setSubmitting(true);
      try {
        await AddPathDenyEntry(
          path,
          denyForm.kind,
          operation,
          denyForm.scope,
          denyForm.reason.trim(),
        );
        setDenyForm(EMPTY_DENY_FORM);
        setPathError(undefined);
        setOperationError(undefined);
        addToast(t('pathAllowlist.toast.denyAdded'), 'success', undefined, undefined, {
          suppressAnnounce: true,
        });
        announce(t('pathAllowlist.announce.denyAdded', { path }));
        try {
          const entries = (await GetPathAllowlist()) ?? [];
          setLoadFailed(false);
          setRows(mapEntries(entries));
        } catch (error) {
          logger.error('Erro ao sincronizar allowlist de path após deny:', error);
          addToast(t('pathAllowlist.error.loadFailed'), 'warning');
        }
      } catch (error) {
        logger.error('Erro ao adicionar denylist de path:', error);
        addToast(t('pathAllowlist.error.addFailed'), 'error');
      } finally {
        setSubmitting(false);
      }
    },
    [addToast, announce, denyForm, mapEntries, t],
  );

  const rowActions = useCallback(
    (row: PathAllowlistRow) => [
      {
        id: 'remove',
        label: t('pathAllowlist.actions.remove'),
        icon: <DeleteOutlined aria-hidden="true" />,
        onClick: () => remove(row),
        danger: true,
      },
    ],
    [remove, t],
  );

  const columns: DataGridColumn<PathAllowlistRow>[] = [
    { key: 'path', label: t('pathAllowlist.columns.path'), width: '280px', truncate: true },
    {
      key: 'effect',
      label: t('pathAllowlist.columns.effect'),
      width: '110px',
      format: (_value, row) => effectName(row.effect),
    },
    {
      key: 'kind',
      label: t('pathAllowlist.columns.kind'),
      width: '120px',
      format: (_value, row) => kindName(row.kind),
    },
    { key: 'operation', label: t('pathAllowlist.columns.operation'), width: '120px' },
    {
      key: 'scope',
      label: t('pathAllowlist.columns.scope'),
      width: '150px',
      format: (_value, row) => scopeName(row.scope),
    },
    { key: 'createdBy', label: t('pathAllowlist.columns.createdBy'), width: '150px', truncate: true },
    {
      key: 'createdAt',
      label: t('pathAllowlist.columns.createdAt'),
      width: '190px',
      format: (_value, row) => formatCreatedAt(row.createdAt, i18n.language),
    },
    { key: 'reason', label: t('pathAllowlist.columns.reason'), truncate: true },
    {
      key: 'actions',
      label: t('common.actions'),
      width: '80px',
      format: (_value, row) => (
        <MenuButton items={rowActions(row)} buttonLabel={t('common.actions')} />
      ),
    },
  ];

  const getRowId = useCallback((row: PathAllowlistRow) => row.id, []);
  const handleFocusChange = useCallback(
    (row: PathAllowlistRow | null) => setFocused(row),
    [],
  );

  const kindOptions = [
    { value: 'file', label: t('pathAllowlist.kind.file') },
    { value: 'dir', label: t('pathAllowlist.kind.dir') },
  ];

  const scopeOptions = PERSISTENT_SCOPES.map((scope) => ({
    value: scope,
    label: t(`pathAllowlist.scope.${scope}`),
  }));

  const toolbarActions = [
    {
      key: 'remove',
      label: t('pathAllowlist.actions.remove'),
      icon: <DeleteOutlined aria-hidden="true" />,
      onClick: () => focused && remove(focused),
      disabled: !focused,
      variant: 'danger' as const,
    },
    {
      key: 'reload',
      label: t('pathAllowlist.actions.reload'),
      icon: <ReloadOutlined aria-hidden="true" />,
      onClick: load,
      disabled: false,
      variant: 'secondary' as const,
    },
  ];

  if (loading) {
    return (
      <div className="path-allowlist-page">
        <PageLoading
          className="path-allowlist-page__loading"
          message={t('pathAllowlist.loading')}
        />
      </div>
    );
  }

  return (
    <div className="path-allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('pathAllowlist.title')}</h1>}
        actions={toolbarActions}
        ariaLabel={t('pathAllowlist.toolbarLabel')}
      />
      <div className="path-allowlist-page__content">
        <p className="path-allowlist-page__description">{t('pathAllowlist.description')}</p>
        <p className="path-allowlist-page__note">{t('pathAllowlist.sessionNote')}</p>

        <form className="path-allowlist-page__form" noValidate onSubmit={(event) => void submitDeny(event)}>
          <h2 className="path-allowlist-page__form-title">{t('pathAllowlist.form.title')}</h2>
          <div className="path-allowlist-page__form-grid">
            <Input
              id="path-deny-path"
              label={t('pathAllowlist.form.path')}
              value={denyForm.path}
              onChange={(event) => {
                setDenyForm((current) => ({ ...current, path: event.target.value }));
                if (pathError) setPathError(undefined);
              }}
              error={pathError}
              fullWidth
              required
            />
            <Select
              id="path-deny-kind"
              label={t('pathAllowlist.form.kind')}
              value={denyForm.kind}
              options={kindOptions}
              onChange={(event) =>
                setDenyForm((current) => ({ ...current, kind: event.target.value }))
              }
              fullWidth
              required
            />
            <Input
              id="path-deny-operation"
              label={t('pathAllowlist.form.operation')}
              value={denyForm.operation}
              onChange={(event) => {
                setDenyForm((current) => ({ ...current, operation: event.target.value }));
                if (operationError) setOperationError(undefined);
              }}
              error={operationError}
              fullWidth
              required
            />
            <Select
              id="path-deny-scope"
              label={t('pathAllowlist.form.scope')}
              value={denyForm.scope}
              options={scopeOptions}
              onChange={(event) =>
                setDenyForm((current) => ({ ...current, scope: event.target.value }))
              }
              fullWidth
              required
            />
            <Input
              id="path-deny-reason"
              label={t('pathAllowlist.form.reason')}
              value={denyForm.reason}
              onChange={(event) =>
                setDenyForm((current) => ({ ...current, reason: event.target.value }))
              }
              fullWidth
            />
          </div>
          <div className="path-allowlist-page__form-actions">
            <Button type="submit" variant="primary" loading={submitting}>
              {t('pathAllowlist.form.submit')}
            </Button>
          </div>
        </form>

        {loadFailed ? (
          // Cair no texto de lista vazia depois de uma falha diria que nenhum
          // path está autorizado. Os que existirem continuam valendo.
          <p className="path-allowlist-page__empty">{t('pathAllowlist.loadFailedBody')}</p>
        ) : rows.length > 0 ? (
          <DataGrid
            items={rows}
            columns={columns}
            label={t('pathAllowlist.gridLabel')}
            autoFocusOnMount={false}
            getItemId={getRowId}
            onGridReady={handleGridReady}
            getRowActions={rowActions}
            onFocusChange={handleFocusChange}
          />
        ) : (
          <p className="path-allowlist-page__empty">{t('pathAllowlist.empty')}</p>
        )}
      </div>
    </div>
  );
}
