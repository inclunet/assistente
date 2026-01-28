import { useState, useEffect } from 'react';
import { 
  GetInteractionProfiles,
  GetInteractionProfile,
  CreateInteractionProfile,
  UpdateInteractionProfile,
  DeleteInteractionProfile,
  SetDefaultInteractionProfile,
  CreateInteractionTrigger,
  UpdateInteractionTrigger,
  DeleteInteractionTrigger,
} from '../../wailsjs/go/main/App';
import { database } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Select } from '../components/ui/Select';
import { Button } from '../components/ui/Button';
import { useGridFocus } from '../hooks/useGridFocus';
import './VoiceProfilesPage.css';

type InteractionProfile = database.InteractionProfile;
type InteractionTrigger = database.InteractionTrigger;

// Tipos de trigger
const TRIGGER_TYPES = [
  { value: 'hotkey', label: 'Hotkey', description: 'Atalho de teclado' },
  { value: 'button_ptt', label: 'Botão PTT', description: 'Segura para gravar' },
  { value: 'button_toggle', label: 'Botão Toggle', description: 'Clica para alternar' },
  { value: 'wakeword', label: 'Wakeword', description: 'Palavra de ativação' },
  { value: 'vad', label: 'VAD', description: 'Detecção contínua' },
];

// Providers STT
const STT_PROVIDERS = [
  { value: 'webspeech', label: 'WebSpeech' },
  { value: 'whisper_api', label: 'Whisper API' },
];

// Providers Wakeword
const WAKEWORD_PROVIDERS = [
  { value: 'webspeech', label: 'WebSpeech' },
];

export default function InteractionProfilesPage() {
  const [profiles, setProfiles] = useState<InteractionProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const { focusFirstCell, handleGridReady } = useGridFocus();
  
  // Modal de perfil
  const [showProfileModal, setShowProfileModal] = useState(false);
  const [editingProfile, setEditingProfile] = useState<InteractionProfile | null>(null);
  const [saving, setSaving] = useState(false);
  
  // Form state do perfil
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formSttProvider, setFormSttProvider] = useState('webspeech');
  const [formLanguage, setFormLanguage] = useState('pt-BR');
  const [formFeedbackSounds, setFormFeedbackSounds] = useState(true);
  const [formIsDefault, setFormIsDefault] = useState(false);
  const [formError, setFormError] = useState('');

  // Modal de trigger
  const [showTriggerModal, setShowTriggerModal] = useState(false);
  const [editingTrigger, setEditingTrigger] = useState<InteractionTrigger | null>(null);
  const [triggerProfileId, setTriggerProfileId] = useState<number | null>(null);

  // Form state do trigger
  const [triggerType, setTriggerType] = useState('button_toggle');
  const [triggerEnabled, setTriggerEnabled] = useState(true);
  const [triggerAutoStop, setTriggerAutoStop] = useState(false);
  const [triggerHotkey, setTriggerHotkey] = useState('');
  const [triggerHotkeyGlobal, setTriggerHotkeyGlobal] = useState(true);
  const [triggerBringToFront, setTriggerBringToFront] = useState(true);
  const [triggerWakeword, setTriggerWakeword] = useState('assistente');
  const [triggerWakewordProvider, setTriggerWakewordProvider] = useState('webspeech');
  const [triggerWakewordSensitivity, setTriggerWakewordSensitivity] = useState(0.5);
  const [triggerVadSilenceThreshold, setTriggerVadSilenceThreshold] = useState(0.01);
  const [triggerVadSilenceDuration, setTriggerVadSilenceDuration] = useState(1500);
  const [triggerVadActivityThreshold, setTriggerVadActivityThreshold] = useState(0.02);
  const [triggerVadActivityDuration, setTriggerVadActivityDuration] = useState(200);
  const [triggerError, setTriggerError] = useState('');

  useEffect(() => {
    loadProfiles();
  }, []);

  // Atalho Ctrl+N para novo perfil
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.key.toLowerCase() === 'n') {
        event.preventDefault();
        openNewProfileForm();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadProfiles = async () => {
    setLoading(true);
    try {
      const result = await GetInteractionProfiles();
      setProfiles(result || []);
    } catch (error) {
      console.error('Erro ao carregar perfis de interação:', error);
    } finally {
      setLoading(false);
    }
  };

  // === Perfil CRUD ===

  const openNewProfileForm = () => {
    setEditingProfile(null);
    setFormName('');
    setFormDescription('');
    setFormSttProvider('webspeech');
    setFormLanguage('pt-BR');
    setFormFeedbackSounds(true);
    setFormIsDefault(false);
    setFormError('');
    setShowProfileModal(true);
  };

  const openEditProfileForm = async (profile: InteractionProfile) => {
    // Busca o perfil atualizado do backend para garantir que os triggers sejam carregados
    try {
      const freshProfile = await GetInteractionProfile(profile.id);
      setEditingProfile(freshProfile);
      setFormName(freshProfile.name);
      setFormDescription(freshProfile.description || '');
      setFormSttProvider(freshProfile.stt_provider || 'webspeech');
      setFormLanguage(freshProfile.language || 'pt-BR');
      setFormFeedbackSounds(freshProfile.feedback_sounds ?? true);
      setFormIsDefault(freshProfile.is_default || false);
      setFormError('');
      setShowProfileModal(true);
    } catch (error) {
      console.error('Erro ao carregar perfil:', error);
      // Fallback para dados locais
      setEditingProfile(profile);
      setFormName(profile.name);
      setFormDescription(profile.description || '');
      setFormSttProvider(profile.stt_provider || 'webspeech');
      setFormLanguage(profile.language || 'pt-BR');
      setFormFeedbackSounds(profile.feedback_sounds ?? true);
      setFormIsDefault(profile.is_default || false);
      setFormError('');
      setShowProfileModal(true);
    }
  };

  const handleSaveProfile = async () => {
    if (!formName.trim()) {
      setFormError('Nome é obrigatório');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      const profileData = new database.InteractionProfile({
        id: editingProfile?.id || 0,
        name: formName,
        description: formDescription,
        is_default: formIsDefault,
        stt_provider: formSttProvider,
        language: formLanguage,
        feedback_sounds: formFeedbackSounds,
      });

      if (editingProfile) {
        await UpdateInteractionProfile(editingProfile.id, profileData);
      } else {
        await CreateInteractionProfile(profileData);
      }
      await loadProfiles();
      setShowProfileModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteProfile = async (profile: InteractionProfile) => {
    if (!confirm(`Tem certeza que deseja excluir o perfil "${profile.name}"?`)) return;
    
    try {
      await DeleteInteractionProfile(profile.id);
      setProfiles(prev => prev.filter(p => p.id !== profile.id));
    } catch (error) {
      console.error('Erro ao deletar perfil:', error);
      alert('Erro ao deletar perfil de interação');
    }
  };

  const handleSetDefault = async (profile: InteractionProfile) => {
    try {
      await SetDefaultInteractionProfile(profile.id);
      await loadProfiles();
    } catch (error) {
      console.error('Erro ao definir perfil padrão:', error);
      alert('Erro ao definir perfil padrão');
    }
  };

  const handleDeleteSelected = async () => {
    if (selectedIds.size === 0) return;
    if (!confirm(`Tem certeza que deseja excluir ${selectedIds.size} perfil(s)?`)) return;

    try {
      await Promise.all(Array.from(selectedIds).map(id => DeleteInteractionProfile(Number(id))));
      setProfiles(prev => prev.filter(p => !selectedIds.has(p.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar perfis:', error);
      alert('Erro ao deletar perfis');
    }
  };

  // === Trigger CRUD ===

  const openNewTriggerForm = (profileId: number) => {
    setEditingTrigger(null);
    setTriggerProfileId(profileId);
    setTriggerType('button_toggle');
    setTriggerEnabled(true);
    setTriggerAutoStop(false);
    setTriggerHotkey('');
    setTriggerHotkeyGlobal(true);
    setTriggerBringToFront(true);
    setTriggerWakeword('assistente');
    setTriggerWakewordProvider('webspeech');
    setTriggerWakewordSensitivity(0.5);
    setTriggerVadSilenceThreshold(0.01);
    setTriggerVadSilenceDuration(1500);
    setTriggerVadActivityThreshold(0.02);
    setTriggerVadActivityDuration(200);
    setTriggerError('');
    setShowTriggerModal(true);
  };

  const openEditTriggerForm = (trigger: InteractionTrigger, profileId: number) => {
    setEditingTrigger(trigger);
    setTriggerProfileId(profileId);
    setTriggerType(trigger.type || 'button_toggle');
    setTriggerEnabled(trigger.enabled ?? true);
    setTriggerAutoStop(trigger.auto_stop ?? false);
    setTriggerHotkey(trigger.hotkey || '');
    setTriggerHotkeyGlobal(trigger.hotkey_global ?? true);
    setTriggerBringToFront(trigger.hotkey_bring_to_front ?? true);
    setTriggerWakeword(trigger.wakeword_keyword || 'assistente');
    setTriggerWakewordProvider(trigger.wakeword_provider || 'webspeech');
    setTriggerWakewordSensitivity(trigger.wakeword_sensitivity ?? 0.5);
    setTriggerVadSilenceThreshold(trigger.vad_silence_threshold ?? 0.01);
    setTriggerVadSilenceDuration(trigger.vad_silence_duration ?? 1500);
    setTriggerVadActivityThreshold(trigger.vad_activity_threshold ?? 0.02);
    setTriggerVadActivityDuration(trigger.vad_activity_duration ?? 200);
    setTriggerError('');
    setShowTriggerModal(true);
  };

  const handleSaveTrigger = async () => {
    if (triggerType === 'hotkey' && !triggerHotkey.trim()) {
      setTriggerError('Hotkey é obrigatória para este tipo');
      return;
    }
    if (triggerType === 'wakeword' && !triggerWakeword.trim()) {
      setTriggerError('Palavra de ativação é obrigatória para este tipo');
      return;
    }

    setSaving(true);
    setTriggerError('');

    try {
      const triggerData = new database.InteractionTrigger({
        id: editingTrigger?.id || 0,
        profile_id: triggerProfileId!,
        type: triggerType,
        enabled: triggerEnabled,
        auto_stop: triggerAutoStop,
        hotkey: triggerHotkey,
        hotkey_global: triggerHotkeyGlobal,
        hotkey_bring_to_front: triggerBringToFront,
        wakeword_keyword: triggerWakeword,
        wakeword_provider: triggerWakewordProvider,
        wakeword_sensitivity: triggerWakewordSensitivity,
        vad_silence_threshold: triggerVadSilenceThreshold,
        vad_silence_duration: triggerVadSilenceDuration,
        vad_activity_threshold: triggerVadActivityThreshold,
        vad_activity_duration: triggerVadActivityDuration,
      });

      if (editingTrigger) {
        await UpdateInteractionTrigger(editingTrigger.id, triggerData);
      } else {
        await CreateInteractionTrigger(triggerData);
      }
      
      // Recarrega perfis
      const updatedProfiles = await GetInteractionProfiles();
      setProfiles(updatedProfiles || []);
      
      // Atualiza o perfil em edição com os dados atualizados (incluindo triggers)
      if (triggerProfileId) {
        const updatedProfile = updatedProfiles?.find(p => p.id === triggerProfileId);
        if (updatedProfile) {
          setEditingProfile(updatedProfile);
        }
      }
      
      setShowTriggerModal(false);
    } catch (error: any) {
      setTriggerError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteTrigger = async (trigger: InteractionTrigger) => {
    if (!confirm('Tem certeza que deseja excluir este trigger?')) return;
    
    try {
      await DeleteInteractionTrigger(trigger.id);
      
      // Recarrega perfis
      const updatedProfiles = await GetInteractionProfiles();
      setProfiles(updatedProfiles || []);
      
      // Atualiza o perfil em edição
      if (editingProfile) {
        const updatedProfile = updatedProfiles?.find(p => p.id === editingProfile.id);
        if (updatedProfile) {
          setEditingProfile(updatedProfile);
        }
      }
    } catch (error) {
      console.error('Erro ao deletar trigger:', error);
      alert('Erro ao deletar trigger');
    }
  };

  // === Helpers ===

  const getTriggerTypeName = (type: string) => {
    return TRIGGER_TYPES.find(t => t.value === type)?.label || type;
  };

  const getTriggerSummary = (triggers: InteractionTrigger[] | undefined) => {
    if (!triggers || triggers.length === 0) return 'Nenhum trigger';
    const enabled = triggers.filter(t => t.enabled);
    if (enabled.length === 0) return `${triggers.length} trigger(s) desativado(s)`;
    return enabled.map(t => getTriggerTypeName(t.type)).join(', ');
  };

  const getSttProviderLabel = (provider: string) => {
    return STT_PROVIDERS.find(p => p.value === provider)?.label || provider;
  };

  // Filtra perfis
  const filteredProfiles = profiles.filter(profile =>
    profile.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    (profile.description || '').toLowerCase().includes(searchTerm.toLowerCase())
  );

  // Colunas do DataGrid
  const columns: DataGridColumn<InteractionProfile>[] = [
    { 
      key: 'name', 
      label: 'Nome',
      format: (value, item) => item.is_default ? `⭐ ${value}` : value,
    },
    { 
      key: 'triggers', 
      label: 'Triggers',
      width: '200px',
      format: (_, item) => getTriggerSummary(item.triggers),
    },
    { 
      key: 'stt_provider', 
      label: 'STT',
      width: '120px',
      format: (value) => getSttProviderLabel(value),
    },
    { 
      key: 'language', 
      label: 'Idioma',
      width: '100px',
    },
    { 
      key: 'set-default', 
      label: 'Padrão',
      width: '80px',
      action: true,
      actionIcon: '⭐',
      actionLabel: 'Definir como padrão',
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '✏️',
      actionLabel: 'Editar perfil',
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️',
      actionLabel: 'Excluir perfil',
    }
  ];

  // Ações da toolbar
  const toolbarActions: ToolbarAction[] = [
    {
      key: 'new',
      label: 'Novo Perfil',
      icon: '➕',
      onClick: openNewProfileForm,
      variant: 'primary',
      shortcut: 'Ctrl+N',
    },
    ...(selectedIds.size > 0
      ? [
          {
            key: 'delete-selected',
            label: `Deletar (${selectedIds.size})`,
            icon: '🗑️',
            onClick: handleDeleteSelected,
            variant: 'danger' as const,
          },
        ]
      : []),
  ];

  // Verifica se tipo de trigger precisa de VAD config
  const needsVadConfig = triggerAutoStop || triggerType === 'wakeword' || triggerType === 'vad';

  if (loading) {
    return (
      <div className="voice-profiles-page">
        <Toolbar left={<h1 className="page-toolbar__title">Perfis de Interação</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando perfis de interação...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="voice-profiles-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Perfis de Interação</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar perfis..."
        onFocusGrid={focusFirstCell}
      />

      <div className="page-content">
        <div className="info-box">
          <span>🎙️ Configure perfis de interação por voz. Cada perfil define configurações comuns (STT, idioma) e pode ter múltiplos triggers (hotkey, wakeword, botão). O perfil marcado com ⭐ é o padrão.</span>
        </div>

        <DataGrid
          items={filteredProfiles}
          columns={columns}
          label="Lista de Perfis de Interação"
          getItemId={(profile) => profile.id}
          onActivate={(profile) => openEditProfileForm(profile)}
          onDelete={(profile) => handleDeleteProfile(profile)}
          onCellAction={(profile, column) => {
            if (column.key === 'edit') {
              openEditProfileForm(profile);
            } else if (column.key === 'delete') {
              handleDeleteProfile(profile);
            } else if (column.key === 'set-default') {
              handleSetDefault(profile);
            }
          }}
          multiSelect={true}
          selectedIds={selectedIds}
          onSelectionChange={setSelectedIds}
          onGridReady={handleGridReady}
        />

        {profiles.length === 0 && (
          <div className="empty-state">
            <p>Nenhum perfil de interação criado ainda.</p>
            <p>Clique em "Novo Perfil" para criar seu primeiro perfil de interação.</p>
          </div>
        )}
      </div>

      {/* Modal de Perfil */}
      {showProfileModal && (
        <SimpleModal
          isOpen={showProfileModal}
          onClose={() => setShowProfileModal(false)}
          title={editingProfile ? 'Editar Perfil de Interação' : 'Novo Perfil de Interação'}
          size="lg"
        >
          <div className="modal-form">
            {formError && (
              <div className="form-error">{formError}</div>
            )}

            <div className="form-group">
              <label htmlFor="name">Nome *</label>
              <Input
                id="name"
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
                placeholder="Ex: Conversa Rápida, Ditado Longo"
                autoFocus
              />
            </div>

            <div className="form-group">
              <label htmlFor="description">Descrição</label>
              <Textarea
                id="description"
                value={formDescription}
                onChange={(e) => setFormDescription(e.target.value)}
                placeholder="Descrição opcional do perfil"
                rows={2}
              />
            </div>

            <div className="form-section">
              <label className="form-section-title">Configurações de STT</label>
              
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="stt-provider">Provider</label>
                  <Select
                    id="stt-provider"
                    value={formSttProvider}
                    onChange={(e) => setFormSttProvider(e.target.value)}
                    options={STT_PROVIDERS}
                  />
                </div>

                <div className="form-group">
                  <label htmlFor="language">Idioma</label>
                  <Input
                    id="language"
                    value={formLanguage}
                    onChange={(e) => setFormLanguage(e.target.value)}
                    placeholder="pt-BR"
                  />
                </div>
              </div>

              <div className="checkbox-row">
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={formFeedbackSounds}
                    onChange={(e) => setFormFeedbackSounds(e.target.checked)}
                  />
                  <span>Sons de feedback (início/fim da gravação)</span>
                </label>

                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={formIsDefault}
                    onChange={(e) => setFormIsDefault(e.target.checked)}
                  />
                  <span>Definir como perfil padrão</span>
                </label>
              </div>
            </div>

            {/* Triggers */}
            {editingProfile && (
              <div className="form-section">
                <div className="form-section-header">
                  <label className="form-section-title">Triggers</label>
                  <Button 
                    variant="secondary" 
                    size="sm"
                    onClick={() => openNewTriggerForm(editingProfile.id)}
                  >
                    + Adicionar
                  </Button>
                </div>

                {(!editingProfile.triggers || editingProfile.triggers.length === 0) ? (
                  <p className="empty-triggers">Nenhum trigger configurado. Adicione um trigger para definir como este perfil pode ser ativado.</p>
                ) : (
                  <ul className="triggers-list" role="list" aria-label="Lista de triggers">
                    {editingProfile.triggers.map(trigger => (
                      <li 
                        key={trigger.id} 
                        className={`trigger-item ${!trigger.enabled ? 'disabled' : ''}`}
                        role="listitem"
                      >
                        <div className="trigger-info">
                          <span className="trigger-type">{getTriggerTypeName(trigger.type)}</span>
                          {trigger.type === 'hotkey' && trigger.hotkey && (
                            <span className="trigger-detail">{trigger.hotkey}</span>
                          )}
                          {trigger.type === 'wakeword' && trigger.wakeword_keyword && (
                            <span className="trigger-detail">"{trigger.wakeword_keyword}"</span>
                          )}
                          {!trigger.enabled && <span className="trigger-disabled-badge" aria-label="Status: desativado">Desativado</span>}
                        </div>
                        <div className="trigger-actions" role="group" aria-label={`Ações para trigger ${getTriggerTypeName(trigger.type)}`}>
                          <button 
                            className="trigger-action-btn"
                            onClick={() => openEditTriggerForm(trigger, editingProfile.id)}
                            aria-label={`Editar trigger ${getTriggerTypeName(trigger.type)}`}
                            title="Editar"
                          >
                            ✏️
                          </button>
                          <button 
                            className="trigger-action-btn danger"
                            onClick={() => handleDeleteTrigger(trigger)}
                            aria-label={`Excluir trigger ${getTriggerTypeName(trigger.type)}`}
                            title="Excluir"
                          >
                            🗑️
                          </button>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}

            <div className="modal-actions">
              <div></div>
              <div className="modal-actions-right">
                <Button variant="secondary" onClick={() => setShowProfileModal(false)}>
                  Cancelar
                </Button>
                <Button variant="primary" onClick={handleSaveProfile} disabled={saving}>
                  {saving ? 'Salvando...' : 'Salvar'}
                </Button>
              </div>
            </div>
          </div>
        </SimpleModal>
      )}

      {/* Modal de Trigger */}
      {showTriggerModal && (
        <SimpleModal
          isOpen={showTriggerModal}
          onClose={() => setShowTriggerModal(false)}
          title={editingTrigger ? 'Editar Trigger' : 'Novo Trigger'}
          size="md"
        >
          <div className="modal-form">
            {triggerError && (
              <div className="form-error">{triggerError}</div>
            )}

            <div className="form-group">
              <label htmlFor="trigger-type">Tipo *</label>
              <Select
                id="trigger-type"
                value={triggerType}
                onChange={(e) => setTriggerType(e.target.value)}
                options={TRIGGER_TYPES.map(t => ({ value: t.value, label: `${t.label} - ${t.description}` }))}
              />
            </div>

            <div className="checkbox-row">
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={triggerEnabled}
                  onChange={(e) => setTriggerEnabled(e.target.checked)}
                />
                <span>Trigger ativado</span>
              </label>

              {(triggerType === 'hotkey' || triggerType === 'button_toggle') && (
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={triggerAutoStop}
                    onChange={(e) => setTriggerAutoStop(e.target.checked)}
                  />
                  <span>Parar automaticamente (VAD)</span>
                </label>
              )}
            </div>

            {/* Hotkey config */}
            {(triggerType === 'hotkey' || triggerType === 'wakeword' || triggerType === 'vad') && (
              <div className="form-section">
                <label className="form-section-title">
                  {triggerType === 'hotkey' ? 'Tecla de ativação' : 'Tecla de toggle'}
                </label>

                <div className="form-group">
                  <label htmlFor="hotkey">Combinação de tecla {triggerType === 'hotkey' ? '*' : ''}</label>
                  <Input
                    id="hotkey"
                    value={triggerHotkey}
                    onChange={(e) => setTriggerHotkey(e.target.value)}
                    placeholder="Ex: Ctrl+Shift+Space"
                  />
                  <span className="form-hint">
                    {triggerType === 'hotkey' 
                      ? 'Tecla que inicia/para a gravação'
                      : 'Tecla para ligar/desligar a escuta'}
                  </span>
                </div>

                <div className="checkbox-row">
                  <label className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={triggerHotkeyGlobal}
                      onChange={(e) => setTriggerHotkeyGlobal(e.target.checked)}
                    />
                    <span>Hotkey global (funciona em qualquer app)</span>
                  </label>

                  {triggerHotkeyGlobal && (
                    <label className="checkbox-label">
                      <input
                        type="checkbox"
                        checked={triggerBringToFront}
                        onChange={(e) => setTriggerBringToFront(e.target.checked)}
                      />
                      <span>Trazer janela para frente</span>
                    </label>
                  )}
                </div>
              </div>
            )}

            {/* Wakeword config */}
            {triggerType === 'wakeword' && (
              <div className="form-section">
                <label className="form-section-title">Configuração Wakeword</label>

                <div className="form-row">
                  <div className="form-group">
                    <label htmlFor="wakeword-provider">Provider</label>
                    <Select
                      id="wakeword-provider"
                      value={triggerWakewordProvider}
                      onChange={(e) => setTriggerWakewordProvider(e.target.value)}
                      options={WAKEWORD_PROVIDERS}
                    />
                  </div>

                  <div className="form-group">
                    <label htmlFor="wakeword">Palavra de ativação *</label>
                    <Input
                      id="wakeword"
                      value={triggerWakeword}
                      onChange={(e) => setTriggerWakeword(e.target.value)}
                      placeholder="Ex: assistente"
                    />
                  </div>
                </div>

                <div className="form-group">
                  <label htmlFor="wakeword-sensitivity">Sensibilidade: {triggerWakewordSensitivity.toFixed(2)}</label>
                  <input
                    type="range"
                    id="wakeword-sensitivity"
                    aria-label={`Sensibilidade do wakeword: ${triggerWakewordSensitivity.toFixed(2)}`}
                    aria-valuemin={0}
                    aria-valuemax={1}
                    aria-valuenow={triggerWakewordSensitivity}
                    min="0"
                    max="1"
                    step="0.05"
                    value={triggerWakewordSensitivity}
                    onChange={(e) => setTriggerWakewordSensitivity(parseFloat(e.target.value))}
                    className="slider"
                  />
                </div>
              </div>
            )}

            {/* VAD config */}
            {needsVadConfig && (
              <div className="form-section">
                <label className="form-section-title">Configuração VAD</label>

                <div className="form-row">
                  <div className="form-group">
                    <label htmlFor="vad-silence-threshold">Limiar silêncio: {triggerVadSilenceThreshold.toFixed(3)}</label>
                    <input
                      type="range"
                      id="vad-silence-threshold"
                      aria-label={`Limiar de silêncio: ${triggerVadSilenceThreshold.toFixed(3)}`}
                      aria-valuemin={0.001}
                      aria-valuemax={0.1}
                      aria-valuenow={triggerVadSilenceThreshold}
                      min="0.001"
                      max="0.1"
                      step="0.001"
                      value={triggerVadSilenceThreshold}
                      onChange={(e) => setTriggerVadSilenceThreshold(parseFloat(e.target.value))}
                      className="slider"
                    />
                  </div>

                  <div className="form-group">
                    <label htmlFor="silence-duration">Duração silêncio (ms)</label>
                    <Input
                      id="silence-duration"
                      type="number"
                      value={triggerVadSilenceDuration.toString()}
                      onChange={(e) => setTriggerVadSilenceDuration(parseInt(e.target.value) || 1500)}
                      min={500}
                      max={5000}
                      step={100}
                      aria-describedby="silence-duration-hint"
                    />
                    <span id="silence-duration-hint" className="sr-only">Tempo em milissegundos para detectar silêncio</span>
                  </div>
                </div>

                <div className="form-row">
                  <div className="form-group">
                    <label htmlFor="vad-activity-threshold">Limiar atividade: {triggerVadActivityThreshold.toFixed(3)}</label>
                    <input
                      type="range"
                      id="vad-activity-threshold"
                      aria-label={`Limiar de atividade: ${triggerVadActivityThreshold.toFixed(3)}`}
                      aria-valuemin={0.001}
                      aria-valuemax={0.1}
                      aria-valuenow={triggerVadActivityThreshold}
                      min="0.001"
                      max="0.1"
                      step="0.001"
                      value={triggerVadActivityThreshold}
                      onChange={(e) => setTriggerVadActivityThreshold(parseFloat(e.target.value))}
                      className="slider"
                    />
                  </div>

                  <div className="form-group">
                    <label htmlFor="activity-duration">Duração atividade (ms)</label>
                    <Input
                      id="activity-duration"
                      type="number"
                      value={triggerVadActivityDuration.toString()}
                      onChange={(e) => setTriggerVadActivityDuration(parseInt(e.target.value) || 200)}
                      min={50}
                      max={1000}
                      step={50}
                      aria-describedby="activity-duration-hint"
                    />
                    <span id="activity-duration-hint" className="sr-only">Tempo em milissegundos para detectar atividade de voz</span>
                  </div>
                </div>
              </div>
            )}

            <div className="modal-actions">
              <div></div>
              <div className="modal-actions-right">
                <Button variant="secondary" onClick={() => setShowTriggerModal(false)}>
                  Cancelar
                </Button>
                <Button variant="primary" onClick={handleSaveTrigger} disabled={saving}>
                  {saving ? 'Salvando...' : 'Salvar'}
                </Button>
              </div>
            </div>
          </div>
        </SimpleModal>
      )}

      <style>{`
        .form-section-header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 0.5rem;
        }

        .empty-triggers {
          color: var(--text-muted);
          font-style: italic;
          font-size: 0.9rem;
          padding: 0.5rem 0;
        }

        .triggers-list {
          display: flex;
          flex-direction: column;
          gap: 0.5rem;
          list-style: none;
          margin: 0;
          padding: 0;
        }

        .trigger-item {
          display: flex;
          justify-content: space-between;
          align-items: center;
          padding: 0.75rem;
          background: var(--bg-secondary);
          border-radius: 6px;
          border: 1px solid var(--border-color);
        }

        .trigger-item.disabled {
          opacity: 0.6;
        }

        .trigger-info {
          display: flex;
          align-items: center;
          gap: 0.5rem;
        }

        .trigger-type {
          font-weight: 500;
        }

        .trigger-detail {
          color: var(--text-muted);
          font-size: 0.9rem;
        }

        .trigger-disabled-badge {
          background: var(--warning-color, #f0ad4e);
          color: var(--warning-text, #000);
          padding: 0.125rem 0.5rem;
          border-radius: 4px;
          font-size: 0.75rem;
        }

        .trigger-actions {
          display: flex;
          gap: 0.25rem;
        }

        .trigger-action-btn {
          background: none;
          border: none;
          padding: 0.25rem 0.5rem;
          cursor: pointer;
          border-radius: 4px;
          transition: background-color 0.2s;
          font-size: 1rem;
        }

        .trigger-action-btn:hover {
          background: var(--bg-tertiary, #e0e0e0);
        }

        .trigger-action-btn:focus-visible {
          outline: 2px solid var(--primary, #007bff);
          outline-offset: 2px;
        }

        .trigger-action-btn.danger:hover {
          background: var(--danger-bg, #ffcccc);
        }

        .sr-only {
          position: absolute;
          width: 1px;
          height: 1px;
          padding: 0;
          margin: -1px;
          overflow: hidden;
          clip: rect(0, 0, 0, 0);
          white-space: nowrap;
          border: 0;
        }
      `}</style>
    </div>
  );
}
