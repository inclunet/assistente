import { useState, useEffect, useImperativeHandle, forwardRef } from 'react';
import { GetModels } from '@wailsjs/go/main/App';
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
  onAnnounce
}, ref) => {
  const [models, setModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadModels = async () => {
    setLoading(true);
    setError('');
    try {
      const modelsList = await GetModels() || [];
      setModels(modelsList);
    } catch (e) {
      setError('Erro ao carregar modelos');
      console.error('ModelPicker: erro ao carregar modelos', e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, []);

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
