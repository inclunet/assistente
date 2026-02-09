import { useState, useEffect } from 'react';
import { 
  GetChatProfiles,
  CreateChatProfile,
  UpdateChatProfile,
  DeleteChatProfile,
  SetDefaultChatProfile,
  GetModels
} from '../../wailsjs/go/main/App';
import { database } from '../../wailsjs/go/models';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Select } from '../components/ui/Select';
import { Button } from '../components/ui/Button';
import { useGridFocus } from '../hooks/useGridFocus';
import './ChatProfilesPage.css';

type ChatProfile = database.ChatProfile;

const ICONS = [
  { value: '💬', label: '💬 Chat' },
  { value: '🏠', label: '🏠 Casa' },
  { value: '💻', label: '💻 Código' },
  { value: '📚', label: '📚 Estudo' },
  { value: '🎨', label: '🎨 Criativo' },
  { value: '🔬', label: '🔬 Pesquisa' },
  { value: '📝', label: '📝 Escrita' },
  { value: '🤖', label: '🤖 Robô' },
  { value: '⚡', label: '⚡ Rápido' },
  { value: '🎯', label: '🎯 Focado' },
];

export default function ChatProfilesPage() {
  const [profiles, setProfiles] = useState<ChatProfile[]>([]);
  const [models, setModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [showModal, setShowModal] = useState(false);
  const [editingProfile, setEditingProfile] = useState<ChatProfile | null>(null);
  const [saving, setSaving] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const { focusFirstCell, handleGridReady } = useGridFocus();
  
  // Form state
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formIcon, setFormIcon] = useState('💬');
  const [formModel, setFormModel] = useState('');
  const [formTemperature, setFormTemperature] = useState(0.7);
  const [formMaxTokens, setFormMaxTokens] = useState(4096);
  const [formTopP, setFormTopP] = useState(1.0);
  const [formResponseTimeout, setFormResponseTimeout] = useState(180);
  const [formSystemPrompt, setFormSystemPrompt] = useState('');
  const [formSystemPromptPosition, setFormSystemPromptPosition] = useState('after');
  const [formIsDefault, setFormIsDefault] = useState(false);
  const [formEnableThinking, setFormEnableThinking] = useState(false);
  const [formError, setFormError] = useState('');

  useEffect(() => {
    loadProfiles();
    loadModels();
  }, []);

  // Escuta eventos de atualização
  useEffect(() => {
    const handleCreated = () => loadProfiles();
    const handleUpdated = () => loadProfiles();
    const handleDeleted = () => loadProfiles();

    EventsOn('chat:profile:created', handleCreated);
    EventsOn('chat:profile:updated', handleUpdated);
    EventsOn('chat:profile:deleted', handleDeleted);

    return () => {
      EventsOff('chat:profile:created');
      EventsOff('chat:profile:updated');
      EventsOff('chat:profile:deleted');
    };
  }, []);

  // Atalho Ctrl+N para novo perfil
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.key.toLowerCase() === 'n') {
        event.preventDefault();
        openNewForm();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadProfiles = async () => {
    setLoading(true);
    try {
      const result = await GetChatProfiles();
      setProfiles(result || []);
    } catch (error) {
      console.error('Erro ao carregar perfis de conversa:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadModels = async () => {
    try {
      const result = await GetModels();
      setModels(result || []);
    } catch (error) {
      console.error('Erro ao carregar modelos:', error);
    }
  };

  const openNewForm = () => {
    setEditingProfile(null);
    setFormName('');
    setFormDescription('');
    setFormIcon('💬');
    setFormModel('');
    setFormTemperature(0.7);
    setFormMaxTokens(4096);
    setFormTopP(1.0);
    setFormResponseTimeout(180);
    setFormSystemPrompt('');
    setFormSystemPromptPosition('after');
    setFormIsDefault(false);
    setFormEnableThinking(false);
    setFormError('');
    setShowModal(true);
  };

  const openEditForm = (profile: ChatProfile) => {
    setEditingProfile(profile);
    setFormName(profile.name);
    setFormDescription(profile.description || '');
    setFormIcon(profile.icon || '💬');
    setFormModel(profile.model || '');
    setFormTemperature(profile.temperature || 0.7);
    setFormMaxTokens(profile.max_tokens || 4096);
    setFormTopP(profile.top_p || 1.0);
    setFormResponseTimeout(profile.response_timeout || 180);
    setFormSystemPrompt(profile.system_prompt || '');
    setFormSystemPromptPosition(profile.system_prompt_position || 'after');
    setFormIsDefault(profile.is_default);
    setFormEnableThinking(profile.enable_thinking || false);
    setFormError('');
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!formName.trim()) {
      setFormError('Nome é obrigatório');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      const profileData: Partial<ChatProfile> = {
        name: formName.trim(),
        description: formDescription.trim(),
        icon: formIcon,
        model: formModel,
        temperature: formTemperature,
        max_tokens: formMaxTokens,
        top_p: formTopP,
        response_timeout: formResponseTimeout,
        system_prompt: formSystemPrompt.trim(),
        system_prompt_position: formSystemPromptPosition,
        is_default: formIsDefault,
        enable_thinking: formEnableThinking,
      };

      if (editingProfile) {
        await UpdateChatProfile(editingProfile.id, profileData as ChatProfile);
      } else {
        await CreateChatProfile(profileData as ChatProfile);
      }
      await loadProfiles();
      setShowModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (profile: ChatProfile) => {
    if (profile.is_default) {
      alert('Não é possível excluir o perfil padrão');
      return;
    }
    if (!confirm(`Tem certeza que deseja excluir o perfil "${profile.name}"?`)) return;
    
    try {
      await DeleteChatProfile(profile.id);
      setProfiles(prev => prev.filter(p => p.id !== profile.id));
    } catch (error) {
      console.error('Erro ao deletar perfil:', error);
      alert('Erro ao deletar perfil de conversa');
    }
  };

  const handleSetDefault = async (profile: ChatProfile) => {
    try {
      await SetDefaultChatProfile(profile.id);
      await loadProfiles();
    } catch (error) {
      console.error('Erro ao definir perfil padrão:', error);
      alert('Erro ao definir perfil padrão');
    }
  };

  const handleDeleteSelected = async () => {
    if (selectedIds.size === 0) return;
    
    // Verifica se algum selecionado é o padrão
    const hasDefault = profiles.some(p => selectedIds.has(p.id) && p.is_default);
    if (hasDefault) {
      alert('Não é possível excluir o perfil padrão. Defina outro perfil como padrão primeiro.');
      return;
    }
    
    if (!confirm(`Tem certeza que deseja excluir ${selectedIds.size} perfil(s)?`)) return;

    try {
      await Promise.all(Array.from(selectedIds).map(id => DeleteChatProfile(Number(id))));
      setProfiles(prev => prev.filter(p => !selectedIds.has(p.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar perfis:', error);
      alert('Erro ao deletar perfis');
    }
  };

  const filteredProfiles = profiles.filter(profile =>
    profile.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    (profile.description || '').toLowerCase().includes(searchTerm.toLowerCase())
  );

  const columns: DataGridColumn<ChatProfile>[] = [
    { 
      key: 'name', 
      label: 'Nome',
      format: (value, item) => `${item.icon || '💬'} ${item.is_default ? '⭐ ' : ''}${value}`,
    },
    { 
      key: 'model', 
      label: 'Modelo',
      width: '150px',
      format: (value) => value || '(padrão)',
    },
    { 
      key: 'temperature', 
      label: 'Temperatura',
      width: '100px',
      format: (value) => value?.toFixed(1) || '0.7',
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

  const toolbarActions: ToolbarAction[] = [
    {
      key: 'new',
      label: 'Novo Perfil',
      icon: '➕',
      onClick: openNewForm,
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

  if (loading) {
    return (
      <div className="chat-profiles-page">
        <Toolbar left={<h1 className="page-toolbar__title">Perfis de Conversa</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando perfis de conversa...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="chat-profiles-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Perfis de Conversa</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar perfis..."
        onFocusGrid={focusFirstCell}
      />

      <div className="page-content">
        <div className="info-box">
          <span>💬 Configure perfis de conversa com modelo, parâmetros e ferramentas. O perfil marcado com ⭐ é o padrão para novas conversas.</span>
        </div>

        <DataGrid
          items={filteredProfiles}
          columns={columns}
          label="Lista de Perfis de Conversa"
          getItemId={(profile) => profile.id}
          onActivate={(profile) => openEditForm(profile)}
          onDelete={(profile) => handleDelete(profile)}
          onCellAction={(profile, column) => {
            if (column.key === 'edit') {
              openEditForm(profile);
            } else if (column.key === 'delete') {
              handleDelete(profile);
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
            <p>Nenhum perfil de conversa criado ainda.</p>
            <p>Clique em "Novo Perfil" para criar seu primeiro perfil.</p>
          </div>
        )}
      </div>

      {showModal && (
        <SimpleModal
          isOpen={showModal}
          onClose={() => setShowModal(false)}
          title={editingProfile ? 'Editar Perfil de Conversa' : 'Novo Perfil de Conversa'}
          size="xl"
        >
          <div className="modal-form chat-profile-form">
            {formError && (
              <div className="form-error">{formError}</div>
            )}

            {/* Seção: Identificação */}
            <fieldset className="form-fieldset">
              <legend>Identificação</legend>
              
              <div className="form-row">
                <div className="form-group" style={{ flex: 2 }}>
                  <label htmlFor="name">Nome *</label>
                  <Input
                    id="name"
                    value={formName}
                    onChange={(e) => setFormName(e.target.value)}
                    placeholder="Ex: Programação Python"
                    autoFocus
                  />
                </div>

                <div className="form-group" style={{ flex: 1 }}>
                  <label htmlFor="icon">Ícone</label>
                  <Select
                    id="icon"
                    value={formIcon}
                    onChange={(e) => setFormIcon(e.target.value)}
                    options={ICONS}
                  />
                </div>
              </div>

              <div className="form-group">
                <label htmlFor="description">Descrição</label>
                <Input
                  id="description"
                  value={formDescription}
                  onChange={(e) => setFormDescription(e.target.value)}
                  placeholder="Descrição breve do uso deste perfil..."
                />
              </div>
            </fieldset>

            {/* Seção: Modelo e Parâmetros */}
            <fieldset className="form-fieldset">
              <legend>Modelo e Parâmetros</legend>
              
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="model">Modelo</label>
                  <Select
                    id="model"
                    value={formModel}
                    onChange={(e) => setFormModel(e.target.value)}
                    options={[
                      { value: '', label: '(usar modelo padrão)' },
                      ...models.map(m => ({ value: m, label: m }))
                    ]}
                  />
                </div>

                <div className="form-group">
                  <label htmlFor="timeout">Timeout (s)</label>
                  <Input
                    id="timeout"
                    type="number"
                    value={formResponseTimeout}
                    onChange={(e) => setFormResponseTimeout(Number(e.target.value))}
                    min={10}
                    max={600}
                  />
                </div>
              </div>

              <div className="form-row form-row-sliders">
                <div className="form-group">
                  <label htmlFor="temperature">Temperatura: {formTemperature.toFixed(1)}</label>
                  <input
                    type="range"
                    id="temperature"
                    min="0"
                    max="2"
                    step="0.1"
                    value={formTemperature}
                    onChange={(e) => setFormTemperature(parseFloat(e.target.value))}
                    className="slider"
                  />
                  <span className="slider-hint">0 = determinístico, 2 = criativo</span>
                </div>

                <div className="form-group">
                  <label htmlFor="topP">Top P: {formTopP.toFixed(1)}</label>
                  <input
                    type="range"
                    id="topP"
                    min="0"
                    max="1"
                    step="0.1"
                    value={formTopP}
                    onChange={(e) => setFormTopP(parseFloat(e.target.value))}
                    className="slider"
                  />
                </div>

                <div className="form-group">
                  <label htmlFor="maxTokens">Max Tokens</label>
                  <Input
                    id="maxTokens"
                    type="number"
                    value={formMaxTokens}
                    onChange={(e) => setFormMaxTokens(Number(e.target.value))}
                    min={1}
                    max={128000}
                  />
                </div>
              </div>

              <div className="form-group checkbox-group">
                <label>
                  <input
                    type="checkbox"
                    checked={formEnableThinking}
                    onChange={(e) => setFormEnableThinking(e.target.checked)}
                  />
                  <span>Habilitar reasoning/thinking (Ollama)</span>
                </label>
                <p className="form-hint">
                  Envia parâmetro <code>think=true</code> para modelos que suportam (Ollama, QwQ, etc). 
                  Exibe a cadeia de pensamento do modelo antes da resposta final.
                </p>
              </div>
            </fieldset>

            {/* Seção: System Prompt */}
            <fieldset className="form-fieldset">
              <legend>System Prompt</legend>
              
              <div className="form-group">
                <label htmlFor="systemPrompt">Custom system prompt (leave empty to use default)</label>
                <Textarea
                  id="systemPrompt"
                  value={formSystemPrompt}
                  onChange={(e) => setFormSystemPrompt(e.target.value)}
                  placeholder="Leave empty for the default assistant prompt, or customize it here..."
                  rows={4}
                />
              </div>

              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="promptPosition">Custom prompt position</label>
                  <Select
                    id="promptPosition"
                    value={formSystemPromptPosition}
                    onChange={(e) => setFormSystemPromptPosition(e.target.value)}
                    options={[
                      { value: 'before', label: 'Before (override default)' },
                      { value: 'after', label: 'After (extend default)' },
                    ]}
                  />
                </div>
              </div>

            </fieldset>

            {/* Opção de perfil padrão */}
            <div className="form-group checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={formIsDefault}
                  onChange={(e) => setFormIsDefault(e.target.checked)}
                />
                <span>Definir como perfil padrão para novas conversas</span>
              </label>
            </div>

            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setShowModal(false)}>
                Cancelar
              </Button>
              <Button variant="primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Salvando...' : 'Salvar'}
              </Button>
            </div>
          </div>
        </SimpleModal>
      )}
    </div>
  );
}
