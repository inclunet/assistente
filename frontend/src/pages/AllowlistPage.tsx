import { useEffect } from 'react';
import {
  GetAllowlists,
  CreateAllowlist,
  UpdateAllowlist,
  DeleteAllowlist,
} from '@wailsjs/go/main/App';

import { allowlist } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { Modal } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { AllowlistGeneralSection } from '../components/allowlist/AllowlistGeneralSection';
import { AllowlistRulesSection } from '../components/allowlist/AllowlistRulesSection';
import { useGridFocus } from '../hooks/useGridFocus';
import { useEditableList } from '../hooks/useEditableList';
import './AllowlistPage.css';

type AllowlistInfo = allowlist.AllowlistInfo;

interface AllowlistRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  ruleCount: number;
}

export default function AllowlistPage() {
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const crud = useEditableList<AllowlistRow, allowlist.Allowlist, allowlist.Allowlist>(
    {
      loadItems: async () => {
        const list = await GetAllowlists();
        return (list || []).map((a: AllowlistInfo) => ({
          id: a.slug,
          slug: a.slug,
          name: a.name,
          description: a.description || '',
          ruleCount: a.ruleCount,
        }));
      },
      loadItem: async (id) => {
         // Carrega info da lista para obter ruleCount
         const list = await GetAllowlists();
         const info = (list || []).find((a: AllowlistInfo) => a.slug === id);
         return {
           id: id as string,
           slug: id as string,
           name: info?.name || '',
           description: info?.description || '',
           ruleCount: info?.ruleCount || 0,
         } as AllowlistRow;
      },
      createItem: async (data) => {
        return await CreateAllowlist(data);
      },
      updateItem: async (id, data) => {
        await UpdateAllowlist(id as string, data);
      },
      deleteItem: async (id) => {
        await DeleteAllowlist(id as string);
      },
    },
    {
      entityName: 'Allowlist',
      messages: {
        loadError: 'Erro ao carregar allowlists',
        createSuccess: 'Allowlist criada!',
        updateSuccess: 'Allowlist atualizada!',
        deleteSuccess: 'Allowlist excluída!',
        deleteConfirm: (item) => {
          const name = (item as any).name || (item as any).slug || 'Allowlist';
          return `Tem certeza que deseja excluir a allowlist "${name}"?`;
        },
      },
      createDefault: () => ({
        id: '',
        slug: '',
        name: '',
        description: '',
        ruleCount: 0,
      }),
      validate: (item) => {
        const asAllowlist = item as any;
        if (!asAllowlist.name || !asAllowlist.name.trim()) {
          return 'Nome é obrigatório';
        }
        return null;
      },
    }
  );

  useEffect(() => {
    crud.loadItems();
  }, []);

  const handleEdit = async (row: AllowlistRow) => {
    await crud.openEdit(row);
  };

  const updateRules = (field: 'auto_approve' | 'always_deny', rules: string[]) => {
    if (!crud.editingItem) return;
    crud.updateField(field as any, rules);
  };

  const columns: DataGridColumn<AllowlistRow>[] = [
    { key: 'name', label: 'Nome', format: (_val, row) => row.name },
    { key: 'description', label: 'Descrição' },
    { key: 'ruleCount', label: 'Regras', format: (val) => `${val}` },
  ];

  return (
    <div className="allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">Allowlists de Comandos</h1>}
        ariaLabel="Barra de ferramentas de allowlists"
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new',
            label: 'Nova Allowlist',
            icon: '+',
            onClick: crud.openNew,
          },
        ]}
      />

      <div className="allowlist-page__content">
        <DataGrid
          columns={columns}
          items={crud.items}
          getItemId={(row) => row.id}
          onActivate={(row) => handleEdit(row)}
          onDelete={(row) => crud.deleteItem(row as any)}
          label="Allowlists de Comandos"
          onGridReady={handleGridReady}
        />
      </div>

      <Modal
        isOpen={!!crud.editingItem}
        onClose={crud.closeEditor}
        title={crud.isNew ? 'Nova Allowlist' : `Editando: ${(crud.editingItem as any)?.name || ''}`}
        size="lg"
      >
        {crud.editingItem && (
          <div className="allowlist-editor">
            <AllowlistGeneralSection
              item={crud.editingItem as any}
              onFieldChange={(field, value) => crud.updateField(field as any, value)}
            />

            <AllowlistRulesSection
              item={crud.editingItem as any}
              onRulesChange={updateRules}
            />

            <EditorPanelFooter>
              {!crud.isNew && crud.editingId && (
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
                Salvar
              </Button>
            </EditorPanelFooter>
          </div>
        )}
      </Modal>
    </div>
  );
}
