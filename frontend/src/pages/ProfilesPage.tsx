import { useState, useEffect, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  GetProfiles,
  GetProfile,
  GetActiveProfileSlug,
  SetActiveProfile,
  CreateProfile,
  UpdateProfile,
  DeleteProfile,
  GetProfileSearchPaths,
} from '../../wailsjs/go/main/App';
import { profiles } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { useGridFocus } from '../hooks/useGridFocus';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import './ProfilesPage.css';

type ProfileInfo = profiles.ProfileInfo;
type Profile = profiles.Profile;

interface ProfileRow {
  id: string; // slug as id
  slug: string;
  name: string;
  description: string;
  icon: string;
  source: string;
  isActive: boolean;
}

export default function ProfilesPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  // Grid state
  const [profileRows, setProfileRows] = useState<ProfileRow[]>([]);
  const [activeSlug, setActiveSlug] = useState<string>('padrao');
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);

  // Editor state
  const [editingProfile, setEditingProfile] = useState<Profile | null>(null);
  const [editingSlug, setEditingSlug] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Refs for focus management
  const editorRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);

  const loadProfiles = useCallback(async () => {
    setLoading(true);
    try {
      const [allProfiles, currentSlug, paths] = await Promise.all([
        GetProfiles(),
        GetActiveProfileSlug(),
        GetProfileSearchPaths(),
      ]);
      setActiveSlug(currentSlug || 'padrao');
      setSearchPaths(paths || []);

      const rows: ProfileRow[] = (allProfiles || []).map((p: ProfileInfo) => ({
        id: p.slug,
        slug: p.slug,
        name: p.name,
        description: p.description || '',
        icon: p.icon || '',
        source: p.source,
        isActive: p.slug === (currentSlug || 'padrao'),
      }));
      setProfileRows(rows);
    } catch (error) {
      console.error('Erro ao carregar perfis:', error);
      addToast(t('profiles.loadError', 'Erro ao carregar perfis'), 'error');
    } finally {
      setLoading(false);
    }
  }, [addToast, t]);

  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

  // Focus editor when it opens, restore focus when it closes
  useEffect(() => {
    if (editingProfile && editorRef.current) {
      previousFocusRef.current = document.activeElement as HTMLElement;
      // Focus the first input inside the editor
      const firstInput = editorRef.current.querySelector<HTMLElement>(
        'input, select, textarea'
      );
      setTimeout(() => firstInput?.focus(), 50);
    }
  }, [editingProfile]);

  // ESC to close editor
  useEffect(() => {
    if (!editingProfile) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && editingProfile) {
        e.preventDefault();
        handleCloseEditor();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [editingProfile]);

  // --- Grid actions ---

  const handleEditProfile = async (row: ProfileRow) => {
    try {
      const profile = await GetProfile(row.slug);
      setEditingSlug(row.slug);
      setEditingProfile(profile);
      setIsNew(false);
      announce(t('profiles.editorOpened', `Editor aberto para ${row.name}`));
    } catch (error) {
      console.error('Erro ao carregar perfil:', error);
      addToast(t('profiles.loadOneError', 'Erro ao carregar perfil'), 'error');
    }
  };

  const handleActivateProfile = async (row: ProfileRow) => {
    try {
      await SetActiveProfile(row.slug);
      setActiveSlug(row.slug);
      setProfileRows(prev =>
        prev.map(r => ({ ...r, isActive: r.slug === row.slug }))
      );
      addToast(t('profiles.activated', `Perfil "${row.name}" ativado!`), 'success');
      announce(t('profiles.activatedAnnounce', `Perfil ${row.name} ativado`));
    } catch (error: any) {
      console.error('Erro ao ativar perfil:', error);
      addToast(error.message || t('profiles.activateError', 'Erro ao ativar perfil'), 'error');
    }
  };

  const handleDeleteProfile = async (row: ProfileRow) => {
    if (row.isActive) {
      addToast(t('profiles.cannotDeleteActive', 'Não é possível excluir o perfil ativo'), 'error');
      return;
    }
    if (!confirm(t('profiles.confirmDelete', `Tem certeza que deseja excluir o perfil "${row.name}"?`))) return;

    try {
      await DeleteProfile(row.slug);
      addToast(t('profiles.deleted', 'Perfil excluído!'), 'success');
      announce(t('profiles.deletedAnnounce', 'Perfil excluído'));

      if (editingSlug === row.slug) {
        setEditingSlug(null);
        setEditingProfile(null);
      }
      await loadProfiles();
    } catch (error: any) {
      console.error('Erro ao excluir perfil:', error);
      addToast(error.message || t('profiles.deleteError', 'Erro ao excluir perfil'), 'error');
    }
  };

  const handleNewProfile = () => {
    const defaultProfile = profiles.Profile.createFrom({
      name: 'Novo Perfil',
      description: '',
      icon: 'chatbox',
      chat: {
        model: '',
        temperature: 0.7,
        max_tokens: 4096,
        top_p: 1.0,
        response_timeout: 180,
        enable_thinking: false,
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
    });
    setEditingProfile(defaultProfile);
    setEditingSlug(null);
    setIsNew(true);
    announce(t('profiles.newProfileAnnounce', 'Editor aberto para novo perfil'));
  };

  // --- Editor actions ---

  const handleSave = async () => {
    if (!editingProfile) return;

    setSaving(true);
    try {
      if (isNew) {
        const slug = await CreateProfile(editingProfile);
        addToast(t('profiles.created', `Perfil "${editingProfile.name}" criado!`), 'success');
        announce(t('profiles.createdAnnounce', `Perfil ${editingProfile.name} criado`));
        setIsNew(false);
        setEditingSlug(slug);
      } else if (editingSlug) {
        await UpdateProfile(editingSlug, editingProfile);
        addToast(t('profiles.updated', `Perfil "${editingProfile.name}" atualizado!`), 'success');
        announce(t('profiles.updatedAnnounce', `Perfil ${editingProfile.name} atualizado`));
      }
      await loadProfiles();
    } catch (error: any) {
      console.error('Erro ao salvar perfil:', error);
      addToast(error.message || t('profiles.saveError', 'Erro ao salvar perfil'), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleCloseEditor = () => {
    setEditingProfile(null);
    setEditingSlug(null);
    setIsNew(false);
    announce(t('profiles.editorClosed', 'Editor fechado'));
    // Restore focus
    setTimeout(() => previousFocusRef.current?.focus(), 50);
  };

  const updateField = (path: string, value: any) => {
    if (!editingProfile) return;
    const updated = JSON.parse(JSON.stringify(editingProfile));
    const keys = path.split('.');
    let obj = updated;
    for (let i = 0; i < keys.length - 1; i++) {
      obj = obj[keys[i]];
    }
    obj[keys[keys.length - 1]] = value;
    setEditingProfile(profiles.Profile.createFrom(updated));
  };

  // --- Grid columns ---

  const columns: DataGridColumn<ProfileRow>[] = [
    {
      key: 'name',
      label: t('profiles.colName', 'Nome'),
      width: '30%',
      editable: true,
    },
    {
      key: 'description',
      label: t('profiles.colDescription', 'Descrição'),
      width: '30%',
      truncate: true,
    },
    {
      key: 'source',
      label: t('profiles.colSource', 'Origem'),
      width: '12%',
    },
    {
      key: 'isActive',
      label: t('profiles.colStatus', 'Status'),
      width: '13%',
      format: (_value: any, item: ProfileRow) =>
        item.isActive ? (
          <span className="profiles-badge profiles-badge--active">
            {t('profiles.active', 'Ativo')}
          </span>
        ) : (
          <span className="profiles-badge profiles-badge--inactive">
            {t('profiles.inactive', 'Inativo')}
          </span>
        ),
    },
    {
      key: 'activate',
      label: '',
      width: '5%',
      action: true,
      actionIcon: '⚡',
      actionLabel: t('profiles.activate', 'Ativar perfil'),
    },
    {
      key: 'edit',
      label: '',
      width: '5%',
      action: true,
      actionIcon: '✏️',
      actionLabel: t('profiles.edit', 'Editar perfil'),
    },
    {
      key: 'delete',
      label: '',
      width: '5%',
      action: true,
      actionIcon: '🗑️',
      actionLabel: t('profiles.delete', 'Excluir perfil'),
    },
  ];

  const handleCellAction = (item: ProfileRow, column: DataGridColumn<ProfileRow>) => {
    if (column.key === 'activate') {
      handleActivateProfile(item);
    } else if (column.key === 'edit') {
      handleEditProfile(item);
    } else if (column.key === 'delete') {
      handleDeleteProfile(item);
    }
  };

  const handleCellEdit = async (item: ProfileRow, column: DataGridColumn<ProfileRow>, newValue: string) => {
    if (column.key === 'name') {
      try {
        const profile = await GetProfile(item.slug);
        profile.name = newValue;
        await UpdateProfile(item.slug, profile);
        setProfileRows(prev =>
          prev.map(r => (r.slug === item.slug ? { ...r, name: newValue } : r))
        );
        if (editingSlug === item.slug && editingProfile) {
          updateField('name', newValue);
        }
      } catch (error) {
        console.error('Erro ao atualizar nome:', error);
        addToast(t('profiles.renameError', 'Erro ao renomear perfil'), 'error');
      }
    }
  };

  // --- Filtering ---

  const filteredRows = profileRows.filter(row =>
    row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    row.description.toLowerCase().includes(searchTerm.toLowerCase())
  );

  // --- Loading ---

  if (loading) {
    return (
      <div className="profiles-page">
        <div className="loading" role="status">{t('profiles.loading', 'Carregando perfis...')}</div>
      </div>
    );
  }

  // --- Render ---

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
            variant: 'primary',
          },
          {
            key: 'back',
            label: t('profiles.back', 'Voltar'),
            icon: '←',
            onClick: () => navigate('/'),
            variant: 'secondary',
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
        onDelete={(item) => handleDeleteProfile(item)}
        onCellAction={handleCellAction}
        onCellEdit={handleCellEdit}
        onGridReady={handleGridReady}
      />

      {/* Editor Panel */}
      {editingProfile && (
        <div
          ref={editorRef}
          className="profiles-editor"
          role="region"
          aria-label={t('profiles.editorLabel', `Editor de perfil: ${editorTitle}`)}
          aria-live="polite"
        >
          <div className="profiles-editor__header">
            <h2 id="profiles-editor-title">{editorTitle}</h2>
            <div className="profiles-editor__actions">
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="outline"
                  onClick={() => handleActivateProfile({ slug: editingSlug, name: editingProfile.name } as ProfileRow)}
                  aria-label={t('profiles.activateBtnLabel', `Ativar perfil ${editingProfile.name}`)}
                >
                  {t('profiles.activateBtn', 'Ativar')}
                </Button>
              )}
              {editingSlug && activeSlug !== editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => handleDeleteProfile({ slug: editingSlug, name: editingProfile.name, isActive: false } as ProfileRow)}
                  aria-label={t('profiles.deleteBtnLabel', `Excluir perfil ${editingProfile.name}`)}
                >
                  {t('profiles.deleteBtn', 'Excluir')}
                </Button>
              )}
              <Button onClick={handleSave} loading={saving}>
                {t('profiles.saveBtn', 'Salvar')}
              </Button>
              <Button
                variant="ghost"
                onClick={handleCloseEditor}
                aria-label={t('profiles.closeBtnLabel', 'Fechar editor, Escape')}
              >
                {t('profiles.closeBtn', 'Fechar')}
              </Button>
            </div>
          </div>

          {/* General Section */}
          <section className="profiles-section" aria-labelledby="section-general">
            <h3 id="section-general">{t('profiles.sectionGeneral', 'Geral')}</h3>
            <div className="profiles-fields">
              <div className="profiles-field">
                <label htmlFor="pf-name" className="profiles-field__label">
                  {t('profiles.fieldName', 'Nome')}
                </label>
                <input
                  id="pf-name"
                  type="text"
                  className="profiles-field__input"
                  value={editingProfile.name}
                  onChange={(e) => updateField('name', e.target.value)}
                />
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-description" className="profiles-field__label">
                  {t('profiles.fieldDescription', 'Descrição')}
                </label>
                <input
                  id="pf-description"
                  type="text"
                  className="profiles-field__input"
                  value={editingProfile.description || ''}
                  onChange={(e) => updateField('description', e.target.value)}
                />
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-icon" className="profiles-field__label">
                  {t('profiles.fieldIcon', 'Ícone (Ionicons)')}
                </label>
                <input
                  id="pf-icon"
                  type="text"
                  className="profiles-field__input"
                  value={editingProfile.icon || ''}
                  onChange={(e) => updateField('icon', e.target.value)}
                  placeholder="chatbox"
                />
              </div>
            </div>
          </section>

          {/* Chat Section */}
          <section className="profiles-section" aria-labelledby="section-chat">
            <h3 id="section-chat">{t('profiles.sectionChat', 'Chat (LLM)')}</h3>
            <div className="profiles-fields">
              <div className="profiles-field">
                <label htmlFor="pf-model" className="profiles-field__label">
                  {t('profiles.fieldModel', 'Modelo')}
                </label>
                <input
                  id="pf-model"
                  type="text"
                  className="profiles-field__input"
                  value={editingProfile.chat?.model || ''}
                  onChange={(e) => updateField('chat.model', e.target.value)}
                  placeholder={t('profiles.modelPlaceholder', 'gpt-4o-mini (vazio = usa padrão)')}
                />
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-temperature" className="profiles-field__label">
                  {t('profiles.fieldTemperature', 'Temperatura')}
                </label>
                <div className="profiles-field__range-group">
                  <input
                    id="pf-temperature"
                    type="range"
                    className="profiles-field__range"
                    min="0"
                    max="2"
                    step="0.05"
                    value={editingProfile.chat?.temperature ?? 0.7}
                    onChange={(e) => updateField('chat.temperature', parseFloat(e.target.value))}
                    aria-valuetext={`${editingProfile.chat?.temperature?.toFixed(2) ?? '0.70'}`}
                  />
                  <span className="profiles-field__range-value" aria-hidden="true">
                    {editingProfile.chat?.temperature?.toFixed(2) ?? '0.70'}
                  </span>
                </div>
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-max-tokens" className="profiles-field__label">
                  {t('profiles.fieldMaxTokens', 'Max Tokens')}
                </label>
                <input
                  id="pf-max-tokens"
                  type="number"
                  className="profiles-field__input"
                  min="1"
                  max="128000"
                  value={editingProfile.chat?.max_tokens ?? 4096}
                  onChange={(e) => updateField('chat.max_tokens', parseInt(e.target.value) || 4096)}
                />
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-top-p" className="profiles-field__label">
                  {t('profiles.fieldTopP', 'Top P')}
                </label>
                <div className="profiles-field__range-group">
                  <input
                    id="pf-top-p"
                    type="range"
                    className="profiles-field__range"
                    min="0"
                    max="1"
                    step="0.05"
                    value={editingProfile.chat?.top_p ?? 1.0}
                    onChange={(e) => updateField('chat.top_p', parseFloat(e.target.value))}
                    aria-valuetext={`${editingProfile.chat?.top_p?.toFixed(2) ?? '1.00'}`}
                  />
                  <span className="profiles-field__range-value" aria-hidden="true">
                    {editingProfile.chat?.top_p?.toFixed(2) ?? '1.00'}
                  </span>
                </div>
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-timeout" className="profiles-field__label">
                  {t('profiles.fieldTimeout', 'Timeout (segundos)')}
                </label>
                <input
                  id="pf-timeout"
                  type="number"
                  className="profiles-field__input"
                  min="10"
                  max="600"
                  value={editingProfile.chat?.response_timeout ?? 180}
                  onChange={(e) => updateField('chat.response_timeout', parseInt(e.target.value) || 180)}
                />
              </div>
              <div className="profiles-field profiles-field--checkbox">
                <input
                  id="pf-thinking"
                  type="checkbox"
                  checked={editingProfile.chat?.enable_thinking ?? false}
                  onChange={(e) => updateField('chat.enable_thinking', e.target.checked)}
                />
                <label htmlFor="pf-thinking" className="profiles-field__label">
                  {t('profiles.fieldThinking', 'Habilitar Thinking/Reasoning')}
                </label>
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-system-prompt" className="profiles-field__label">
                  {t('profiles.fieldSystemPrompt', 'System Prompt')}
                </label>
                <textarea
                  id="pf-system-prompt"
                  className="profiles-field__textarea"
                  value={editingProfile.chat?.system_prompt || ''}
                  onChange={(e) => updateField('chat.system_prompt', e.target.value)}
                  placeholder={t('profiles.systemPromptPlaceholder', 'Prompt customizado (vazio = usa padrão)')}
                  rows={4}
                />
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-prompt-position" className="profiles-field__label">
                  {t('profiles.fieldPromptPosition', 'Posição do System Prompt')}
                </label>
                <select
                  id="pf-prompt-position"
                  className="profiles-field__select"
                  value={editingProfile.chat?.system_prompt_position || 'after'}
                  onChange={(e) => updateField('chat.system_prompt_position', e.target.value)}
                >
                  <option value="before">{t('profiles.promptBefore', 'Antes do prompt base')}</option>
                  <option value="after">{t('profiles.promptAfter', 'Depois do prompt base')}</option>
                </select>
              </div>
            </div>
          </section>

          {/* Voice Section */}
          <section className="profiles-section" aria-labelledby="section-voice">
            <h3 id="section-voice">{t('profiles.sectionVoice', 'Voz (TTS)')}</h3>
            <div className="profiles-fields">
              <div className="profiles-field">
                <label htmlFor="pf-voice-provider" className="profiles-field__label">
                  {t('profiles.fieldVoiceProvider', 'Provedor')}
                </label>
                <select
                  id="pf-voice-provider"
                  className="profiles-field__select"
                  value={editingProfile.voice?.provider || 'disabled'}
                  onChange={(e) => updateField('voice.provider', e.target.value)}
                >
                  <option value="disabled">{t('profiles.voiceDisabled', 'Desativado')}</option>
                  <option value="webspeech">Web Speech</option>
                  <option value="sapi5">SAPI5 (Windows)</option>
                  <option value="openai">OpenAI TTS</option>
                </select>
              </div>
              {editingProfile.voice?.provider !== 'disabled' && (
                <>
                  <div className="profiles-field">
                    <label htmlFor="pf-voice-id" className="profiles-field__label">Voice ID</label>
                    <input
                      id="pf-voice-id"
                      type="text"
                      className="profiles-field__input"
                      value={editingProfile.voice?.voice_id || ''}
                      onChange={(e) => updateField('voice.voice_id', e.target.value)}
                      placeholder="alloy, nova, echo..."
                    />
                  </div>
                  <div className="profiles-field">
                    <label htmlFor="pf-voice-rate" className="profiles-field__label">
                      {t('profiles.fieldVoiceRate', 'Velocidade')}
                    </label>
                    <div className="profiles-field__range-group">
                      <input
                        id="pf-voice-rate"
                        type="range"
                        className="profiles-field__range"
                        min="0.25"
                        max="4"
                        step="0.1"
                        value={editingProfile.voice?.rate ?? 1.0}
                        onChange={(e) => updateField('voice.rate', parseFloat(e.target.value))}
                        aria-valuetext={`${editingProfile.voice?.rate?.toFixed(1) ?? '1.0'}x`}
                      />
                      <span className="profiles-field__range-value" aria-hidden="true">
                        {editingProfile.voice?.rate?.toFixed(1) ?? '1.0'}
                      </span>
                    </div>
                  </div>
                  <div className="profiles-field">
                    <label htmlFor="pf-voice-volume" className="profiles-field__label">
                      {t('profiles.fieldVoiceVolume', 'Volume')}
                    </label>
                    <div className="profiles-field__range-group">
                      <input
                        id="pf-voice-volume"
                        type="range"
                        className="profiles-field__range"
                        min="0"
                        max="1"
                        step="0.1"
                        value={editingProfile.voice?.volume ?? 1.0}
                        onChange={(e) => updateField('voice.volume', parseFloat(e.target.value))}
                        aria-valuetext={`${Math.round((editingProfile.voice?.volume ?? 1.0) * 100)}%`}
                      />
                      <span className="profiles-field__range-value" aria-hidden="true">
                        {editingProfile.voice?.volume?.toFixed(1) ?? '1.0'}
                      </span>
                    </div>
                  </div>
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
                </>
              )}
            </div>
          </section>

          {/* Interaction Section */}
          <section className="profiles-section" aria-labelledby="section-interaction">
            <h3 id="section-interaction">{t('profiles.sectionInteraction', 'Interação (STT)')}</h3>
            <div className="profiles-fields">
              <div className="profiles-field">
                <label htmlFor="pf-stt-provider" className="profiles-field__label">
                  {t('profiles.fieldSTTProvider', 'Provedor STT')}
                </label>
                <select
                  id="pf-stt-provider"
                  className="profiles-field__select"
                  value={editingProfile.interaction?.stt_provider || 'webspeech'}
                  onChange={(e) => updateField('interaction.stt_provider', e.target.value)}
                >
                  <option value="webspeech">Web Speech API</option>
                  <option value="whisper_api">Whisper API</option>
                </select>
              </div>
              <div className="profiles-field">
                <label htmlFor="pf-language" className="profiles-field__label">
                  {t('profiles.fieldLanguage', 'Idioma')}
                </label>
                <input
                  id="pf-language"
                  type="text"
                  className="profiles-field__input"
                  value={editingProfile.interaction?.language || 'pt-BR'}
                  onChange={(e) => updateField('interaction.language', e.target.value)}
                  placeholder="pt-BR"
                />
              </div>
              <div className="profiles-field profiles-field--checkbox">
                <input
                  id="pf-feedback-sounds"
                  type="checkbox"
                  checked={editingProfile.interaction?.feedback_sounds ?? true}
                  onChange={(e) => updateField('interaction.feedback_sounds', e.target.checked)}
                />
                <label htmlFor="pf-feedback-sounds" className="profiles-field__label">
                  {t('profiles.fieldFeedbackSounds', 'Sons de feedback')}
                </label>
              </div>
            </div>
          </section>
        </div>
      )}

      {/* Empty state when no profile is being edited */}
      {!editingProfile && profileRows.length > 0 && (
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
