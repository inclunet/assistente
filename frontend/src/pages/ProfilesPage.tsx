import { useState, useEffect, useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  CheckOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import {
  GetProfiles,
  GetProfile,
  GetActiveProfileSlug,
  SetActiveProfile,
  CreateProfile,
  UpdateProfile,
  DeleteProfile,
  DuplicateProfile,
  GetProfileSearchPaths,
} from '@wailsjs/go/app/App';
import { profiles } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button, PageLoading } from '../components';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { ProfileEditorTabs } from '../components/profiles/ProfileEditorTabs';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useEditableList } from '../hooks/useEditableList';
import { useProfileDependencies } from '../hooks/useProfileDependencies';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './ProfilesPage.css';

type ProfileInfo = profiles.ProfileInfo;
type Profile = profiles.Profile;

interface ProfileRow extends Profile {
  id: string; // slug as id
  slug: string;
  source?: string;
  isActive?: boolean;
  [key: string]: unknown;
}

export default function ProfilesPage() {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'profiles-page' });

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  // Grid state
  const [activeSlug, setActiveSlug] = useState<string>('padrao');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);
  const [focusedRow, setFocusedRow] = useState<ProfileRow | null>(null);

  const { tools: availableTools, skills: availableSkills, allowlists: availableAllowlists, contextProviders: availableContextProviders, loading: depsLoading } =
    useProfileDependencies();

  const crud = useEditableList<ProfileRow, Profile, Profile>(
    {
      loadItems: async () => {
        const [allProfiles, currentSlug] = await Promise.all([
          GetProfiles(),
          GetActiveProfileSlug(),
        ]);
        const resolvedSlug = currentSlug || 'padrao';
        setActiveSlug(resolvedSlug);

        return (allProfiles || []).map((p: ProfileInfo) => ({
          id: p.slug,
          slug: p.slug,
          name: p.name,
          description: p.description || '',
          icon: p.icon || '',
          source: p.source,
          isActive: p.slug === resolvedSlug,
        })) as ProfileRow[];
      },
      loadItem: async (id) => {
        const profile = await GetProfile(id as string);
        const row = profiles.Profile.createFrom(profile) as ProfileRow;
        row.id = String(id);
        row.slug = String(id);
        row.source = (profile as { source?: string }).source ?? 'workdir';
        row.isActive = row.slug === activeSlug;
        return row;
      },
      createItem: async (data) => await CreateProfile(data),
      updateItem: async (id, data) => await UpdateProfile(id as string, data),
      deleteItem: async (id) => await DeleteProfile(id as string),
    },
    {
      entityName: t('profiles.entityName', 'Perfil'),
      messages: {
        loadError: t('profiles.loadError', 'Erro ao carregar perfis'),
        createSuccess: t('profiles.created', 'Perfil criado com sucesso!'),
        createError: t('profiles.saveError', 'Erro ao criar perfil'),
        updateSuccess: t('profiles.updated', 'Perfil atualizado com sucesso!'),
        updateError: t('profiles.saveError', 'Erro ao atualizar perfil'),
        deleteSuccess: t('profiles.deleted', 'Perfil excluído!'),
        deleteError: t('profiles.deleteError', 'Erro ao excluir perfil'),
        deleteConfirm: (item) =>
          t('profiles.confirmDelete', `Tem certeza que deseja excluir o perfil "${item.name}"?`),
      },
      validate: (item) => {
        if (!item.name?.trim()) {
          return t('profiles.nameRequired', 'Nome é obrigatório');
        }
        return null;
      },
      canDelete: (item) => {
        if (item.isActive) {
          return t('profiles.cannotDeleteActive', 'Não é possível excluir o perfil ativo');
        }
        return true;
      },
      createDefault: () => {
        const defaultProfile = profiles.Profile.createFrom({
          name: 'Novo Perfil',
          description: '',
          icon: 'chatbox',
          chat: {
            llm_provider: '',
            model: '',
            temperature: 0.7,
            max_tokens: 4096,
            top_p: 1.0,
            response_timeout: 180,
            reasoning_effort: '',
            prompt_cache: {
              enabled: false,
              provider_hints: false,
              explicit_cache_control: false,
            },
            streaming_recovery_enabled: true,
            streaming_recovery_max_attempts: 3,
            streaming_recovery_show_continue: true,
            system_prompt: '',
            system_prompt_position: 'after',
          },
          voice: {
            assistant: {
              enabled: false,
              provider: 'disabled',
              voice_id: '',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
            user: {
              enabled: false,
              provider: 'disabled',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
            system: {
              enabled: false,
              provider: 'disabled',
              rate: 1.0,
              pitch: 1.0,
              volume: 1.0,
            },
          },
          input: {
            enabled: true,
            stt_provider: 'webspeech',
            language: 'pt-BR',
            feedback_sounds: true,
            triggers: [],
          },
          channels: {
            response_mode: 'mirror',
          },
          context_providers: {},
        }) as ProfileRow;
        defaultProfile.id = '';
        defaultProfile.isActive = false;
        defaultProfile.source = 'workdir';
        return defaultProfile;
      },
    }
  );

  useEffect(() => {
    crud.loadItems();
    GetProfileSearchPaths().then((paths) => setSearchPaths(paths || []));
  }, []);

  useResourceEditRequest('profiles', {
    onEdit: (slug) => crud.openEdit({ id: slug, slug } as ProfileRow),
    onNew: () => crud.openNew(),
    ready: !crud.loading && crud.items.length > 0,
  });

  // --- Grid actions ---

  const handleEditProfile = useCallback(async (row: ProfileRow) => {
    await crud.openEdit(row);
  }, [crud]);

  const handleDuplicateProfile = async (row: ProfileRow) => {
    try {
      const newSlug = await DuplicateProfile(row.slug);
      const successMessage = t('profiles.duplicated', 'Perfil duplicado!');
      addToast(successMessage, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(successMessage);
      await crud.loadItems();
      await crud.openEdit({ id: newSlug, slug: newSlug, name: row.name } as ProfileRow);
    } catch (error: unknown) {
      addToast(
        getErrorMessage(error) || t('profiles.duplicateError', 'Erro ao duplicar perfil'),
        'error'
      );
    }
  };

  const handleActivateProfile = async (row: ProfileRow) => {
    try {
      await SetActiveProfile(row.slug);
      addToast(t('profiles.activated', `Perfil "${row.name}" ativado!`), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('profiles.activatedAnnounce', `Perfil ${row.name} ativado`));
      await crud.loadItems();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('profiles.activateError', 'Erro ao ativar perfil'), 'error');
    }
  };

  const handleNewProfile = () => {
    crud.openNew();
  };

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
      handleNewProfile();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [handleNewProfile]);

  const handleSave = async () => {
    await crud.save();
  };

  const handleCloseEditor = () => {
    crud.closeEditor();
  };

  const updateFields = (updates: Record<string, unknown>) => {
    if (!crud.editingItem) return;
    const updated = JSON.parse(JSON.stringify(crud.editingItem));

    const setDeepValue = (target: Record<string, unknown>, path: string, value: unknown) => {
      const keys = path.split('.');
      let obj = target as Record<string, unknown>;
      for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        const next = obj[key];
        if (!next || typeof next !== 'object') {
          obj[key] = {};
        }
        obj = obj[key] as Record<string, unknown>;
      }
      obj[keys[keys.length - 1]] = value;
    };

    for (const [path, value] of Object.entries(updates)) {
      setDeepValue(updated, path, value);
    }

    const next = profiles.Profile.createFrom(updated) as ProfileRow;
    next.id = crud.editingItem.id || next.slug || String(crud.editingId || '');
    next.source = crud.editingItem.source;
    next.isActive = crud.editingItem.isActive;
    crud.setEditingItem(next);
  };

  const updateField = (path: string, value: unknown) => {
    updateFields({ [path]: value });
  };

  // --- Grid columns ---

  const columns: DataGridColumn<ProfileRow>[] = [
    {
      key: 'name',
      label: t('profiles.colName', 'Nome'),
      width: '22%',
      editable: true,
    },
    {
      key: 'description',
      label: t('profiles.colDescription', 'Descrição'),
      width: '28%',
      truncate: true,
    },
    {
      key: 'source',
      label: t('profiles.colSource', 'Origem'),
      width: '12%',
      format: (value) => {
        switch (String(value || '')) {
          case 'workdir':
            return t('profiles.sourceWorkdir', 'Projeto');
          case 'home':
            return t('profiles.sourceHome', 'Global');
          case 'exe':
            return t('profiles.sourceExe', 'Embutido');
          default:
            return String(value || '-');
        }
      },
    },
    {
      key: 'isActive',
      label: t('profiles.colStatus', 'Status'),
      width: '10%',
      format: (value) => {
        const isActive = Boolean(value);
        return (
          <span className={isActive ? 'profiles-badge profiles-badge--active' : 'profiles-badge profiles-badge--inactive'}>
            {isActive ? t('profiles.active', 'Ativo') : t('profiles.inactive', 'Inativo')}
          </span>
        );
      },
    },
    {
      key: 'actions',
      label: '',
      width: '8%',
      format: (_value, item) => (
        <MenuButton
          items={getProfileRowActions(item)}
          buttonLabel={t('profiles.actions', 'Ações')}
        />
      ),
    },
  ];


  // Gera as ações contextuais para cada linha
  function getProfileRowActions(item: ProfileRow) {
    return [
      {
        id: 'activate',
        label: t('profiles.activate', 'Ativar perfil'),
        icon: <CheckOutlined />,
        onClick: () => handleActivateProfile(item),
        disabled: !!item.isActive,
      },
      {
        id: 'edit',
        label: t('profiles.edit', 'Editar perfil'),
        icon: <EditOutlined />,
        onClick: () => handleEditProfile(item),
      },
      {
        id: 'duplicate',
        label: t('profiles.duplicate', 'Duplicar'),
        icon: <CopyOutlined />,
        onClick: () => handleDuplicateProfile(item),
      },
      {
        id: 'delete',
        label: t('profiles.delete', 'Excluir perfil'),
        icon: <DeleteOutlined />,
        onClick: () => crud.deleteItem(item),
        danger: true,
        disabled: !!item.isActive,
      },
    ];
  }

  const handleCellEdit = async (item: ProfileRow, column: DataGridColumn<ProfileRow>, newValue: string) => {
    if (column.key === 'name') {
      try {
        const profile = await GetProfile(item.slug);
        profile.name = newValue;
        await UpdateProfile(item.slug, profile);
        if (crud.editingId === item.slug && crud.editingItem) {
          updateField('name', newValue);
        }
        await crud.loadItems();
      } catch {
        addToast(t('profiles.renameError', 'Erro ao renomear perfil'), 'error');
      }
    }
  };

  // --- Filtering ---

  const filteredRows = useMemo(
    () =>
      crud.items.filter(
        (row) =>
          row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
          (row.description || '').toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [crud.items, searchTerm]
  );

  const getItemId = useCallback((item: ProfileRow) => item.id, []);
  const handleActivateRow = useCallback(
    (item: ProfileRow) => handleEditProfile(item),
    [handleEditProfile]
  );
  const handleDeleteRow = useCallback(
    (item: ProfileRow) => crud.deleteItem(item),
    [crud]
  );
  const handleFocusChange = useCallback((item: ProfileRow | null) => setFocusedRow(item), []);

  // --- Loading ---

  const loading = crud.loading || depsLoading;

  if (loading) {
    return (
      <div className="profiles-page">
        <PageLoading message={t('profiles.loading', 'Carregando perfis...')} />
      </div>
    );
  }

  // --- Render ---

  const editingProfile = crud.editingItem;
  const editingSlug = crud.editingId ? String(crud.editingId) : null;
  const isNew = crud.isNew;
  const saving = crud.saving;

  const editorTitle = isNew
    ? t('profiles.newProfileTitle', 'Novo Perfil')
    : editingProfile?.name || '';

  return (
    <div className="profiles-page">
      <Toolbar
        left={
          <h1 className="page-toolbar__title">
            {t('profiles.pageTitle', 'Perfis')}
          </h1>
        }
        searchPlaceholder={t('profiles.search', 'Buscar perfis...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        actions={[
          {
            key: 'new-profile',
            label: t('profiles.newProfile', 'Novo Perfil'),
            icon: <PlusOutlined />,
            onClick: handleNewProfile,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'activate-profile',
            label: t('profiles.activate', 'Ativar perfil'),
            icon: <CheckOutlined />,
            onClick: () => focusedRow && handleActivateProfile(focusedRow),
            disabled: !focusedRow || !!focusedRow?.isActive,
          },
          {
            key: 'edit-profile',
            label: t('profiles.edit', 'Editar perfil'),
            icon: <EditOutlined />,
            onClick: () => focusedRow && handleEditProfile(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'duplicate-profile',
            label: t('profiles.duplicate', 'Duplicar'),
            icon: <CopyOutlined />,
            onClick: () => focusedRow && handleDuplicateProfile(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete-profile',
            label: t('profiles.delete', 'Excluir perfil'),
            icon: <DeleteOutlined />,
            onClick: () => focusedRow && crud.deleteItem(focusedRow),
            disabled: !focusedRow || !!focusedRow?.isActive,
            variant: 'danger',
          },
        ]}
      />

      <DataGrid
        items={filteredRows}
        columns={columns}
        label={t('profiles.gridLabel', 'Lista de perfis')}
        getItemId={getItemId}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={handleActivateRow}
        onDelete={handleDeleteRow}
        onCellEdit={handleCellEdit}
        onGridReady={handleGridReady}
        getRowActions={getProfileRowActions}
        onFocusChange={handleFocusChange}
      />

      {/* Editor Modal */}
      <Modal
        isOpen={!!editingProfile}
        onClose={handleCloseEditor}
        title={editorTitle}
        size="xl"
      >
        {editingProfile && (
          <div className="profiles-editor">
            <ProfileEditorTabs
              editingProfile={editingProfile}
              availableTools={availableTools}
              availableSkills={availableSkills}
              availableContextProviders={availableContextProviders}
              availableAllowlists={availableAllowlists}
              updateField={updateField}
              updateFields={updateFields}
            />

            <EditorPanelFooter className="profiles-editor__footer">
              <Button onClick={handleSave} loading={saving}>
                {t('profiles.saveBtn', 'Salvar')}
              </Button>
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="secondary"
                  onClick={() => handleActivateProfile({ slug: editingSlug, name: editingProfile.name } as ProfileRow)}
                  aria-label={t('profiles.activateBtnLabel', `Ativar perfil ${editingProfile.name}`)}
                >
                  {t('profiles.activateBtn', 'Ativar')}
                </Button>
              )}
              <div className="profiles-editor__footer-spacer" />
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => {
                    const deleteTarget = profiles.Profile.createFrom(editingProfile) as ProfileRow;
                    deleteTarget.id = editingSlug;
                    deleteTarget.slug = editingSlug;
                    deleteTarget.isActive = activeSlug === editingSlug;
                    deleteTarget.source = (editingProfile as ProfileRow).source;
                    crud.deleteItem(deleteTarget);
                  }}
                  aria-label={t('profiles.deleteBtnLabel', `Excluir perfil ${editingProfile.name}`)}
                >
                  {t('profiles.deleteBtn', 'Excluir')}
                </Button>
              )}
            </EditorPanelFooter>
          </div>
        )}
      </Modal>

      {/* Empty state when no profile is being edited */}
      {!editingProfile && crud.items.length > 0 && (
        <div className="profiles-empty" role="status">
          <p>
            {t('profiles.selectHint', 'Pressione Enter ou clique para editar.')}
            {' '}
            <EditOutlined aria-hidden="true" />
          </p>
        </div>
      )}

      {/* Search paths footer */}
      {searchPaths.length > 0 && (
        <div className="profiles-search-paths" role="contentinfo" aria-label={t('profiles.searchPathsLabel', 'Caminhos de busca de perfis')}>
          <p className="profiles-search-paths__title">
            {t('profiles.searchPaths', 'Caminhos de busca:')}
          </p>
          {searchPaths.map((path, i) => (
            <p key={i} className="profiles-search-paths__item">{path}</p>
          ))}
        </div>
      )}
    </div>
  );
}
