import { useState, useEffect, useCallback } from 'react';
import { 
  GetAllVoiceProfiles,
  CreateVoiceProfileFull,
  UpdateVoiceProfileFull,
  DeleteVoiceProfile,
  SetDefaultVoiceProfile,
  PreviewVoiceSettings,
  ExportVoiceProfiles,
  ImportVoiceProfiles
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Select } from '../components/ui/Select';
import { Button } from '../components/ui/Button';
import { ttsService } from '../services/tts';
import { TTSVoice, TTSProvider } from '../services/tts/types';
import { downloadJSON, openFileDialog, generateFilename } from '../lib/exportImport';
import './VoiceProfilesPage.css';

interface VoiceProfile {
  id: number;
  name: string;
  description: string;
  provider: string;
  voice_id: string;
  rate: number;
  pitch: number;
  volume: number;
  enabled_for_agent: boolean;
  enabled_for_user: boolean;
  is_default: boolean;
}

const PROVIDERS = [
  { value: 'disabled', label: 'Desativado (aria-live)' },
  { value: 'openai', label: 'OpenAI TTS' },
  { value: 'sapi5', label: 'Windows SAPI5' },
  { value: 'webspeech', label: 'Web Speech API' },
];

export default function VoiceProfilesPage() {
  const [profiles, setProfiles] = useState<VoiceProfile[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [showModal, setShowModal] = useState(false);
  const [editingProfile, setEditingProfile] = useState<VoiceProfile | null>(null);
  const [saving, setSaving] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [availableVoices, setAvailableVoices] = useState<TTSVoice[]>([]);
  const [previewPlaying, setPreviewPlaying] = useState(false);
  
  // Form state
  const [formName, setFormName] = useState('');
  const [formDescription, setFormDescription] = useState('');
  const [formProvider, setFormProvider] = useState('openai');
  const [formVoiceId, setFormVoiceId] = useState('');
  const [formRate, setFormRate] = useState(1.0);
  const [formPitch, setFormPitch] = useState(1.0);
  const [formVolume, setFormVolume] = useState(1.0);
  const [formEnabledForAgent, setFormEnabledForAgent] = useState(true);
  const [formEnabledForUser, setFormEnabledForUser] = useState(false);
  const [formIsDefault, setFormIsDefault] = useState(false);
  const [formError, setFormError] = useState('');

  useEffect(() => {
    loadProfiles();
    loadVoices();
  }, []);

  // Escuta evento de preview de áudio
  useEffect(() => {
    const handlePreview = (data: any) => {
      console.log('[VoiceProfilesPage] Recebeu evento voice_profile:preview:', data);
      if (data.audio_base64) {
        try {
          // Decodifica e reproduz o áudio
          const audioData = atob(data.audio_base64);
          const audioArray = new Uint8Array(audioData.length);
          for (let i = 0; i < audioData.length; i++) {
            audioArray[i] = audioData.charCodeAt(i);
          }
          const blob = new Blob([audioArray], { type: 'audio/mpeg' });
          const url = URL.createObjectURL(blob);
          console.log('[VoiceProfilesPage] Blob criado, tamanho:', blob.size, 'bytes');
          
          const audio = new Audio(url);
          audio.volume = formVolume;
          audio.onended = () => {
            console.log('[VoiceProfilesPage] Áudio terminou');
            URL.revokeObjectURL(url);
            setPreviewPlaying(false);
          };
          audio.onerror = (e) => {
            console.error('[VoiceProfilesPage] Erro no áudio:', e);
            URL.revokeObjectURL(url);
            setPreviewPlaying(false);
          };
          audio.play()
            .then(() => console.log('[VoiceProfilesPage] Áudio iniciado'))
            .catch((err) => {
              console.error('[VoiceProfilesPage] Erro ao tocar:', err);
              setPreviewPlaying(false);
            });
        } catch (err) {
          console.error('[VoiceProfilesPage] Erro ao processar áudio:', err);
          setPreviewPlaying(false);
        }
      } else {
        console.warn('[VoiceProfilesPage] Evento recebido sem audio_base64');
        setPreviewPlaying(false);
      }
    };

    console.log('[VoiceProfilesPage] Registrando listener para voice_profile:preview');
    EventsOn('voice_profile:preview', handlePreview);
    return () => {
      console.log('[VoiceProfilesPage] Removendo listener para voice_profile:preview');
      EventsOff('voice_profile:preview');
    };
  }, [formVolume]);

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
      const result = await GetAllVoiceProfiles();
      setProfiles(result || []);
    } catch (error) {
      console.error('Erro ao carregar perfis de voz:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadVoices = async () => {
    try {
      const voices = await ttsService.getVoices();
      setAvailableVoices(voices);
    } catch (error) {
      console.error('Erro ao carregar vozes:', error);
    }
  };

  const getVoicesForProvider = useCallback((provider: string) => {
    const providerMap: Record<string, TTSProvider> = {
      'openai': TTSProvider.OPENAI,
      'sapi5': TTSProvider.SAPI5,
      'webspeech': TTSProvider.WEBSPEECH,
    };
    
    const ttsProvider = providerMap[provider];
    if (!ttsProvider) return [];
    
    return availableVoices.filter(v => v.provider === ttsProvider);
  }, [availableVoices]);

  const openNewForm = () => {
    setEditingProfile(null);
    setFormName('');
    setFormDescription('');
    setFormProvider('openai');
    setFormVoiceId('');
    setFormRate(1.0);
    setFormPitch(1.0);
    setFormVolume(1.0);
    setFormEnabledForAgent(true);
    setFormEnabledForUser(false);
    setFormIsDefault(false);
    setFormError('');
    setShowModal(true);
  };

  const openEditForm = (profile: VoiceProfile) => {
    setEditingProfile(profile);
    setFormName(profile.name);
    setFormDescription(profile.description || '');
    setFormProvider(profile.provider);
    setFormVoiceId(profile.voice_id || '');
    setFormRate(profile.rate);
    setFormPitch(profile.pitch);
    setFormVolume(profile.volume);
    setFormEnabledForAgent(profile.enabled_for_agent);
    setFormEnabledForUser(profile.enabled_for_user);
    setFormIsDefault(profile.is_default);
    setFormError('');
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!formName.trim()) {
      setFormError('Nome é obrigatório');
      return;
    }
    if (!formProvider) {
      setFormError('Provider é obrigatório');
      return;
    }
    // VoiceID é obrigatório se TTS estiver ativado para assistente ou usuário
    if (formProvider !== 'disabled' && !formVoiceId && (formEnabledForAgent || formEnabledForUser)) {
      setFormError('Selecione uma voz para ativar TTS');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      if (editingProfile) {
        await UpdateVoiceProfileFull(
          editingProfile.id,
          formName,
          formDescription,
          formProvider,
          formVoiceId || '',
          formRate,
          formPitch,
          formVolume,
          formEnabledForAgent,
          formEnabledForUser,
          formIsDefault
        );
      } else {
        await CreateVoiceProfileFull(
          formName,
          formDescription,
          formProvider,
          formVoiceId || '',
          formRate,
          formPitch,
          formVolume,
          formEnabledForAgent,
          formEnabledForUser,
          formIsDefault
        );
      }
      await loadProfiles();
      setShowModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (profile: VoiceProfile) => {
    if (!confirm(`Tem certeza que deseja excluir o perfil "${profile.name}"?`)) return;
    
    try {
      await DeleteVoiceProfile(profile.id);
      setProfiles(prev => prev.filter(p => p.id !== profile.id));
    } catch (error) {
      console.error('Erro ao deletar perfil:', error);
      alert('Erro ao deletar perfil de voz');
    }
  };

  const handleSetDefault = async (profile: VoiceProfile) => {
    try {
      await SetDefaultVoiceProfile(profile.id);
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
      await Promise.all(Array.from(selectedIds).map(id => DeleteVoiceProfile(Number(id))));
      setProfiles(prev => prev.filter(p => !selectedIds.has(p.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar perfis:', error);
      alert('Erro ao deletar perfis');
    }
  };

  const handleExport = async () => {
    const idsToExport = selectedIds.size > 0 
      ? Array.from(selectedIds).map(id => Number(id))
      : profiles.map(p => p.id);
    
    if (idsToExport.length === 0) {
      alert('Nenhum perfil para exportar');
      return;
    }

    try {
      const jsonData = await ExportVoiceProfiles(idsToExport);
      const filename = generateFilename('voice-profiles');
      downloadJSON(jsonData, filename);
    } catch (error) {
      console.error('Erro ao exportar perfis:', error);
      alert('Erro ao exportar perfis de voz');
    }
  };

  const handleImport = async () => {
    try {
      const jsonData = await openFileDialog('.json');
      const result = await ImportVoiceProfiles(jsonData);
      
      if (result.success) {
        alert(`Importação concluída: ${result.message}`);
        loadProfiles();
      } else {
        alert(`Importação parcial: ${result.message}\nErros: ${result.errors?.join(', ')}`);
        loadProfiles();
      }
    } catch (error) {
      console.error('Erro ao importar perfis:', error);
      alert('Erro ao importar perfis de voz');
    }
  };

  const handlePreview = async () => {
    if (!formProvider || formProvider === 'disabled') {
      setFormError('Selecione um provider para testar');
      return;
    }
    if (!formVoiceId) {
      setFormError('Selecione uma voz para testar');
      return;
    }

    console.log('[VoiceProfilesPage] Iniciando preview:', { formProvider, formVoiceId, formRate, formVolume });
    setPreviewPlaying(true);
    setFormError('');
    
    try {
      const sampleText = `Olá! Este é um teste do perfil de voz ${formName || 'novo'}. A velocidade está em ${formRate.toFixed(1)} e o volume em ${(formVolume * 100).toFixed(0)} porcento.`;
      console.log('[VoiceProfilesPage] Chamando PreviewVoiceSettings...');
      
      await PreviewVoiceSettings(
        formProvider,
        formVoiceId,
        formRate,
        formPitch,
        formVolume,
        sampleText
      );
      
      console.log('[VoiceProfilesPage] PreviewVoiceSettings retornou, aguardando evento...');
      // O evento voice_profile:preview será emitido pelo backend
    } catch (error: any) {
      console.error('[VoiceProfilesPage] Erro ao testar voz:', error);
      setFormError('Erro ao testar voz: ' + (error.message || error));
      setPreviewPlaying(false);
    }
  };

  const filteredProfiles = profiles.filter(profile =>
    profile.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    (profile.description || '').toLowerCase().includes(searchTerm.toLowerCase()) ||
    profile.provider.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const columns: DataGridColumn<VoiceProfile>[] = [
    { 
      key: 'name', 
      label: 'Nome',
      format: (value, item) => item.is_default ? `⭐ ${value}` : value,
    },
    { 
      key: 'provider', 
      label: 'Provider',
      width: '120px',
      format: (value) => {
        const provider = PROVIDERS.find(p => p.value === value);
        return provider?.label || value;
      }
    },
    { 
      key: 'voice_id', 
      label: 'Voz',
      width: '120px',
    },
    { 
      key: 'rate', 
      label: 'Velocidade',
      width: '100px',
      format: (value) => `${value.toFixed(1)}x`
    },
    { 
      key: 'volume', 
      label: 'Volume',
      width: '80px',
      format: (value) => `${Math.round(value * 100)}%`
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
    {
      key: 'export',
      label: selectedIds.size > 0 
        ? `Exportar (${selectedIds.size})`
        : 'Exportar Tudo',
      icon: '📤',
      onClick: handleExport,
      variant: 'secondary',
    },
    {
      key: 'import',
      label: 'Importar',
      icon: '📥',
      onClick: handleImport,
      variant: 'secondary',
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

  const voicesForProvider = getVoicesForProvider(formProvider);

  if (loading) {
    return (
      <div className="voice-profiles-page">
        <Toolbar left={<h1 className="page-toolbar__title">Perfis de Voz</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando perfis de voz...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="voice-profiles-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Perfis de Voz</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar perfis..."
      />

      <div className="page-content">
        <div className="info-box">
          <span>🔊 Configure perfis de voz para usar no chat. O perfil marcado com ⭐ é o padrão.</span>
        </div>

        <DataGrid
          items={filteredProfiles}
          columns={columns}
          label="Lista de Perfis de Voz"
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
        />

        {profiles.length === 0 && (
          <div className="empty-state">
            <p>Nenhum perfil de voz criado ainda.</p>
            <p>Clique em "Novo Perfil" para criar seu primeiro perfil de voz.</p>
          </div>
        )}
      </div>

      {showModal && (
        <SimpleModal
          isOpen={showModal}
          onClose={() => setShowModal(false)}
          title={editingProfile ? 'Editar Perfil de Voz' : 'Novo Perfil de Voz'}
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
                placeholder="Ex: Narrador Formal"
                autoFocus
              />
            </div>

            <div className="form-group">
              <label htmlFor="description">Descrição</label>
              <Textarea
                id="description"
                value={formDescription}
                onChange={(e) => setFormDescription(e.target.value)}
                placeholder="Descrição do perfil e seu uso..."
                rows={2}
              />
            </div>

            <div className="form-row">
              <div className="form-group">
                <label htmlFor="provider">Provider *</label>
                <Select
                  id="provider"
                  value={formProvider}
                  onChange={(e) => {
                    setFormProvider(e.target.value);
                    setFormVoiceId(''); // Reset voice when provider changes
                  }}
                  options={PROVIDERS}
                />
              </div>

              <div className="form-group">
                <label htmlFor="voice">Voz *</label>
                <Select
                  id="voice"
                  value={formVoiceId}
                  onChange={(e) => setFormVoiceId(e.target.value)}
                  options={[
                    { value: '', label: 'Selecione uma voz...' },
                    ...voicesForProvider.map(v => ({
                      value: v.id,
                      label: v.name + (v.description ? ` - ${v.description}` : '')
                    }))
                  ]}
                />
              </div>
            </div>

            <div className="form-row">
              <div className="form-group">
                <label htmlFor="rate">Velocidade: {formRate.toFixed(1)}x</label>
                <input
                  type="range"
                  id="rate"
                  min="0.25"
                  max="4.0"
                  step="0.1"
                  value={formRate}
                  onChange={(e) => setFormRate(parseFloat(e.target.value))}
                  className="slider"
                />
              </div>

              <div className="form-group">
                <label htmlFor="volume">Volume: {Math.round(formVolume * 100)}%</label>
                <input
                  type="range"
                  id="volume"
                  min="0"
                  max="1"
                  step="0.05"
                  value={formVolume}
                  onChange={(e) => setFormVolume(parseFloat(e.target.value))}
                  className="slider"
                />
              </div>
            </div>

            {formProvider === 'webspeech' && (
              <div className="form-group">
                <label htmlFor="pitch">Tom: {formPitch.toFixed(1)}</label>
                <input
                  type="range"
                  id="pitch"
                  min="0.5"
                  max="2.0"
                  step="0.1"
                  value={formPitch}
                  onChange={(e) => setFormPitch(parseFloat(e.target.value))}
                  className="slider"
                />
              </div>
            )}

            {/* Opções de ativação */}
            <div className="form-section">
              <label className="form-section-title">Ativar TTS para:</label>
              <div className="checkbox-row">
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={formEnabledForAgent}
                    onChange={(e) => setFormEnabledForAgent(e.target.checked)}
                    disabled={formProvider === 'disabled'}
                  />
                  <span>Assistente (mensagens do agente)</span>
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={formEnabledForUser}
                    onChange={(e) => setFormEnabledForUser(e.target.checked)}
                    disabled={formProvider === 'disabled'}
                  />
                  <span>Usuário (leitura de mensagens enviadas)</span>
                </label>
              </div>
            </div>

            <div className="form-group checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={formIsDefault}
                  onChange={(e) => setFormIsDefault(e.target.checked)}
                />
                <span>Definir como perfil padrão</span>
              </label>
            </div>

            <div className="modal-actions">
              <Button 
                variant="secondary" 
                onClick={handlePreview}
                disabled={previewPlaying || (formProvider !== 'disabled' && !formVoiceId)}
              >
                {previewPlaying ? '🔊 Tocando...' : '🔊 Testar Voz'}
              </Button>
              <div className="modal-actions-right">
                <Button variant="secondary" onClick={() => setShowModal(false)}>
                  Cancelar
                </Button>
                <Button variant="primary" onClick={handleSave} disabled={saving}>
                  {saving ? 'Salvando...' : 'Salvar'}
                </Button>
              </div>
            </div>
          </div>
        </SimpleModal>
      )}
    </div>
  );
}
