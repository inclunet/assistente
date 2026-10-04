import { useCallback, useEffect, useState } from 'react';
import { DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { ListCredentials, UpsertCredential, DeleteCredential } from '@wailsjs/go/wailsapi/Credentials';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, Input } from '../components';
import { Modal } from '../components/ui/Modal';
import { DialogActions } from '../components/ui/DialogActions';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useEditableList } from '../hooks/useEditableList';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { useActivePanelNewShortcut } from '../hooks/useActivePanelShortcut';
import './CredentialsPage.css';
import { CredentialFields } from '../components/credentials/CredentialFields';
import { credentialFromSummary, credentialInput, newCredential, validateCredential, type CredentialDraft as CredentialRow } from '../components/credentials/credentialDraft';

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
          source: c.source,
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
        if (!found || found.managed) throw new Error(t('credentials.sourceFields.loadError'));
        return credentialFromSummary(found);
      },
      createItem: async (data) => {
        await UpsertCredential(credentialInput(data));
        return data.pattern;
      },
      updateItem: async (_id, data) => {
        await UpsertCredential(credentialInput(data));
      },
      deleteItem: async (id) => {
        await DeleteCredential(String(id));
      },
    },
    {
      entityName: t('credentials.sourceFields.entity'),
      messages: {
        loadError: t('credentials.sourceFields.loadError'),
        createSuccess: t('credentials.sourceFields.createSuccess'),
        updateSuccess: t('credentials.sourceFields.updateSuccess'),
        deleteSuccess: t('credentials.sourceFields.deleteSuccess'),
        deleteConfirm: (item) => t('credentials.sourceFields.deleteConfirm', { pattern: item.pattern }),
      },
      createDefault: () => newCredential(),
      validate: (item) => validateCredential(item, t),
    }
  );

  useEffect(() => {
    crud.loadItems();
  }, []);

  useResourceEditRequest('credentials', {
    onEdit: (pattern) => crud.openEdit({ id: pattern, pattern } as CredentialRow),
    onNew: (request) => {
      crud.openNew();
      // Pré-preenchimento via deep link (allowlist: pattern/type; segredos
      // nunca chegam aqui — ver parseResourceNewInitial). updateField é
      // funcional, então acumula sobre o item recém-aberto com segurança.
      const initial = request?.initial;
      if (initial?.pattern) crud.updateField('pattern', initial.pattern);
      if (initial?.type && typeOptions.some((o) => o.value === initial.type)) {
        crud.updateField('type', initial.type);
      }
    },
    // Só o fim do carregamento: exigir lista não-vazia deixaria o deep link
    // morto em instalação nova (zero credenciais) — justo o cenário de
    // primeira configuração.
    ready: !crud.loading,
  });

  useActivePanelNewShortcut(crud.openNew);

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
    { key: 'source', label: t('credentials.sourceFields.source'), width: '120px',
      format: (value) => value ? t(`credentials.sourceFields.${value}`) : t('credentials.sourceFields.unconfigured') },
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
          icon: <EyeOutlined aria-hidden="true" />,
          onClick: () => setViewingManaged(row),
        },
      ];
    }
    return [
      {
        id: 'edit',
        label: t('credentials.buttons.edit', 'Editar'),
        icon: <EditOutlined aria-hidden="true" />,
        onClick: () => crud.openEdit(row),
      },
      {
        id: 'delete',
        label: t('credentials.buttons.delete', 'Excluir'),
        icon: <DeleteOutlined aria-hidden="true" />,
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
            icon: <PlusOutlined aria-hidden="true" />,
            onClick: crud.openNew,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'edit',
            label: t('credentials.buttons.edit', 'Editar'),
            icon: <EditOutlined aria-hidden="true" />,
            onClick: () => focusedRow && !focusedRow.managed && crud.openEdit(focusedRow),
            disabled: !focusedRow || focusedRow.managed,
          },
          {
            key: 'delete',
            label: t('credentials.buttons.delete', 'Excluir'),
            icon: <DeleteOutlined aria-hidden="true" />,
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
        onClose={() => { crud.closeEditor(); }}
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
            <CredentialFields value={crud.editingItem} onChange={crud.updateField} />
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
          <DialogActions
            primary={
              <Button onClick={crud.save} loading={crud.saving} disabled={crud.editingItem?.source === 'oauth'}>
                {crud.isNew ? t('credentials.buttons.create') : t('common.save')}
              </Button>
            }
            secondary={
              <Button variant="ghost" onClick={() => { crud.closeEditor(); }}>
                {t('common.cancel')}
              </Button>
            }
          />
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
