import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { GetConfig, SaveSettings, ResetConfig, ResetDatabase, TestConnectionWithModels, GetAllChannelConfigs, GetChannelConfig, SaveChannelConfig, RestartChannel } from '@wailsjs/go/main/App';
import { llm } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Input, Button } from '../components';
import { ProfilePicker, ProfilePickerRef } from '../components/pickers/ProfilePicker';
import CreateChannelModal from '../components/modals/CreateChannelModal';
import { useAnnouncer } from '../hooks/useAnnouncer';
import './SettingsPage.css';

interface FormData {
  apiKey: string;
  apiBaseURL: string;
  responseTimeout: number;
}

export default function SettingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  const { handleDatabaseReset } = useChatStore();
  const { announce } = useAnnouncer();
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  const [channels, setChannels] = useState<Record<string, any>>({});
  const [showCreateChannelModal, setShowCreateChannelModal] = useState(false);
  
  const profilePickerRef = useRef<ProfilePickerRef>(null);
  
  const [formData, setFormData] = useState<FormData>({
    apiKey: '',
    apiBaseURL: 'https://api.openai.com/v1',
    responseTimeout: 180,
  });

  useEffect(() => {
    loadConfig();
    loadChannels();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const config = await GetConfig();
      
      if (config) {
        setFormData({
          apiKey: config.api_key || '',
          apiBaseURL: config.api_base_url || 'https://api.openai.com/v1',
          responseTimeout: config.response_timeout || 180,
        });
      }
    } catch (error) {
      console.error('Erro ao carregar configuração:', error);
      addToast('Erro ao carregar configuração', 'error');
    } finally {
      setLoading(false);
    }
  };

  const loadChannels = async () => {
    try {
      const result = await GetAllChannelConfigs();
      setChannels(result || {});
    } catch (error) {
      console.error('Erro ao carregar canais:', error);
    }
  };

  const handleChannelCreated = () => {
    addToast('Canal criado com sucesso!', 'success');
    announce('Canal criado');
    loadChannels();
  };

  const handleToggleChannel = async (channelName: string, currentEnabled: boolean) => {
    try {
      const channelConfig = await GetChannelConfig(channelName);
      if (!channelConfig) {
        throw new Error('Configuração do canal não encontrada');
      }

      channelConfig.enabled = !currentEnabled;
      await SaveChannelConfig(channelName, channelConfig);
      await RestartChannel(channelName);
      await loadChannels();
      
      addToast(`Canal ${channelName} ${!currentEnabled ? 'ativado' : 'desativado'}`, 'success');
    } catch (error: any) {
      console.error('Erro ao alterar status do canal:', error);
      addToast(error.message || 'Erro ao alterar canal', 'error');
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
      await loadConfig();
    } catch (error: any) {
      console.error('Erro ao resetar configurações:', error);
      addToast(error.message || 'Erro ao resetar configurações', 'error');
    }
  };

  const handleResetDatabase = async () => {
    if (!confirm('ATENÇÃO: Tem certeza que deseja apagar o banco de dados?\n\nIsso irá REMOVER PERMANENTEMENTE todas as conversas.\n\nEsta ação NÃO pode ser desfeita!')) {
      return;
    }
    
    if (!confirm('Esta é sua ÚLTIMA CHANCE!\n\nConfirmar exclusão de todos os dados?')) {
      return;
    }
    
    try {
      await ResetDatabase();
      handleDatabaseReset();
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
          <h2>API</h2>
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
                {availableModels.length} modelos disponíveis na API
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

        {/* Profile Section */}
        <section className="settings-section">
          <h2>Perfil Ativo</h2>
          <p className="settings-section-description">
            O perfil ativo define modelo, temperatura, voz e interação para todas as conversas.
          </p>
          <div className="settings-fields">
            <ProfilePicker
              ref={profilePickerRef}
              label="Perfil Ativo"
              maxWidth="100%"
              onAnnounce={announce}
            />
            <Button variant="outline" size="sm" onClick={() => navigate('/profiles')}>
              Gerenciar Perfis
            </Button>
          </div>
        </section>

        {/* Channels Section */}
        <section className="settings-section">
          <div className="channels-header">
            <div>
              <h2>Canais de Mensageria</h2>
              <p className="settings-section-description">
                Configure canais externos para receber e enviar mensagens (Telegram, Signal, etc.).
              </p>
            </div>
            <Button 
              onClick={() => setShowCreateChannelModal(true)}
              className="channels-create-button"
            >
              + Criar Novo Canal
            </Button>
          </div>
          
          <div className="settings-fields">
            {Object.keys(channels).length === 0 ? (
              <div className="channels-empty">
                <p>Nenhum canal configurado ainda.</p>
                <p className="channels-empty-hint">
                  Clique em "Criar Novo Canal" para configurar Telegram, Signal ou outros mensageiros.
                </p>
              </div>
            ) : (
              <div className="channels-list">
                {Object.entries(channels).map(([name, config]: [string, any]) => (
                  <div key={name} className="channel-item">
                    <div className="channel-info">
                      <span className="channel-name">{name}</span>
                      <span className={`channel-status ${config.enabled ? 'enabled' : 'disabled'}`}>
                        {config.enabled ? '● Ativo' : '○ Inativo'}
                      </span>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleToggleChannel(name, config.enabled)}
                    >
                      {config.enabled ? 'Desativar' : 'Ativar'}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </section>

        {/* Danger Zone */}
        <section className="settings-section settings-danger">
          <h2>Zona de Perigo</h2>
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
                <p>Remove permanentemente todas as conversas.</p>
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

      <CreateChannelModal
        isOpen={showCreateChannelModal}
        onClose={() => setShowCreateChannelModal(false)}
        onSuccess={handleChannelCreated}
      />
    </div>
  );
}
