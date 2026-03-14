import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation();
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
        loadError: t('allowlist.error.load'),
        createSuccess: t('allowlist.toast.created'),
        updateSuccess: t('allowlist.toast.updated'),
        deleteSuccess: t('allowlist.toast.deleted'),
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
          return t('allowlist.error.nameRequired');
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
    { key: 'name', label: t('allowlist.columns.name'), format: (_val, row) => row.name },
    { key: 'description', label: t('allowlist.columns.description') },
    { key: 'ruleCount', label: t('allowlist.columns.rules'), format: (val) => `${val}` },
  ];

  return (
    <div className="allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('allowlist.pageTitle')}</h1>}
        ariaLabel={t('allowlist.aria.toolbar')}
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new',
            label: t('allowlist.buttons.new'),
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
          label={t('allowlist.pageTitle')}
          onGridReady={handleGridReady}
        />
      </div>

      <Modal
        isOpen={!!crud.editingItem}
        onClose={crud.closeEditor}
        title={crud.isNew ? t('allowlist.modal.newTitle') : t('allowlist.modal.editTitle', { name: (crud.editingItem as any)?.name || '' })}
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
                  {t('allowlist.buttons.delete')}
                </Button>
              )}
              <Button variant="ghost" onClick={crud.closeEditor}>
                {t('common.cancel')}
              </Button>
              <Button onClick={crud.save} loading={crud.saving}>
                {t('common.save')}
              </Button>
            </EditorPanelFooter>
          </div>
        )}
      </Modal>
    </div>
  );
}
