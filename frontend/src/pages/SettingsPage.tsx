import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { GetConfig, SaveSettings, ResetConfig, ResetDatabase, TestConnectionWithModels, GetDefaultVoiceProfile, GetDefaultInteractionProfile, GetDefaultChatProfile, SetDefaultVoiceProfile, SetDefaultInteractionProfile, SetDefaultChatProfile } from '../../wailsjs/go/main/App';
import { llm } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Input, Button } from '../components';
import { VoiceProfilePicker, VoiceProfilePickerRef, ChatProfilePicker, ChatProfilePickerRef } from '../components/pickers';
import { InteractionProfilePicker, InteractionProfilePickerRef } from '../components/pickers/InteractionProfilePicker';
import { useAnnouncer } from '../hooks/useAnnouncer';
import './SettingsPage.css';

interface FormData {
  apiKey: string;
  apiBaseURL: string;
  responseTimeout: number;
}

export default function SettingsPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { handleDatabaseReset } = useChatStore();
  const { announce } = useAnnouncer();
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  
  // Refs para os pickers de perfis
  const voiceProfilePickerRef = useRef<VoiceProfilePickerRef>(null);
  const interactionProfilePickerRef = useRef<InteractionProfilePickerRef>(null);
  const chatProfilePickerRef = useRef<ChatProfilePickerRef>(null);
  
  // Estados dos perfis padrão
  const [defaultVoiceProfileId, setDefaultVoiceProfileId] = useState<number>(0);
  const [defaultInteractionProfileId, setDefaultInteractionProfileId] = useState<number>(0);
  const [defaultChatProfileId, setDefaultChatProfileId] = useState<number>(0);
  
  const [formData, setFormData] = useState<FormData>({
    apiKey: '',
    apiBaseURL: 'https://api.openai.com/v1',
    responseTimeout: 180,
  });

  // Carrega configuração do backend
  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      
      // Carrega configuração e perfis padrão em paralelo
      const [config, defaultVoiceProfile, defaultInteractionProfile, defaultChatProfile] = await Promise.all([
        GetConfig(),
        GetDefaultVoiceProfile().catch(() => null),
        GetDefaultInteractionProfile().catch(() => null),
        GetDefaultChatProfile().catch(() => null),
      ]);
      
      if (config) {
        setFormData({
          apiKey: config.api_key || '',
          apiBaseURL: config.api_base_url || 'https://api.openai.com/v1',
          responseTimeout: config.response_timeout || 180,
        });
      }
      
      // Define perfis padrão
      if (defaultVoiceProfile) {
        setDefaultVoiceProfileId(defaultVoiceProfile.id);
      }
      if (defaultInteractionProfile) {
        setDefaultInteractionProfileId(defaultInteractionProfile.id);
      }
      if (defaultChatProfile) {
        setDefaultChatProfileId(defaultChatProfile.id);
      }
    } catch (error) {
      console.error('Erro ao carregar configuração:', error);
      addToast('Erro ao carregar configuração', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (field: keyof FormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const settings = llm.SettingsInput.createFrom({
        api_key: formData.apiKey,
        api_base_url: formData.apiBaseURL,
        response_timeout: formData.responseTimeout,
      });
      await SaveSettings(settings);
      addToast('Configurações salvas com sucesso!', 'success');
      announce('Configurações salvas');
    } catch (error: any) {
      console.error('Erro ao salvar configurações:', error);
      addToast(error.message || 'Erro ao salvar configurações', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleChatProfileChange = async (profileId: number) => {
    try {
      await SetDefaultChatProfile(profileId);
      setDefaultChatProfileId(profileId);
      addToast('Perfil de conversa padrão atualizado!', 'success');
      announce('Perfil de conversa padrão atualizado');
    } catch (error: any) {
      console.error('Erro ao definir perfil de conversa padrão:', error);
      addToast(error.message || 'Erro ao definir perfil de conversa padrão', 'error');
    }
  };

  const handleVoiceProfileChange = async (profileId: number) => {
    try {
      await SetDefaultVoiceProfile(profileId);
      setDefaultVoiceProfileId(profileId);
      addToast('Perfil de voz padrão atualizado!', 'success');
      announce('Perfil de voz padrão atualizado');
    } catch (error: any) {
      console.error('Erro ao definir perfil de voz padrão:', error);
      addToast(error.message || 'Erro ao definir perfil de voz padrão', 'error');
    }
  };

  const handleInteractionProfileChange = async (profileId: number) => {
    try {
      await SetDefaultInteractionProfile(profileId);
      setDefaultInteractionProfileId(profileId);
      addToast('Perfil de interação padrão atualizado!', 'success');
      announce('Perfil de interação padrão atualizado');
    } catch (error: any) {
      console.error('Erro ao definir perfil de interação padrão:', error);
      addToast(error.message || 'Erro ao definir perfil de interação padrão', 'error');
    }
  };

  const handleTestConnection = async () => {
    setTesting(true);
    try {
      const models = await TestConnectionWithModels();
      setAvailableModels(models);
      addToast(`Conexão OK! ${models.length} modelos disponíveis.`, 'success');
      announce(`Conexão estabelecida. ${models.length} modelos disponíveis.`);
    } catch (error: any) {
      console.error('Erro ao testar conexão:', error);
      addToast(error.message || 'Erro ao conectar com a API', 'error');
      announce('Falha na conexão');
    } finally {
      setTesting(false);
    }
  };

  const handleResetConfig = async () => {
    if (!confirm('Tem certeza que deseja resetar as configurações?\n\nIsso irá apagar o arquivo config.json e restaurar os valores padrão.')) {
      return;
    }
    
    try {
      await ResetConfig();
      addToast('Configurações resetadas com sucesso!', 'success');
      announce('Configurações resetadas');
      await loadConfig(); // Recarrega os valores padrão
    } catch (error: any) {
      console.error('Erro ao resetar configurações:', error);
      addToast(error.message || 'Erro ao resetar configurações', 'error');
    }
  };

  const handleResetDatabase = async () => {
    if (!confirm('⚠️ ATENÇÃO: Tem certeza que deseja apagar o banco de dados?\n\nIsso irá REMOVER PERMANENTEMENTE:\n- Todas as conversas\n- Todas as memórias\n- Todos os FAQs\n- Todos os perfis\n- Todas as conexões OAuth\n\nEsta ação NÃO pode ser desfeita!')) {
      return;
    }
    
    // Segunda confirmação
    if (!confirm('Esta é sua ÚLTIMA CHANCE!\n\nDigite OK para confirmar a exclusão de todos os dados.')) {
      return;
    }
    
    try {
      await ResetDatabase();
      handleDatabaseReset(); // Atualiza o frontend
      addToast('Banco de dados resetado com sucesso!', 'success');
      announce('Banco de dados resetado');
    } catch (error: any) {
      console.error('Erro ao resetar banco de dados:', error);
      addToast(error.message || 'Erro ao resetar banco de dados', 'error');
    }
  };

  if (loading) {
    return (
      <div className="settings-page">
        <div className="settings-loading">Carregando configurações...</div>
      </div>
    );
  }

  return (
    <div className="settings-page">
      <div className="settings-header">
        <h1>{t('settings.title', 'Configurações')}</h1>
        <p>{t('settings.subtitle', 'Configure as opções do assistente')}</p>
      </div>

      <div className="settings-content">
        {/* API Section */}
        <section className="settings-section">
          <h2>🔑 API</h2>
          <div className="settings-fields">
            <Input
              label="API Key"
              type="password"
              value={formData.apiKey}
              onChange={(e) => handleChange('apiKey', e.target.value)}
              placeholder="sk-..."
              fullWidth
            />

            <Input
              label="Base URL"
              value={formData.apiBaseURL}
              onChange={(e) => handleChange('apiBaseURL', e.target.value)}
              placeholder="https://api.openai.com/v1"
              fullWidth
            />
            
            <Button 
              variant="outline" 
              onClick={handleTestConnection} 
              loading={testing}
              disabled={!formData.apiKey}
            >
              Testar Conexão
            </Button>
            
            {availableModels.length > 0 && (
              <p className="settings-field-hint">
                ✅ {availableModels.length} modelos disponíveis na API
              </p>
            )}

            <Input
              label="Timeout de Resposta (segundos)"
              type="number"
              min="30"
              max="600"
              step="10"
              value={formData.responseTimeout}
              onChange={(e) => handleChange('responseTimeout', parseInt(e.target.value) || 180)}
              fullWidth
            />
            <p className="settings-field-hint">
              Tempo máximo para aguardar o início da resposta do modelo (padrão: 180s).
            </p>
          </div>
        </section>

        {/* Default Profiles Section */}
        <section className="settings-section">
          <h2>🎭 Perfis Padrão</h2>
          <p className="settings-section-description">
            Defina os perfis que serão usados por padrão em novas conversas.
            Configure modelo, temperatura e mais nos perfis de conversa.
          </p>
          <div className="settings-fields settings-profiles-grid">
            <div className="settings-profile-item">
              <label className="settings-label">Perfil de Conversa</label>
              <p className="settings-field-hint">Modelo e parâmetros</p>
              <ChatProfilePicker
                ref={chatProfilePickerRef}
                value={defaultChatProfileId}
                onChange={handleChatProfileChange}
                label="Perfil de Conversa Padrão"
                maxWidth="100%"
                onAnnounce={announce}
              />
            </div>
            
            <div className="settings-profile-item">
              <label className="settings-label">Perfil de Voz</label>
              <p className="settings-field-hint">Síntese de voz (TTS)</p>
              <VoiceProfilePicker
                ref={voiceProfilePickerRef}
                value={defaultVoiceProfileId}
                onChange={handleVoiceProfileChange}
                label="Perfil de Voz Padrão"
                maxWidth="100%"
                onAnnounce={announce}
              />
            </div>
            
            <div className="settings-profile-item">
              <label className="settings-label">Perfil de Interação</label>
              <p className="settings-field-hint">Reconhecimento de voz (STT)</p>
              <InteractionProfilePicker
                ref={interactionProfilePickerRef}
                value={defaultInteractionProfileId}
                onChange={handleInteractionProfileChange}
                label="Perfil de Interação Padrão"
                maxWidth="100%"
                onAnnounce={announce}
              />
            </div>
          </div>
        </section>

        {/* Danger Zone */}
        <section className="settings-section settings-danger">
          <h2>⚠️ Zona de Perigo</h2>
          <p className="settings-danger-warning">
            As ações abaixo são irreversíveis. Tenha certeza antes de prosseguir.
          </p>
          <div className="settings-danger-actions">
            <div className="settings-danger-item">
              <div>
                <strong>Resetar Configurações</strong>
                <p>Apaga o arquivo config.json e restaura os valores padrão.</p>
              </div>
              <Button variant="outline" onClick={handleResetConfig}>
                Resetar Config
              </Button>
            </div>
            
            <div className="settings-danger-item">
              <div>
                <strong>Apagar Banco de Dados</strong>
                <p>Remove permanentemente todas as conversas, memórias, FAQs e perfis.</p>
              </div>
              <Button variant="danger" onClick={handleResetDatabase}>
                Apagar Tudo
              </Button>
            </div>
          </div>
        </section>
      </div>

      <div className="settings-footer">
        <Button variant="ghost" onClick={loadConfig} disabled={saving}>
          Recarregar
        </Button>
        <Button onClick={handleSave} loading={saving}>
          Salvar Configurações
        </Button>
      </div>
    </div>
  );
}
