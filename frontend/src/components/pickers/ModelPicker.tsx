import { useState, useEffect, useImperativeHandle, forwardRef } from 'react';
import { GetModels, GetModelsByProvider } from '@wailsjs/go/main/App';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import './ModelPicker.css';

export interface ModelPickerProps {
    value: string;
    onChange: (value: string) => void;
    label?: string;
    icon?: string;
    placeholder?: string;
    disabled?: boolean;
    maxWidth?: string;
    variant?: 'toolbar' | 'form';
    helpText?: string;
    onAnnounce?: (message: string) => void;
    providerID?: string; // ID do provedor para filtrar modelos
}

export interface ModelPickerRef {
    reload: () => void;
}

export const ModelPicker = forwardRef<ModelPickerRef, ModelPickerProps>(({
  value,
  onChange,
  label = 'Modelo',
  icon = '🤖',
  placeholder = 'Filtrar modelos...',
  disabled = false,
  maxWidth = '180px',
  variant = 'toolbar',
  helpText = '',
  onAnnounce,
  providerID = '', // Provedor específico (se vazio, usa GetModels do ativo)
}, ref) => {
  const [models, setModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [endpointNotSupported, setEndpointNotSupported] = useState(false);

  const loadModels = async () => {
    // Se estamos no modo form e não há providerID, não tenta carregar
    if (variant === 'form' && !providerID) {
      setLoading(false);
      setError('Selecione um provedor primeiro');
      setModels([]);
      setEndpointNotSupported(false);
      return;
    }

    setLoading(true);
    setError('');
    setEndpointNotSupported(false);
    try {
      let modelsList: string[];
      
      // Se providerID foi fornecido, usa GetModelsByProvider
      if (providerID) {
        modelsList = await GetModelsByProvider(providerID);
      } else {
        // Caso contrário, usa GetModels (do provedor ativo)
        modelsList = await GetModels();
      }
      
      setModels(modelsList || []);
      if (!modelsList || modelsList.length === 0) {
        const msg = providerID
          ? 'Nenhum modelo disponível para este provedor.'
          : 'Nenhum modelo disponível. Configure um provedor primeiro.';
        setError(msg);
      }
    } catch (e: any) {
      const errorMsg = e?.message || String(e) || 'Erro desconhecido';
      
      // Detecta se o endpoint de modelos não é suportado (404)
      if (errorMsg.includes('models_endpoint_not_supported')) {
        setEndpointNotSupported(true);
        setError('');
        setModels([]);
      } else if (errorMsg.includes('credencial não configurada') || errorMsg.includes('Missing bearer authentication')) {
        // Detecta erro de credencial não configurada
        setError('Configure a API key deste provedor em Configurações → Credenciais');
        setEndpointNotSupported(false);
        setModels([]);
      } else {
        setError(`Erro ao carregar modelos: ${errorMsg}`);
        setEndpointNotSupported(false);
        setModels([]);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, [providerID]); // Recarrega quando providerID muda

  useImperativeHandle(ref, () => ({
    reload: loadModels
  }));

  const items: ComboboxItem[] = models.map(m => ({ value: m, label: m }));

  const handleSelect = (selectedValue: string) => {
    onChange(selectedValue);
  };

  return (
    <BasePicker
      variant={variant}
      items={items}
      selected={value}
      onSelect={handleSelect}
      label={label}
      icon={icon}
      placeholder={endpointNotSupported ? 'Digite o modelo...' : placeholder}
      disabled={disabled}
      maxWidth={variant === 'form' ? '100%' : maxWidth}
      helpText={variant === 'form' ? (endpointNotSupported ? 'Modelos não carregados. Digite manualmente o nome do modelo.' : helpText) : undefined}
      onAnnounce={onAnnounce}
      loading={loading && !endpointNotSupported}
      error={endpointNotSupported ? null : (error || null)}
      onRetry={endpointNotSupported ? undefined : loadModels}
      retryLabel="🔄 Tentar novamente"
      showFormLabel={variant === 'form'}
      showFormLabelIcon={false}
      showEmptyState={!endpointNotSupported}
      allowFreeInput={endpointNotSupported}
      formClassName={`model-picker-form${endpointNotSupported ? ' models-not-available' : ''}`}
      toolbarClassName={`model-picker-toolbar${endpointNotSupported ? ' models-not-available' : ''}`}
      loadingClassName={{ form: 'loading-state', toolbar: 'model-picker-toolbar loading' }}
      errorClassName={{ form: 'error-state', toolbar: 'model-picker-toolbar error' }}
      helpTextClassName="help-text"
      errorIcon={{ toolbar: '❌', form: undefined }}
      loadingLabel={{ form: 'Carregando modelos...', toolbar: 'Carregando...' }}
      errorLabel={{ form: error || 'Erro ao carregar modelos', toolbar: 'Erro' }}
    />
  );
});

ModelPicker.displayName = 'ModelPicker';
