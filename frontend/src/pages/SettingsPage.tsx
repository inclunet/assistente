import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useSettingsStore } from '../store/settingsStore';
import { useUIStore } from '../store/uiStore';
import { Input, Select, Checkbox, Button } from '../components';
import './SettingsPage.css';

export default function SettingsPage() {
  const { t } = useTranslation();
  const { config, setConfig } = useSettingsStore();
  const { addToast } = useUIStore();
  const [loading, setLoading] = useState(false);

  // Form state
  const [formData, setFormData] = useState({
    apiKey: config?.apiKey || '',
    baseURL: config?.baseURL || 'https://api.openai.com/v1',
    defaultModel: config?.defaultModel || 'gpt-4',
    temperature: config?.temperature || 0.7,
    maxTokens: config?.maxTokens || 2000,
    streamEnabled: config?.streamEnabled ?? true,
    language: config?.language || 'pt-BR',
  });

  // Update form when config changes
  useEffect(() => {
    if (config) {
      setFormData({
        apiKey: config.apiKey,
        baseURL: config.baseURL,
        defaultModel: config.defaultModel,
        temperature: config.temperature,
        maxTokens: config.maxTokens,
        streamEnabled: config.streamEnabled,
        language: config.language,
      });
    }
  }, [config]);

  const handleChange = (field: string, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      // Atualizar store local
      setConfig({
        ...formData,
        theme: config?.theme || 'system',
      });

      // TODO: Integrar com backend quando SaveConfig estiver disponível
      addToast('Configurações salvas com sucesso!', 'success');
    } catch (error: any) {
      console.error('Erro ao salvar configurações:', error);
      addToast(error.message || 'Erro ao salvar configurações', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleReset = () => {
    if (config) {
      setFormData({
        apiKey: config.apiKey,
        baseURL: config.baseURL,
        defaultModel: config.defaultModel,
        temperature: config.temperature,
        maxTokens: config.maxTokens,
        streamEnabled: config.streamEnabled,
        language: config.language,
      });
    }
    addToast('Formulário resetado', 'info');
  };

  if (!config) {
    return (
      <div className="settings-page">
        <div className="settings-loading">Carregando configurações...</div>
      </div>
    );
  }

  return (
    <div className="settings-page">
      <div className="settings-header">
        <h1>Configurações</h1>
        <p>Configure as opções do assistente</p>
      </div>

      <div className="settings-content">
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
              value={formData.baseURL}
              onChange={(e) => handleChange('baseURL', e.target.value)}
              placeholder="https://api.openai.com/v1"
              fullWidth
            />
          </div>
        </section>

        <section className="settings-section">
          <h2>Modelo</h2>
          <div className="settings-fields">
            <Select
              label="Modelo Padrão"
              value={formData.defaultModel}
              onChange={(e) => handleChange('defaultModel', e.target.value)}
              options={[
                { value: 'gpt-4', label: 'GPT-4' },
                { value: 'gpt-4-turbo', label: 'GPT-4 Turbo' },
                { value: 'gpt-3.5-turbo', label: 'GPT-3.5 Turbo' },
                { value: 'claude-3-opus', label: 'Claude 3 Opus' },
                { value: 'claude-3-sonnet', label: 'Claude 3 Sonnet' },
              ]}
              fullWidth
            />

            <div className="settings-row">
              <Input
                label="Temperatura"
                type="number"
                min="0"
                max="2"
                step="0.1"
                value={formData.temperature}
                onChange={(e) => handleChange('temperature', parseFloat(e.target.value))}
              />

              <Input
                label="Max Tokens"
                type="number"
                min="1"
                max="32000"
                step="100"
                value={formData.maxTokens}
                onChange={(e) => handleChange('maxTokens', parseInt(e.target.value))}
              />
            </div>

            <Checkbox
              label="Habilitar streaming de respostas"
              checked={formData.streamEnabled}
              onChange={(e) => handleChange('streamEnabled', e.target.checked)}
            />
          </div>
        </section>

        <section className="settings-section">
          <h2>Interface</h2>
          <div className="settings-fields">
            <Select
              label="Idioma"
              value={formData.language}
              onChange={(e) => handleChange('language', e.target.value)}
              options={[
                { value: 'pt-BR', label: 'Português (Brasil)' },
                { value: 'en-US', label: 'English (US)' },
              ]}
              fullWidth
            />
          </div>
        </section>
      </div>

      <div className="settings-footer">
        <Button variant="ghost" onClick={handleReset} disabled={loading}>
          Resetar
        </Button>
        <Button onClick={handleSave} loading={loading}>
          Salvar Configurações
        </Button>
      </div>
    </div>
  );
}
