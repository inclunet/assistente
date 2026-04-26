import { useCallback, useEffect, useMemo, useState } from 'react';
import { Trans, useTranslation } from 'react-i18next';
import { CopyOutlined, DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import {
  GetSkills,
  GetSkill,
  CreateSkill,
  UpdateSkill,
  DeleteSkill,
  GetSkillSearchPaths,
  DuplicateSkill,
} from '@wailsjs/go/app/App';
import { skills, main } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, PageLoading } from '../components';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { SkillGeneralSection } from '../components/skills/SkillGeneralSection';
import { SkillContentSection } from '../components/skills/SkillContentSection';
import { SkillToolsSection } from '../components/skills/SkillToolsSection';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useEditableList } from '../hooks/useEditableList';
import { useConfirm } from '../hooks/useConfirm';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './SkillsPage.css';

type SkillInfo = skills.SkillInfo;

interface SkillRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  auto: boolean;
  source: string;
  tools: string[];
  // Campos para edição (só preenchidos após loadItem)
  content?: string;
  toolsString?: string;
  [key: string]: unknown;
}

interface SkillFormData {
  name: string;
  description: string;
  auto: boolean;
  disableModelInvocation?: boolean;
  content?: string;
  toolsString?: string;
}

export default function SkillsPage() {
  const { t } = useTranslation();
  const confirm = useConfirm();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'skills-page' });
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const getAllowedTools = (tools?: { allowed?: string[] }) => tools?.allowed || [];

  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);
  const [focusedRow, setFocusedRow] = useState<SkillRow | null>(null);

  // useEditableList hook gerencia todo estado CRUD
  const crud = useEditableList<SkillRow, SkillFormData, SkillFormData>(
    {
      loadItems: async () => {
        const list = await GetSkills();
        return (list || []).map((s: SkillInfo) => ({
          id: s.slug,
          slug: s.slug,
          name: s.name,
          description: s.description || '',
          auto: !s.disableModelInvocation,
          source: s.source,
          tools: getAllowedTools(s.tools as { allowed?: string[] } | undefined),
        }));
      },
      loadItem: async (id) => {
        const skill = await GetSkill(id as string);
        return {
          id: skill.slug,
          slug: skill.slug,
          name: skill.name,
          description: skill.description,
          auto: !skill.disableModelInvocation,
          source: skill.source,
          tools: getAllowedTools(skill.tools as { allowed?: string[] } | undefined),
          content: skill.content,
          toolsString: getAllowedTools(skill.tools as { allowed?: string[] } | undefined).join(', '),
        };
      },
      createItem: async (data) => {
        const toolsList = (data.toolsString || '').split(',').map(s => s.trim()).filter(Boolean);
        const req = main.SkillCreateRequest.createFrom({
          name: data.name.trim(),
          description: data.description.trim(),
          disableModelInvocation: !data.auto,
          tools: toolsList.length > 0 ? { allowed: toolsList } : undefined,
          content: data.content || '',
        });
        return await CreateSkill(req);
      },
      updateItem: async (id, data) => {
        const toolsList = (data.toolsString || '').split(',').map(s => s.trim()).filter(Boolean);
        const req = main.SkillCreateRequest.createFrom({
          name: data.name.trim(),
          description: data.description.trim(),
          disableModelInvocation: !data.auto,
          tools: toolsList.length > 0 ? { allowed: toolsList } : undefined,
          content: data.content || '',
        });
        await UpdateSkill(id as string, req);
      },
      deleteItem: async (id) => {
        await DeleteSkill(id as string);
      },
    },
    {
      entityName: 'Skill',
      messages: {
        loadError: t('skills.loadError', 'Erro ao carregar skills'),
        createSuccess: t('skills.created', 'Skill criado com sucesso!'),
        createError: t('skills.saveError', 'Erro ao criar skill'),
        updateSuccess: t('skills.updated', 'Skill atualizado com sucesso!'),
        updateError: t('skills.saveError', 'Erro ao atualizar skill'),
        deleteSuccess: t('skills.deleted', 'Skill excluído!'),
        deleteError: t('skills.deleteError', 'Erro ao excluir skill'),
      },
      skipBuiltInDeleteConfirm: true,
      canDelete: async (item) => {
        const ok = await confirm({
          title: t('skills.confirmDeleteTitle'),
          message: t('skills.confirmDelete', { name: item.name }),
          confirmText: t('common.delete'),
          cancelText: t('common.cancel'),
          variant: 'danger',
        });
        if (!ok) return t('skills.deleteCancelled');
        return true;
      },
      validate: (item) => {
        if (!item.name.trim()) {
          return t('skills.nameRequired', 'Nome é obrigatório');
        }
        if (!item.description.trim()) {
          return t('skills.descriptionRequired', 'Descrição é obrigatória');
        }
        return null;
      },
      createDefault: () => ({
        id: '',
        slug: '',
        name: '',
        description: '',
        auto: false,
        source: 'workdir',
        tools: [],
        content: '',
        toolsString: '',
      }),
    }
  );

  useEffect(() => {
    crud.loadItems();
    GetSkillSearchPaths().then(paths => setSearchPaths(paths || []));
  }, []);

  useResourceEditRequest('skills', {
    onEdit: (slug) => crud.openEdit({ id: slug, slug } as SkillRow),
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

  // --- Grid columns ---

  const columns: DataGridColumn<SkillRow>[] = [
    {
      key: 'name',
      label: t('skills.colName', 'Nome'),
      width: '25%',
    },
    {
      key: 'description',
      label: t('skills.colDescription', 'Descrição'),
      width: '35%',
      truncate: true,
    },
    {
      key: 'auto',
      label: t('skills.colMode', 'Modo'),
      width: '12%',
      format: (value) =>
        Boolean(value) ? (
          <span className="skills-badge skills-badge--auto">
            {t('skills.auto', 'Auto')}
          </span>
        ) : (
          <span className="skills-badge skills-badge--manual">
            {t('skills.manual', 'Manual')}
          </span>
        ),
    },
    {
      key: 'source',
      label: t('skills.colSource', 'Origem'),
      width: '13%',
      format: (value) => {
        switch (String(value || '')) {
          case 'workdir':
            return t('skills.sourceWorkdir', 'Projeto');
          case 'home':
            return t('skills.sourceHome', 'Global');
          case 'exe':
            return t('skills.sourceExe', 'Embutido');
          default:
            return String(value || '');
        }
      },
    },
    {
      key: 'actions',
      label: '',
      width: '10%',
      format: (_val, row) => (
        <MenuButton
          items={getRowActions(row)}
          buttonLabel={t('skills.actions.actions', 'Ações')}
        />
      ),
    },
  ];

  function getRowActions(row: SkillRow) {
    return [
      {
        id: 'edit',
        label: t('skills.edit', 'Editar skill'),
        icon: <EditOutlined />,
        onClick: () => crud.openEdit(row),
      },
      {
        id: 'duplicate',
        label: t('skills.duplicate', 'Duplicar'),
        icon: <CopyOutlined />,
        onClick: () => handleDuplicateSkill(row),
      },
      {
        id: 'delete',
        label: t('skills.delete', 'Excluir skill'),
        icon: <DeleteOutlined />,
        onClick: () => crud.deleteItem(row),
        danger: true,
      },
    ];
  }

  const handleDuplicateSkill = async (row: SkillRow) => {
    try {
      const newSlug = await DuplicateSkill(row.slug);
      const successMessage = t('skills.duplicated', 'Skill duplicado!');
      addToast(successMessage, 'success');
      announce(successMessage);
      await crud.loadItems();
      await crud.openEdit({ id: newSlug, slug: newSlug, name: row.name } as SkillRow);
    } catch (error: unknown) {
      addToast(
        getErrorMessage(error) || t('skills.duplicateError', 'Erro ao duplicar skill'),
        'error'
      );
    }
  };

  // Removido: handleCellAction (ações agora via MenuButton/getRowActions)

  // --- Filtering ---

  const filteredRows = useMemo(
    () =>
      crud.items.filter(
        row =>
          row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          row.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
          row.slug.toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [crud.items, searchTerm]
  );

  // --- Stable callbacks for DataGrid ---

  const getRowId = useCallback((item: SkillRow) => item.id, []);
  const handleActivateRow = useCallback((item: SkillRow) => crud.openEdit(item), [crud]);
  const handleDeleteRow = useCallback((item: SkillRow) => crud.deleteItem(item), [crud]);
  const handleFocusChange = useCallback((item: SkillRow | null) => setFocusedRow(item), []);

  // --- Loading ---

  if (crud.loading && crud.items.length === 0) {
    return (
      <div className="skills-page">
        <PageLoading message={t('skills.loading', 'Carregando skills...')} />
      </div>
    );
  }

  // --- Render ---

  const editorTitle = crud.isNew
    ? t('skills.newSkillTitle', 'Novo Skill')
    : crud.editingItem?.name || '';

  return (
    <div className="skills-page">
      <Toolbar
        left={
          <h1 className="page-toolbar__title">{t('skills.pageTitle', 'Skills')}</h1>
        }
        searchPlaceholder={t('skills.search', 'Buscar skills...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'new-skill',
            label: t('skills.newSkill', 'Novo Skill'),
            icon: <PlusOutlined />,
            onClick: crud.openNew,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'edit-skill',
            label: t('skills.edit', 'Editar skill'),
            icon: <EditOutlined />,
            onClick: () => focusedRow && crud.openEdit(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'duplicate-skill',
            label: t('skills.duplicate', 'Duplicar'),
            icon: <CopyOutlined />,
            onClick: () => focusedRow && handleDuplicateSkill(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete-skill',
            label: t('skills.delete', 'Excluir skill'),
            icon: <DeleteOutlined />,
            onClick: () => focusedRow && crud.deleteItem(focusedRow),
            disabled: !focusedRow,
            variant: 'danger',
          },
        ]}
      />

      <DataGrid
        items={filteredRows}
        columns={columns}
        label={t('skills.gridLabel', 'Lista de skills')}
        getItemId={getRowId}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={handleActivateRow}
        onDelete={handleDeleteRow}
        onGridReady={handleGridReady}
        getRowActions={getRowActions}
        onFocusChange={handleFocusChange}
      />

      {/* Editor Modal */}
      <Modal
        isOpen={!!crud.editingItem}
        onClose={crud.closeEditor}
        title={editorTitle}
        size="lg"
      >
        {crud.editingItem && (
          <div className="skills-editor">
            <SkillGeneralSection
              item={crud.editingItem}
              onFieldChange={(field, value) => {
                const nextField = field as keyof SkillFormData;
                crud.updateField(nextField, value as SkillFormData[keyof SkillFormData]);
              }}
            />

            <SkillContentSection
              content={crud.editingItem.content || ''}
              onContentChange={(content) => crud.updateField('content', content)}
            />

            <SkillToolsSection
              toolsString={crud.editingItem.toolsString || ''}
              onToolsChange={(toolsString) => crud.updateField('toolsString', toolsString)}
            />

            <EditorPanelFooter className="skills-editor__footer">
              {crud.editingId && (
                <Button
                  variant="danger"
                  onClick={() => {
                    if (crud.editingItem) crud.deleteItem(crud.editingItem);
                  }}
                  aria-label={t(
                    'skills.deleteBtnLabel',
                    `Excluir skill ${crud.editingItem.name}`
                  )}
                >
                  {t('skills.deleteBtn', 'Excluir')}
                </Button>
              )}
              <Button
                variant="ghost"
                onClick={crud.closeEditor}
                aria-label={t('skills.closeBtnLabel', 'Fechar editor, Escape')}
              >
                {t('skills.closeBtn', 'Fechar')}
              </Button>
              <Button onClick={crud.save} loading={crud.saving}>
                {t('skills.saveBtn', 'Salvar')}
              </Button>
            </EditorPanelFooter>
          </div>
        )}
      </Modal>

      {/* Empty state when no skill is being edited */}
      {!crud.editingItem && crud.items.length > 0 && (
        <div className="skills-empty" role="status">
          <p>
            <Trans
              i18nKey="skills.selectHint"
              defaults="Pressione Enter ou clique <0></0> para editar."
              components={[<EditOutlined key="skills-select-hint-edit" aria-hidden="true" />]}
            />
          </p>
        </div>
      )}

      {/* Empty state when no skills exist */}
      {!crud.editingItem && crud.items.length === 0 && (
        <div className="skills-empty" role="status">
          <p>
            {t(
              'skills.noSkills',
              'Nenhum skill encontrado. Crie arquivos .md em'
            )}{' '}
            <code>
              {searchPaths.length > 0
                ? searchPaths[searchPaths.length - 1]
                : '.assistente/skills/'}
            </code>{' '}
            {t('skills.orUseButton', 'ou use o botão "Novo Skill".')}
          </p>
        </div>
      )}

      {/* Search paths footer */}
      {searchPaths.length > 0 && (
        <div
          className="skills-search-paths"
          role="contentinfo"
          aria-label={t('skills.searchPathsLabel', 'Caminhos de busca de skills')}
        >
          <p className="skills-search-paths__title">
            {t('skills.searchPaths', 'Caminhos de busca:')}
          </p>
          {searchPaths.map((path, i) => (
            <p key={i} className="skills-search-paths__item">
              {path}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}
