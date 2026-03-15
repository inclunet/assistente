import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
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
} from '@wailsjs/go/main/App';
import { profiles } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { CollapsibleSection } from '../components/ui/CollapsibleSection';
import { ProfileGeneralSection } from '../components/profiles/ProfileGeneralSection';
import { ProfileChatSection } from '../components/profiles/ProfileChatSection';
import { ProfileSkillsSection } from '../components/profiles/ProfileSkillsSection';
import { ProfileToolsSection } from '../components/profiles/ProfileToolsSection';
import { ProfileVoiceSection } from '../components/profiles/ProfileVoiceSection';
import { ProfileInteractionSection } from '../components/profiles/ProfileInteractionSection';
import { VOICE_DISABLED } from '../components/pickers/VoicePicker';
import { useGridFocus } from '../hooks/useGridFocus';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import { useEditableList } from '../hooks/useEditableList';
import { useProfileDependencies } from '../hooks/useProfileDependencies';
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
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  // Grid state
  const [activeSlug, setActiveSlug] = useState<string>('padrao');
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);
  const [focusedRow, setFocusedRow] = useState<ProfileRow | null>(null);

  const { tools: availableTools, skills: availableSkills, allowlists: availableAllowlists, loading: depsLoading } =
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
            system_prompt: '',
            system_prompt_position: 'after',
          },
          voice: {
            provider: 'disabled',
            voice_id: '',
            rate: 1.0,
            pitch: 1.0,
            volume: 1.0,
            enabled_for_agent: false,
            enabled_for_user: false,
          },
          interaction: {
            stt_provider: 'webspeech',
            language: 'pt-BR',
            feedback_sounds: true,
            triggers: [],
          },
        }) as ProfileRow;
        defaultProfile.id = '';
        defaultProfile.isActive = false;
        defaultProfile.source = 'workdir';
        return defaultProfile;
      },
      onSuccess: () => {
        setTimeout(() => focusFirstCell?.(), 50);
      },
    }
  );

  useEffect(() => {
    crud.loadItems();
    GetProfileSearchPaths().then((paths) => setSearchPaths(paths || []));
  }, []);

  // --- Grid actions ---

  const handleEditProfile = async (row: ProfileRow) => {
    await crud.openEdit(row);
  };

  const handleDuplicateProfile = async (row: ProfileRow) => {
    try {
      const newSlug = await DuplicateProfile(row.slug);
      const successMessage = t('profiles.duplicated', 'Perfil duplicado!');
      addToast(successMessage, 'success');
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
      addToast(t('profiles.activated', `Perfil "${row.name}" ativado!`), 'success');
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
    
    // Aplica todas as atualizações
    for (const [path, value] of Object.entries(updates)) {
      const keys = path.split('.');
      let obj = updated;
      for (let i = 0; i < keys.length - 1; i++) {
        obj = obj[keys[i]];
      }
      obj[keys[keys.length - 1]] = value;
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
        icon: '✅',
        onClick: () => handleActivateProfile(item),
        disabled: !!item.isActive,
      },
      {
        id: 'edit',
        label: t('profiles.edit', 'Editar perfil'),
        icon: '✏️',
        onClick: () => handleEditProfile(item),
      },
      {
        id: 'duplicate',
        label: t('profiles.duplicate', 'Duplicar'),
        icon: '📄',
        onClick: () => handleDuplicateProfile(item),
      },
      {
        id: 'delete',
        label: t('profiles.delete', 'Excluir perfil'),
        icon: '🗑️',
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

  const filteredRows = crud.items.filter(row =>
    row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    (row.description || '').toLowerCase().includes(searchTerm.toLowerCase())
  );

  // --- Loading ---

  const loading = crud.loading || depsLoading;

  if (loading) {
    return (
      <div className="profiles-page">
        <div className="loading" role="status">{t('profiles.loading', 'Carregando perfis...')}</div>
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
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new-profile',
            label: t('profiles.newProfile', 'Novo Perfil'),
            icon: '➕',
            onClick: handleNewProfile,
            shortcut: 'Ctrl+N',
            variant: 'primary',
          },
          {
            key: 'activate-profile',
            label: t('profiles.activate', 'Ativar perfil'),
            icon: '✅',
            onClick: () => focusedRow && handleActivateProfile(focusedRow),
            disabled: !focusedRow || !!focusedRow?.isActive,
          },
          {
            key: 'edit-profile',
            label: t('profiles.edit', 'Editar perfil'),
            icon: '✏️',
            onClick: () => focusedRow && handleEditProfile(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'duplicate-profile',
            label: t('profiles.duplicate', 'Duplicar'),
            icon: '📄',
            onClick: () => focusedRow && handleDuplicateProfile(focusedRow),
            disabled: !focusedRow,
          },
          {
            key: 'delete-profile',
            label: t('profiles.delete', 'Excluir perfil'),
            icon: '🗑️',
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
        getItemId={(item) => item.id}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={(item) => handleEditProfile(item)}
        onDelete={(item) => crud.deleteItem(item)}
        onCellEdit={handleCellEdit}
        onGridReady={handleGridReady}
        getRowActions={getProfileRowActions}
        onFocusChange={(item) => setFocusedRow(item as ProfileRow | null)}
      />

      {/* Editor Modal */}
      <Modal
        isOpen={!!editingProfile}
        onClose={handleCloseEditor}
        title={editorTitle}
        size="xl"
      >
        {editingProfile && (
          <div className="profiles-editor" aria-live="polite">
            {/* General Section */}
            <section className="profiles-section" aria-labelledby="section-general">
              <h3 id="section-general">{t('profiles.sectionGeneral', 'Geral')}</h3>
              <ProfileGeneralSection
                name={editingProfile.name}
                description={editingProfile.description || ''}
                icon={editingProfile.icon || ''}
                onChange={(field, value) => updateField(field, value)}
              />
            </section>

            {/* Chat Section */}
            <section className="profiles-section" aria-labelledby="section-chat">
              <h3 id="section-chat">{t('profiles.sectionChat', 'Chat (LLM)')}</h3>
              <ProfileChatSection
                llmProvider={editingProfile.chat?.llm_provider || ''}
                model={editingProfile.chat?.model || ''}
                temperature={editingProfile.chat?.temperature ?? 0.7}
                maxTokens={editingProfile.chat?.max_tokens ?? 4096}
                maxTokensMode={editingProfile.chat?.max_tokens_mode || 'legacy'}
                contextWindow={editingProfile.chat?.context_window ?? 0}
                maxContextMessages={editingProfile.chat?.max_context_messages ?? 0}
                minContextMessages={editingProfile.chat?.min_context_messages ?? 0}
                topP={editingProfile.chat?.top_p ?? 1.0}
                responseTimeout={editingProfile.chat?.response_timeout ?? 180}
                reasoningEffort={editingProfile.chat?.reasoning_effort || ''}
                onChange={(field, value) => updateField(`chat.${field}`, value)}
                onMultiChange={(updates) => {
                  const prefixedUpdates = Object.fromEntries(
                    Object.entries(updates).map(([k, v]) => [`chat.${k}`, v])
                  );
                  updateFields(prefixedUpdates);
                }}
              />
              <ProfileSkillsSection
                availableSkills={availableSkills}
                enabledSkills={editingProfile.chat?.enabled_skills || []}
                disableOnDemand={editingProfile.chat?.disable_on_demand_skills ?? false}
                skillsDisabled={editingProfile.chat?.disable_skills ?? false}
                onChange={(field, value) => updateField(`chat.${field}`, value)}
              />
              <ProfileToolsSection
                availableTools={availableTools}
                enabledTools={editingProfile.chat?.enabled_tools ?? null}
                toolsDisabled={editingProfile.chat?.disable_tools ?? false}
                commandAllowlist={editingProfile.chat?.command_allowlist || ''}
                availableAllowlists={availableAllowlists}
                onChange={(field, value) => updateField(`chat.${field}`, value)}
              />
            </section>

          {/* Voice (TTS) — colapsável */}
          {(() => {
            const isVoiceDisabled = editingProfile.voice?.provider === 'disabled';
            return (
              <CollapsibleSection
                title={t('profiles.collapseVoice', 'Voz (TTS)')}
                isOpen={!isVoiceDisabled}
                onToggle={() => {
                  if (isVoiceDisabled) {
                    updateField('voice.provider', 'webspeech');
                  } else {
                    updateField('voice.provider', 'disabled');
                    updateField('voice.voice_id', '');
                  }
                }}
                badge={isVoiceDisabled ? 'off' : 'on'}
              >
                <ProfileVoiceSection
                  voice={isVoiceDisabled ? VOICE_DISABLED : editingProfile.voice?.voice_id || ''}
                  rate={editingProfile.voice?.rate ?? 1.0}
                  volume={editingProfile.voice?.volume ?? 1.0}
                  onChange={(field, value) => {
                    if (field === 'voice') {
                      if (value === VOICE_DISABLED) {
                        updateField('voice.provider', 'disabled');
                        updateField('voice.voice_id', '');
                        return;
                      }
                      updateField('voice.voice_id', value);
                      if (!editingProfile.voice?.provider || editingProfile.voice.provider === 'disabled') {
                        updateField('voice.provider', 'webspeech');
                      }
                      return;
                    }
                    updateField(`voice.${field}`, value);
                  }}
                />
                <div className="profiles-fields">
                  <div className="profiles-field profiles-field--checkbox">
                    <input
                      id="pf-tts-agent"
                      type="checkbox"
                      checked={editingProfile.voice?.enabled_for_agent ?? false}
                      onChange={(e) => updateField('voice.enabled_for_agent', e.target.checked)}
                    />
                    <label htmlFor="pf-tts-agent" className="profiles-field__label">
                      {t('profiles.fieldTTSAgent', 'TTS para mensagens do assistente')}
                    </label>
                  </div>
                  <div className="profiles-field profiles-field--checkbox">
                    <input
                      id="pf-tts-user"
                      type="checkbox"
                      checked={editingProfile.voice?.enabled_for_user ?? false}
                      onChange={(e) => updateField('voice.enabled_for_user', e.target.checked)}
                    />
                    <label htmlFor="pf-tts-user" className="profiles-field__label">
                      {t('profiles.fieldTTSUser', 'TTS para mensagens do usuário')}
                    </label>
                  </div>
                  <div className="profiles-field">
                    <label htmlFor="pf-channel-response" className="profiles-field__label">
                      {t('profiles.fieldChannelResponse', 'Resposta em canais externos')}
                    </label>
                    <select
                      id="pf-channel-response"
                      className="profiles-field__select"
                      value={editingProfile.voice?.channel_response_mode || 'mirror'}
                      onChange={(e) => updateField('voice.channel_response_mode', e.target.value)}
                    >
                      <option value="mirror">Espelhar (texto→texto, audio→audio)</option>
                      <option value="always_text">Sempre texto</option>
                      <option value="always_audio">Sempre audio (TTS)</option>
                    </select>
                    <p className="profiles-field__hint">
                      Define como conversas via Signal, Telegram e outros canais respondem.
                    </p>
                  </div>
                </div>
              </CollapsibleSection>
            );
          })()}

          {/* Interaction (STT) — colapsável */}
          {(() => {
            const isSTTDisabled = !editingProfile.interaction?.stt_provider;
            return (
              <CollapsibleSection
                title={t('profiles.collapseInteraction', 'Interação (STT)')}
                isOpen={!isSTTDisabled}
                onToggle={() => {
                  if (isSTTDisabled) {
                    updateField('interaction.stt_provider', 'webspeech');
                  } else {
                    updateField('interaction.stt_provider', '');
                  }
                }}
                badge={isSTTDisabled ? 'off' : 'on'}
              >
                <ProfileInteractionSection
                  sttProvider={editingProfile.interaction?.stt_provider || 'webspeech'}
                  sttLanguage={editingProfile.interaction?.language || 'pt-BR'}
                  enableFeedbackSounds={editingProfile.interaction?.feedback_sounds ?? true}
                  onChange={(field, value) => {
                    if (field === 'sttProvider') {
                      updateField('interaction.stt_provider', value);
                      return;
                    }
                    if (field === 'sttLanguage') {
                      updateField('interaction.language', value);
                      return;
                    }
                    updateField('interaction.feedback_sounds', value);
                  }}
                />
              </CollapsibleSection>
            );
          })()}

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
          <p>{t('profiles.selectHint', 'Pressione Enter ou clique ✏️ para editar um perfil.')}</p>
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
