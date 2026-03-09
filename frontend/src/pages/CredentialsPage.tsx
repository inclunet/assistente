import { useEffect } from 'react';
import { ListCredentials, UpsertCredential, DeleteCredential } from '@wailsjs/go/main/App';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, Input, Select } from '../components';
import { Modal } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { useGridFocus } from '../hooks/useGridFocus';
import { useEditableList } from '../hooks/useEditableList';
import './CredentialsPage.css';

interface CredentialRow {
  id: string;
  pattern: string;
  type: string;
  masked: string;
  token?: string;
  username?: string;
  password?: string;
  headerName?: string;
  headerValue?: string;
}

const typeOptions = [
  { value: 'bearer', label: 'Bearer token' },
  { value: 'basic', label: 'Basic (usuário/senha)' },
  { value: 'custom', label: 'Header customizado' },
  { value: 'secret', label: 'Segredo (uso interno)' },
];

export default function CredentialsPage() {
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const crud = useEditableList<CredentialRow, CredentialRow, CredentialRow>(
    {
      loadItems: async () => {
        const list = await ListCredentials();
        return (list || []).map((c) => ({
          id: c.pattern,
          pattern: c.pattern,
          type: c.type,
          masked: c.masked || '',
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

  const columns: DataGridColumn<CredentialRow>[] = [
    { key: 'pattern', label: 'Pattern', width: '260px', truncate: true },
    { key: 'type', label: 'Tipo', width: '120px' },
    { key: 'masked', label: 'Valor', truncate: true },
  ];

  return (
    <div className="credentials-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">Credenciais</h1>}
        right={<Button onClick={crud.openNew} variant="primary">Nova</Button>}
        ariaLabel="Barra de ferramentas de credenciais"
        onFocusGrid={focusFirstCell}
      />

      <div className="credentials-page__content">
        <DataGrid
          columns={columns}
          items={crud.items}
          getItemId={(row) => row.id}
          onActivate={(row) => crud.openEdit(row)}
          onDelete={(row) => crud.deleteItem(row as any)}
          label="Credenciais"
          onGridReady={handleGridReady}
        />
      </div>

      <Modal
        isOpen={Boolean(crud.editingItem)}
        onClose={crud.closeEditor}
        title={crud.isNew ? 'Nova credencial' : 'Editar credencial'}
        size="md"
      >
        {crud.editingItem && (
          <div className="credentials-page__fields">
            <Input
              label="Pattern"
              value={crud.editingItem.pattern}
              onChange={(e) => crud.updateField('pattern', e.target.value)}
              placeholder="ex: *.github.com ou channel:slack:bot_token"
              fullWidth
              disabled={!crud.isNew}
            />
            <Select
              label="Tipo"
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
                placeholder="Informe o token"
                fullWidth
              />
            )}

            {crud.editingItem.type === 'basic' && (
              <div className="credentials-page__row">
                <Input
                  label="Usuário"
                  value={crud.editingItem.username || ''}
                  onChange={(e) => crud.updateField('username', e.target.value)}
                  fullWidth
                />
                <Input
                  label="Senha"
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
                  label="Header"
                  value={crud.editingItem.headerName || ''}
                  onChange={(e) => crud.updateField('headerName', e.target.value)}
                  fullWidth
                />
                <Input
                  label="Valor"
                  type="password"
                  value={crud.editingItem.headerValue || ''}
                  onChange={(e) => crud.updateField('headerValue', e.target.value)}
                  fullWidth
                />
              </div>
            )}

            <p className="credentials-page__hint">
              Os valores sensíveis não são exibidos após salvar. Para atualizar, informe novamente.
            </p>
          </div>
        )}
        <EditorPanelFooter>
          {!crud.isNew && crud.editingItem && (
            <Button
              variant="danger"
              onClick={() => crud.deleteItem(crud.editingItem as any)}
            >
              Excluir
            </Button>
          )}
          <Button variant="ghost" onClick={crud.closeEditor}>
            Cancelar
          </Button>
          <Button onClick={crud.save} loading={crud.saving}>
            {crud.isNew ? 'Criar' : 'Salvar'}
          </Button>
        </EditorPanelFooter>
      </Modal>
    </div>
  );
}
