import { useState, useEffect, useRef, useImperativeHandle, forwardRef } from 'react';
import { GetModels } from '../../../wailsjs/go/main/App';
import { Combobox, ComboboxItem } from './Combobox';
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

    // Expõe método reload
    useImperativeHandle(ref, () => ({
        reload: loadModels
    }));

    // Items formatados para o Combobox
    const items: ComboboxItem[] = models.map(m => ({ value: m, label: m }));

    const handleSelect = (selectedValue: string) => {
        onChange(selectedValue);
    };

    if (variant === 'form') {
        return (
            <div className="model-picker-form">
                <label htmlFor={`model-picker-${label}`}>{label}</label>

                {loading ? (
                    <div className="loading-state" role="status" aria-live="polite">
                        <span className="loading-spinner" aria-hidden="true"></span>
                        Carregando modelos...
                    </div>
                ) : error ? (
                    <div className="error-state" role="alert">
                        <span>{error}</span>
                        <button type="button" className="retry-btn" onClick={loadModels}>
                            🔄 Tentar novamente
                        </button>
                    </div>
                ) : (
                    <Combobox
                        icon={icon}
                        label={label}
                        items={items}
                        selected={value}
                        onSelect={handleSelect}
                        placeholder={placeholder}
                        disabled={disabled}
                        maxWidth="100%"
                        onAnnounce={onAnnounce}
                    />
                )}

                {helpText && <p className="help-text">{helpText}</p>}
            </div>
        );
    }

    // Variant toolbar (compacto)
    if (loading) {
        return (
            <div className="model-picker-toolbar loading" role="status" aria-live="polite">
                <span className="loading-spinner" aria-hidden="true"></span>
                <span>Carregando...</span>
            </div>
        );
    }

    if (error) {
        return (
            <div className="model-picker-toolbar error" role="alert">
                <span>❌ Erro</span>
            </div>
        );
    }

    return (
        <Combobox
            icon={icon}
            label={label}
            items={items}
            selected={value}
            onSelect={handleSelect}
            placeholder={placeholder}
            disabled={disabled}
            maxWidth={maxWidth}
            onAnnounce={onAnnounce}
        />
    );
});

ModelPicker.displayName = 'ModelPicker';
