import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  GetAllowlists,
  GetAllowlist,
  CreateAllowlist,
  UpdateAllowlist,
  DeleteAllowlist,
} from '@wailsjs/go/app/App';

import { allowlist } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { AllowlistGeneralSection } from '../components/allowlist/AllowlistGeneralSection';
import { AllowlistRulesSection } from '../components/allowlist/AllowlistRulesSection';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useEditableList } from '../hooks/useEditableList';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './AllowlistPage.css';

type AllowlistInfo = allowlist.AllowlistInfo;

interface AllowlistRow extends allowlist.Allowlist {
  id: string;
  slug: string;
  ruleCount: number;
  [key: string]: unknown;
}

export default function AllowlistPage() {
  const { t } = useTranslation();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'allowlist-page' });
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const [focusedRow, setFocusedRow] = useState<AllowlistRow | null>(null);
  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const crud = useEditableList<AllowlistRow, allowlist.Allowlist, allowlist.Allowlist>(
    {
      loadItems: async () => {
        const list = await GetAllowlists();
        return (list || []).map((a: AllowlistInfo) => ({
          id: a.slug,
          slug: a.slug,
          name: a.name,
          description: a.description || '',
          auto_approve: [],
          always_deny: [],
          command_rules: [],
          default_action: 'confirm',
          ruleCount: a.ruleCount,
        }));
      },
      loadItem: async (id) => {
        const full = await GetAllowlist(String(id));
        return {
          id: id as string,
          slug: id as string,
          name: full?.name || '',
          description: full?.description || '',
          auto_approve: full?.auto_approve || [],
          always_deny: full?.always_deny || [],
          command_rules: full?.command_rules || [],
          default_action: full?.default_action || 'confirm',
          ruleCount:
            (full?.auto_approve || []).length +
            (full?.always_deny || []).length +
            (full?.command_rules || []).length,
        } as AllowlistRow;
      },
      createItem: async (data) => {
        const payload: allowlist.Allowlist = {
          name: data.name,
          description: data.description,
          auto_approve: data.auto_approve || [],
          always_deny: data.always_deny || [],
          command_rules: data.command_rules || [],
          default_action: data.default_action || 'confirm',
        };
        return await CreateAllowlist(payload);
      },
      updateItem: async (id, data) => {
        const payload: allowlist.Allowlist = {
          name: data.name,
          description: data.description,
          auto_approve: data.auto_approve || [],
          always_deny: data.always_deny || [],
          command_rules: data.command_rules || [],
          default_action: data.default_action || 'confirm',
        };
        await UpdateAllowlist(id as string, payload);
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
          const name = item.name || item.slug || 'Allowlist';
          return `Tem certeza que deseja excluir a allowlist "${name}"?`;
        },
      },
      createDefault: () => ({
        id: '',
        slug: '',
        name: '',
        description: '',
        auto_approve: [],
        always_deny: [],
        command_rules: [],
        default_action: 'confirm',
        ruleCount: 0,
      }),
      validate: (item) => {
        if (!item.name || !item.name.trim()) {
          return t('allowlist.error.nameRequired');
        }
        return null;
      },
    }
  );

  useEffect(() => {
    crud.loadItems();
  }, []);

  useResourceEditRequest('allowlists', {
    onEdit: (slug) => crud.openEdit({ id: slug, slug } as AllowlistRow),
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

  const handleEdit = useCallback(
    async (row: AllowlistRow) => {
      await crud.openEdit(row);
    },
    [crud]
  );

  const getRowId = useCallback((row: AllowlistRow) => row.id, []);
  const handleActivateRow = useCallback(
    (row: AllowlistRow) => {
      void handleEdit(row);
    },
    [handleEdit]
  );
  const handleDeleteRow = useCallback(
    (row: AllowlistRow) => {
      void crud.deleteItem(row);
    },
    [crud]
  );
  const handleFocusChange = useCallback((row: AllowlistRow | null) => setFocusedRow(row), []);

  const getDuplicateName = (name: string) => {
    const base = `${name} (Copia)`;
    const existing = new Set(crud.items.map((item) => item.name.toLowerCase()));
    if (!existing.has(base.toLowerCase())) return base;
    let index = 2;
    while (existing.has(`${base} ${index}`.toLowerCase())) {
      index += 1;
    }
    return `${base} ${index}`;
  };

  const handleDuplicate = async (row: AllowlistRow) => {
    try {
      const full = await GetAllowlist(row.slug);
      const name = getDuplicateName(full?.name || row.name || t('allowlist.buttons.new'));
      const payload: allowlist.Allowlist = {
        name,
        description: full?.description || row.description || '',
        auto_approve: full?.auto_approve || [],
        always_deny: full?.always_deny || [],
        default_action: full?.default_action || 'confirm',
      };
      const newSlug = await CreateAllowlist(payload);
        addToast(t('allowlist.toast.duplicated'), 'success');
        announce(t('allowlist.toast.duplicated'));
      await crud.loadItems();
      await crud.openEdit({
        id: newSlug,
        slug: newSlug,
        name: payload.name,
        description: payload.description || '',
        ruleCount: payload.auto_approve.length + payload.always_deny.length,
      } as AllowlistRow);
    } catch (error: unknown) {
        addToast(getErrorMessage(error) || t('allowlist.error.duplicate'), 'error');
    }
  };

  const updateRules = (field: 'auto_approve' | 'always_deny', rules: string[]) => {
    if (!crud.editingItem) return;
    crud.updateField(field, rules);
  };

  const columns: DataGridColumn<AllowlistRow>[] = [
    { key: 'name', label: t('allowlist.columns.name'), format: (_val, row) => row.name },
    { key: 'description', label: t('allowlist.columns.description') },
    { key: 'ruleCount', label: t('allowlist.columns.rules'), format: (val) => `${val}` },
    {
      key: 'actions',
      label: '',
      width: '6%',
      format: (_val, row) => (
        <MenuButton
          items={getAllowlistRowActions(row)}
          buttonLabel={t('allowlist.actions', 'Ações')}
        />
      ),
    },
  ];

  function getAllowlistRowActions(row: AllowlistRow) {
    return [
      {
        id: 'edit',
        label: t('allowlist.buttons.edit', 'Editar'),
        icon: <EditOutlined />,
        onClick: () => handleEdit(row),
      },
      {
        id: 'duplicate',
        label: t('allowlist.buttons.duplicate', 'Duplicar'),
        icon: <CopyOutlined />,
        onClick: () => handleDuplicate(row),
      },
      {
        id: 'delete',
        label: t('allowlist.buttons.delete', 'Excluir'),
        icon: <DeleteOutlined />,
        onClick: () => crud.deleteItem(row),
        danger: true,
      },
    ];
  }

  return (
    <div className="allowlist-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('allowlist.pageTitle')}</h1>}
        ariaLabel={t('allowlist.aria.toolbar')}
        actions={[
          {
            key: 'new',
            label: t('allowlist.buttons.new'),
            icon: <PlusOutlined />,
            onClick: crud.openNew,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'edit',
            label: t('allowlist.buttons.edit', 'Editar'),
            icon: <EditOutlined />,
            onClick: () => focusedRow && handleEdit(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'duplicate',
            label: t('allowlist.buttons.duplicate', 'Duplicar'),
            icon: <CopyOutlined />,
            onClick: () => focusedRow && handleDuplicate(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete',
            label: t('allowlist.buttons.delete', 'Excluir'),
            icon: <DeleteOutlined />,
            onClick: () => focusedRow && crud.deleteItem(focusedRow),
            disabled: !focusedRow,
            variant: 'danger',
          },
        ]}
      />

      <div className="allowlist-page__content">
        <DataGrid
          columns={columns}
          items={crud.items}
          getItemId={getRowId}
          onActivate={handleActivateRow}
          onDelete={handleDeleteRow}
          label={t('allowlist.pageTitle')}
          onGridReady={handleGridReady}
          getRowActions={getAllowlistRowActions}
          onFocusChange={handleFocusChange}
        />
      </div>

      <Modal
        isOpen={!!crud.editingItem}
        onClose={crud.closeEditor}
        title={crud.isNew ? t('allowlist.modal.newTitle') : t('allowlist.modal.editTitle', { name: crud.editingItem?.name || '' })}
        size="lg"
      >
        {crud.editingItem && (
          <div className="allowlist-editor">
            <AllowlistGeneralSection
              item={crud.editingItem}
              onFieldChange={(field, value) =>
                crud.updateField(field as keyof AllowlistRow, value as AllowlistRow[keyof AllowlistRow])
              }
            />

            <AllowlistRulesSection
              item={crud.editingItem}
              onRulesChange={updateRules}
            />

            <EditorPanelFooter>
              {!crud.isNew && crud.editingId && (
                <Button
                  variant="danger"
                  onClick={() => {
                    if (crud.editingItem) {
                      void crud.deleteItem(crud.editingItem);
                    }
                  }}
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
