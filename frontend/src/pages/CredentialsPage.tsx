import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ListCredentials, UpsertCredential, DeleteCredential } from '@wailsjs/go/main/App';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, Input, Select } from '../components';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useEditableList } from '../hooks/useEditableList';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './CredentialsPage.css';

interface CredentialRow {
  id: string;
  pattern: string;
  type: string;
  masked: string;
  managed: boolean;
  token?: string;
  username?: string;
  password?: string;
  headerName?: string;
  headerValue?: string;
  [key: string]: unknown;
}

export default function CredentialsPage() {
  const { t } = useTranslation();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'credentials-page' });
  const [focusedRow, setFocusedRow] = useState<CredentialRow | null>(null);

  const typeOptions = [
    { value: 'bearer', label: t('credentials.types.bearer') },
    { value: 'basic', label: t('credentials.types.basic') },
    { value: 'custom', label: t('credentials.types.custom') },
    { value: 'secret', label: t('credentials.types.secret') },
  ];

  const crud = useEditableList<CredentialRow, CredentialRow, CredentialRow>(
    {
      loadItems: async () => {
        const list = await ListCredentials();
        return (list || []).map((c) => ({
          id: c.pattern,
          pattern: c.pattern,
          type: c.type,
          masked: c.masked || '',
          managed: c.managed ?? false,
          token: '',
          username: '',
          password: '',
          headerName: '',
          headerValue: '',
        }));
      },
      loadItem: async (id) => {
        const list = await ListCredentials();
        const found = (list || []).find((c) => c.pattern === id);
        return {
          id: String(id),
          pattern: found?.pattern || String(id),
          type: found?.type || 'bearer',
          masked: found?.masked || '',
          managed: found?.managed ?? false,
          token: '',
          username: '',
          password: '',
          headerName: '',
          headerValue: '',
        };
      },
      createItem: async (data) => {
        await UpsertCredential({
          pattern: data.pattern,
          type: data.type,
          token: data.token,
          username: data.username,
          password: data.password,
          headerName: data.headerName,
          headerValue: data.headerValue,
        });
        return data.pattern;
      },
      updateItem: async (_id, data) => {
        await UpsertCredential({
          pattern: data.pattern,
          type: data.type,
          token: data.token,
          username: data.username,
          password: data.password,
          headerName: data.headerName,
          headerValue: data.headerValue,
        });
      },
      deleteItem: async (id) => {
        await DeleteCredential(String(id));
      },
    },
    {
      entityName: 'Credencial',
      messages: {
        loadError: 'Erro ao carregar credenciais',
        createSuccess: 'Credencial criada!',
        updateSuccess: 'Credencial atualizada!',
        deleteSuccess: 'Credencial removida!',
        deleteConfirm: (item) => `Remover credencial ${item.pattern}?`,
      },
      createDefault: () => ({
        id: '',
        pattern: '',
        type: 'bearer',
        masked: '',
        managed: false,
        token: '',
        username: '',
        password: '',
        headerName: '',
        headerValue: '',
      }),
      validate: (item) => {
        if (!item.pattern || !item.pattern.trim()) {
          return 'Pattern é obrigatório';
        }
        if (!item.type) {
          return 'Tipo é obrigatório';
        }
        if (item.type === 'basic') {
          if (!item.username || !item.password) {
            return 'Usuário e senha são obrigatórios';
          }
        }
        if (item.type === 'custom') {
          if (!item.headerName || !item.headerValue) {
            return 'Header e valor são obrigatórios';
          }
        }
        if (item.type === 'bearer' || item.type === 'oauth2' || item.type === 'secret') {
          if (!item.token) {
            return 'Token é obrigatório';
          }
        }
        return null;
      },
    }
  );

  useEffect(() => {
    crud.loadItems();
  }, []);

  useResourceEditRequest('credentials', {
    onEdit: (pattern) => crud.openEdit({ id: pattern, pattern } as CredentialRow),
    onNew: () => crud.openNew(),
    ready: !crud.loading && crud.items.length > 0,
  });

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (!event.ctrlKey || event.shiftKey || event.altKey) return;
      if (event.key !== 'n' && event.key !== 'N') return;
      const target = event.target as HTMLElement | null;
      const isInput =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.isContentEditable;
      if (isInput) return;
      event.preventDefault();
      crud.openNew();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [crud]);

  const [viewingManaged, setViewingManaged] = useState<CredentialRow | null>(null);

  const getRowId = useCallback((row: CredentialRow) => row.id, []);
  const handleActivateRow = useCallback(
    (row: CredentialRow) => {
      if (row.managed) setViewingManaged(row);
      else crud.openEdit(row);
    },
    [crud]
  );
  const handleDeleteRow = useCallback(
    (row: CredentialRow) => {
      if (!row.managed) crud.deleteItem(row);
    },
    [crud]
  );
  const handleFocusChange = useCallback((row: CredentialRow | null) => setFocusedRow(row), []);

  const columns: DataGridColumn<CredentialRow>[] = [
    { key: 'pattern', label: t('credentials.labels.pattern'), width: '260px', truncate: true },
    { key: 'type', label: t('credentials.labels.type'), width: '120px' },
    { key: 'masked', label: t('credentials.labels.value'), truncate: true },
    {
      key: 'managed',
      label: t('credentials.labels.origin', 'Origem'),
      width: '100px',
      format: (val) => (val ? t('credentials.origin.system', 'Sistema') : t('credentials.origin.manual', 'Manual')),
    },
    {
      key: 'actions',
      label: '',
      width: '6%',
      format: (_val, row) => (
        <MenuButton
          items={getCredentialRowActions(row)}
          buttonLabel={t('credentials.actions', 'Ações')}
        />
      ),
    },
  ];

  function getCredentialRowActions(row: CredentialRow) {
    if (row.managed) {
      return [
        {
          id: 'view',
          label: t('credentials.buttons.view', 'Visualizar'),
          icon: '👁',
          onClick: () => setViewingManaged(row),
        },
      ];
    }
    return [
      {
        id: 'edit',
        label: t('credentials.buttons.edit', 'Editar'),
        icon: '✏️',
        onClick: () => crud.openEdit(row),
      },
      {
        id: 'delete',
        label: t('credentials.buttons.delete', 'Excluir'),
        icon: '🗑️',
        onClick: () => crud.deleteItem(row),
        danger: true,
      },
    ];
  }

  return (
    <div className="credentials-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('credentials.pageTitle')}</h1>}
        ariaLabel={t('credentials.aria.toolbar')}
        actions={[
          {
            key: 'new',
            label: t('credentials.buttons.new'),
            icon: '+',
            onClick: crud.openNew,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'edit',
            label: t('credentials.buttons.edit', 'Editar'),
            icon: '✏️',
            onClick: () => focusedRow && !focusedRow.managed && crud.openEdit(focusedRow),
            disabled: !focusedRow || focusedRow.managed,
          },
          {
            key: 'delete',
            label: t('credentials.buttons.delete', 'Excluir'),
            icon: '🗑️',
            onClick: () => focusedRow && !focusedRow.managed && crud.deleteItem(focusedRow),
            disabled: !focusedRow || focusedRow.managed,
            variant: 'danger',
          },
        ]}
      />

      <div className="credentials-page__content">
        <DataGrid
          columns={columns}
          items={crud.items}
          getItemId={getRowId}
          onActivate={handleActivateRow}
          onDelete={handleDeleteRow}
          label={t('credentials.pageTitle')}
          onGridReady={handleGridReady}
          getRowActions={getCredentialRowActions}
          onFocusChange={handleFocusChange}
        />
      </div>

      <Modal
        isOpen={Boolean(crud.editingItem)}
        onClose={crud.closeEditor}
        title={crud.isNew ? t('credentials.modal.newTitle') : t('credentials.modal.editTitle')}
        size="md"
      >
        {crud.editingItem && (
          <div className="credentials-page__fields">
            <Input
              label={t('credentials.labels.pattern')}
              value={crud.editingItem.pattern}
              onChange={(e) => crud.updateField('pattern', e.target.value)}
              placeholder={t('credentials.placeholders.pattern')}
              fullWidth
              disabled={!crud.isNew}
            />
            <Select
              label={t('credentials.labels.type')}
              value={crud.editingItem.type}
              options={typeOptions}
              onChange={(e) => crud.updateField('type', e.target.value)}
              fullWidth
            />

            {(crud.editingItem.type === 'bearer' || crud.editingItem.type === 'oauth2' || crud.editingItem.type === 'secret') && (
              <Input
                label="Token"
                type="password"
                value={crud.editingItem.token || ''}
                onChange={(e) => crud.updateField('token', e.target.value)}
                placeholder={t('credentials.placeholders.token')}
                fullWidth
              />
            )}

            {crud.editingItem.type === 'basic' && (
              <div className="credentials-page__row">
                <Input
                  label={t('credentials.labels.username')}
                  value={crud.editingItem.username || ''}
                  onChange={(e) => crud.updateField('username', e.target.value)}
                  fullWidth
                />
                <Input
                  label={t('credentials.labels.password')}
                  type="password"
                  value={crud.editingItem.password || ''}
                  onChange={(e) => crud.updateField('password', e.target.value)}
                  fullWidth
                />
              </div>
            )}

            {crud.editingItem.type === 'custom' && (
              <div className="credentials-page__row">
                <Input
                  label={t('credentials.labels.header')}
                  value={crud.editingItem.headerName || ''}
                  onChange={(e) => crud.updateField('headerName', e.target.value)}
                  fullWidth
                />
                <Input
                  label={t('credentials.labels.value')}
                  type="password"
                  value={crud.editingItem.headerValue || ''}
                  onChange={(e) => crud.updateField('headerValue', e.target.value)}
                  fullWidth
                />
              </div>
            )}

            <p className="credentials-page__hint">
              {t('credentials.hint.sensitive')}
            </p>
          </div>
        )}
        <EditorPanelFooter>
          {!crud.isNew && crud.editingItem && (
            <Button
              variant="danger"
              onClick={() => {
                if (crud.editingItem) {
                  void crud.deleteItem(crud.editingItem);
                }
              }}
            >
              {t('credentials.buttons.delete')}
            </Button>
          )}
          <Button variant="ghost" onClick={crud.closeEditor}>
            {t('common.cancel')}
          </Button>
          <Button onClick={crud.save} loading={crud.saving}>
            {crud.isNew ? t('credentials.buttons.create') : t('common.save')}
          </Button>
        </EditorPanelFooter>
      </Modal>

      <Modal
        isOpen={Boolean(viewingManaged)}
        onClose={() => setViewingManaged(null)}
        title={t('credentials.modal.viewTitle', 'Credencial do sistema')}
        size="md"
      >
        {viewingManaged && (
          <div className="credentials-page__fields">
            <div className="credentials-page__managed-info">
              <p className="credentials-page__managed-badge">
                {t('credentials.managed.badge', 'Gerenciada pelo sistema')}
              </p>
              <p className="credentials-page__managed-desc">
                {t('credentials.managed.description', 'Esta credencial é gerenciada automaticamente pelo Assistente (ex: OAuth MCP). Não pode ser editada ou removida manualmente.')}
              </p>
            </div>

            <Input
              label={t('credentials.labels.pattern')}
              value={viewingManaged.pattern}
              onChange={() => {}}
              readOnly
              fullWidth
              disabled
            />
            <Input
              label={t('credentials.labels.type')}
              value={viewingManaged.type}
              onChange={() => {}}
              readOnly
              fullWidth
              disabled
            />
            <Input
              label={t('credentials.labels.value')}
              value={viewingManaged.masked}
              onChange={() => {}}
              readOnly
              fullWidth
              disabled
            />
          </div>
        )}
        <EditorPanelFooter>
          <Button variant="ghost" onClick={() => setViewingManaged(null)}>
            {t('common.close', 'Fechar')}
          </Button>
        </EditorPanelFooter>
      </Modal>
    </div>
  );
}
