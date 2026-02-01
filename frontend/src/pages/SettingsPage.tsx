import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { GetConfig, SaveSettings, ResetConfig, ResetDatabase, TestConnectionWithModels, GetDefaultVoiceProfile, GetDefaultInteractionProfile, SetDefaultVoiceProfile, SetDefaultInteractionProfile } from '../../wailsjs/go/main/App';
import { llm } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Input, Select, Checkbox, Button } from '../components';
import { VoiceProfilePicker, VoiceProfilePickerRef } from '../components/pickers';
import { InteractionProfilePicker, InteractionProfilePickerRef } from '../components/pickers/InteractionProfilePicker';
import { useAnnouncer } from '../hooks/useAnnouncer';
import './SettingsPage.css';

interface FormData {
  apiKey: string;
  apiBaseURL: string;
  braveApiKey: string;
  chatModel: string;
  chatTemperature: number;
  chatMaxTokens: number;
  chatTopP: number;
  embeddingsModel: string;
  embeddingsDimensions: number;
  useTools: boolean;
  showInternalMessages: boolean;
  imageModel: string;
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
  
  // Estados dos perfis padrão
  const [defaultVoiceProfileId, setDefaultVoiceProfileId] = useState<number>(0);
  const [defaultInteractionProfileId, setDefaultInteractionProfileId] = useState<number>(0);
  
  const [formData, setFormData] = useState<FormData>({
    apiKey: '',
    apiBaseURL: 'https://api.openai.com/v1',
    braveApiKey: '',
    chatModel: 'gpt-4o-mini',
    chatTemperature: 0.7,
    chatMaxTokens: 4096,
    chatTopP: 1.0,
    embeddingsModel: 'text-embedding-3-small',
    embeddingsDimensions: 0,
    useTools: true,
    showInternalMessages: false,
    imageModel: 'dall-e-3',
  });

  // Carrega configuração do backend
  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      
      // Carrega configuração e perfis padrão em paralelo
      const [config, defaultVoiceProfile, defaultInteractionProfile] = await Promise.all([
        GetConfig(),
        GetDefaultVoiceProfile().catch(() => null),
        GetDefaultInteractionProfile().catch(() => null),
      ]);
      
      if (config) {
        setFormData({
          apiKey: config.api_key || '',
          apiBaseURL: config.api_base_url || 'https://api.openai.com/v1',
          braveApiKey: config.brave_api_key || '',
          chatModel: config.chat_params?.model || 'gpt-4o-mini',
          chatTemperature: config.chat_params?.temperature || 0.7,
          chatMaxTokens: config.chat_params?.max_tokens || 4096,
          chatTopP: config.chat_params?.top_p || 1.0,
          embeddingsModel: config.embeddings_params?.model || 'text-embedding-3-small',
          embeddingsDimensions: config.embeddings_params?.dimensions || 0,
          useTools: config.chat_defaults?.use_tools ?? true,
          showInternalMessages: config.chat_defaults?.show_internal_messages ?? false,
          imageModel: config.image_model || 'dall-e-3',
        });
      }
      
      // Define perfis padrão
      if (defaultVoiceProfile) {
        setDefaultVoiceProfileId(defaultVoiceProfile.id);
      }
      if (defaultInteractionProfile) {
        setDefaultInteractionProfileId(defaultInteractionProfile.id);
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
        brave_api_key: formData.braveApiKey,
        chat_params: {
          model: formData.chatModel,
          temperature: formData.chatTemperature,
          max_tokens: formData.chatMaxTokens,
          top_p: formData.chatTopP,
        },
        embeddings_params: {
          model: formData.embeddingsModel,
          dimensions: formData.embeddingsDimensions,
        },
        chat_defaults: {
          use_tools: formData.useTools,
          show_internal_messages: formData.showInternalMessages,
        },
        image_model: formData.imageModel,
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
    if (!confirm('⚠️ ATENÇÃO: Tem certeza que deseja apagar o banco de dados?\n\nIsso irá REMOVER PERMANENTEMENTE:\n- Todas as conversas\n- Todas as memórias\n- Todos os FAQs\n- Todos os perfis de voz\n- Todos os perfis de interação\n- Todas as conexões OAuth\n\nEsta ação NÃO pode ser desfeita!')) {
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
          </div>
        </section>

        {/* Chat Model Section */}
        <section className="settings-section">
          <h2>💬 Modelo de Chat</h2>
          <div className="settings-fields">
            {availableModels.length > 0 ? (
              <Select
                label="Modelo Padrão"
                value={formData.chatModel}
                onChange={(e) => handleChange('chatModel', e.target.value)}
                options={availableModels.map(m => ({ value: m, label: m }))}
                fullWidth
              />
            ) : (
              <Input
                label="Modelo Padrão"
                value={formData.chatModel}
                onChange={(e) => handleChange('chatModel', e.target.value)}
                placeholder="gpt-4o-mini"
                fullWidth
              />
            )}

            <div className="settings-row">
              <Input
                label="Temperature"
                type="number"
                min="0"
                max="2"
                step="0.1"
                value={formData.chatTemperature}
                onChange={(e) => handleChange('chatTemperature', parseFloat(e.target.value) || 0)}
              />

              <Input
                label="Max Tokens"
                type="number"
                min="1"
                max="128000"
                step="100"
                value={formData.chatMaxTokens}
                onChange={(e) => handleChange('chatMaxTokens', parseInt(e.target.value) || 4096)}
              />
            </div>

            <Input
              label="Top P"
              type="number"
              min="0"
              max="1"
              step="0.05"
              value={formData.chatTopP}
              onChange={(e) => handleChange('chatTopP', parseFloat(e.target.value) || 1)}
            />
          </div>
        </section>

        {/* Embeddings Section */}
        <section className="settings-section">
          <h2>🧠 Embeddings</h2>
          <div className="settings-fields">
            <Select
              label="Modelo de Embeddings"
              value={formData.embeddingsModel}
              onChange={(e) => handleChange('embeddingsModel', e.target.value)}
              options={[
                { value: 'text-embedding-3-small', label: 'text-embedding-3-small (recomendado)' },
                { value: 'text-embedding-3-large', label: 'text-embedding-3-large' },
                { value: 'text-embedding-ada-002', label: 'text-embedding-ada-002 (legado)' },
              ]}
              fullWidth
            />

            <Input
              label="Dimensões (0 = padrão do modelo)"
              type="number"
              min="0"
              max="3072"
              value={formData.embeddingsDimensions}
              onChange={(e) => handleChange('embeddingsDimensions', parseInt(e.target.value) || 0)}
            />
          </div>
        </section>

        {/* Image Model Section */}
        <section className="settings-section">
          <h2>🎨 Geração de Imagens</h2>
          <div className="settings-fields">
            <Select
              label="Modelo de Imagens"
              value={formData.imageModel}
              onChange={(e) => handleChange('imageModel', e.target.value)}
              options={[
                { value: 'dall-e-3', label: 'DALL-E 3 (recomendado)' },
                { value: 'dall-e-2', label: 'DALL-E 2' },
                { value: 'gpt-image-1', label: 'GPT Image 1' },
              ]}
              fullWidth
            />
          </div>
        </section>

        {/* Web Search Section */}
        <section className="settings-section">
          <h2>🔍 Busca na Web</h2>
          <p className="settings-section-description">
            Configure a API do Brave Search para buscas na web. 
            <a href="https://api-dashboard.search.brave.com/" target="_blank" rel="noopener noreferrer" style={{ marginLeft: '4px' }}>
              Obter API Key (gratuito)
            </a>
          </p>
          <div className="settings-fields">
            <Input
              label="Brave Search API Key"
              type="password"
              value={formData.braveApiKey}
              onChange={(e) => handleChange('braveApiKey', e.target.value)}
              placeholder="BSA-..."
              fullWidth
            />
            <p className="settings-field-hint">
              {formData.braveApiKey 
                ? '✅ Configurada - Buscas usarão Brave Search API' 
                : '⚠️ Não configurada - Buscas usarão DuckDuckGo (menos confiável)'}
            </p>
          </div>
        </section>

        {/* Default Profiles Section */}
        <section className="settings-section">
          <h2>🎭 Perfis Padrão</h2>
          <p className="settings-section-description">
            Defina os perfis que serão usados por padrão em novas conversas.
          </p>
          <div className="settings-fields settings-profiles-grid">
            <div className="settings-profile-item">
              <label className="settings-label">Perfil de Voz</label>
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

        {/* Chat Defaults Section */}
        <section className="settings-section">
          <h2>⚙️ Padrões do Chat</h2>
          <div className="settings-fields">
            <Checkbox
              label="Usar agentes e ferramentas por padrão"
              checked={formData.useTools}
              onChange={(e) => handleChange('useTools', e.target.checked)}
            />

            <Checkbox
              label="Mostrar mensagens internas (tool calls) por padrão"
              checked={formData.showInternalMessages}
              onChange={(e) => handleChange('showInternalMessages', e.target.checked)}
            />
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
                <p>Remove permanentemente todas as conversas, memórias, FAQs, perfis e conexões.</p>
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
