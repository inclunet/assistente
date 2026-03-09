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

  const loadModels = async () => {
    // Se estamos no modo form e não há providerID, não tenta carregar
    if (variant === 'form' && !providerID) {
      setLoading(false);
      setError('Selecione um provedor primeiro');
      setModels([]);
      return;
    }

    setLoading(true);
    setError('');
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
      
      // Detecta erro de credencial não configurada
      if (errorMsg.includes('credencial não configurada') || errorMsg.includes('Missing bearer authentication')) {
        setError('Configure a API key deste provedor em Configurações → Credenciais');
      } else {
        setError(`Erro ao carregar modelos: ${errorMsg}`);
      }
      
      setModels([]);
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
      placeholder={placeholder}
      disabled={disabled}
      maxWidth={variant === 'form' ? '100%' : maxWidth}
      helpText={variant === 'form' ? helpText : undefined}
      onAnnounce={onAnnounce}
      loading={loading}
      error={error || null}
      onRetry={loadModels}
      retryLabel="🔄 Tentar novamente"
      showFormLabel={variant === 'form'}
      showFormLabelIcon={false}
      formClassName="model-picker-form"
      toolbarClassName="model-picker-toolbar"
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
