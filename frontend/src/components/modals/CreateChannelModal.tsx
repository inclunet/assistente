import { useState, useEffect } from 'react';
import { GetChannelTemplates, CreateChannelFromTemplate } from '@wailsjs/go/main/App';
import { channels } from '../../../wailsjs/go/models';
import { Button, Input } from '..';
import './CreateChannelModal.css';

interface CreateChannelModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  initialTemplateType?: string | null;
}

export default function CreateChannelModal({ isOpen, onClose, onSuccess, initialTemplateType }: CreateChannelModalProps) {
  const [templates, setTemplates] = useState<channels.ChannelTemplate[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState<channels.ChannelTemplate | null>(null);
  const [formValues, setFormValues] = useState<Record<string, any>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>('');

  useEffect(() => {
    if (isOpen) {
      loadTemplates();
    }
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    if (!initialTemplateType) return;
    if (selectedTemplate) return;
    if (!templates.length) return;

    const match = templates.find((template) => template.type === initialTemplateType);
    if (match) {
      handleTemplateSelect(match);
    }
  }, [isOpen, initialTemplateType, templates, selectedTemplate]);

  const loadTemplates = async () => {
    try {
      const result = await GetChannelTemplates();
      setTemplates(result || []);
    } catch (err) {
      console.error('Erro ao carregar templates:', err);
      setError('Erro ao carregar templates de canais');
    }
  };

  const handleTemplateSelect = (template: channels.ChannelTemplate) => {
    setSelectedTemplate(template);
    // Inicializa valores padrão
    const defaults: Record<string, any> = {};
    template.fields?.forEach((field) => {
      if (field.default_value !== undefined && field.default_value !== null) {
        defaults[field.key] = field.default_value;
      }
    });
    setFormValues(defaults);
    setError('');
  };

  const handleFieldChange = (key: string, value: any) => {
    setFormValues((prev) => ({ ...prev, [key]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedTemplate) return;

    setLoading(true);
    setError('');

    try {
      await CreateChannelFromTemplate(selectedTemplate.type, formValues);
      onSuccess();
      handleClose();
    } catch (err: any) {
      console.error('Erro ao criar canal:', err);
      setError(err.message || 'Erro ao criar canal');
    } finally {
      setLoading(false);
    }
  };

  const handleClose = () => {
    setSelectedTemplate(null);
    setFormValues({});
    setError('');
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div className="modal-overlay" onClick={handleClose}>
      <div className="modal-content create-channel-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>{selectedTemplate ? `Configurar ${selectedTemplate.display_name}` : 'Criar Novo Canal'}</h2>
          <button className="modal-close" onClick={handleClose} aria-label="Fechar">
            ×
          </button>
        </div>

        <div className="modal-body">
          {error && (
            <div className="error-message" role="alert">
              {error}
            </div>
          )}

          {!selectedTemplate ? (
            <div className="template-selection">
              <p className="template-selection-description">
                Selecione o tipo de canal que deseja configurar:
              </p>
              <div className="template-grid">
                {templates.map((template) => (
                  <button
                    key={template.type}
                    className="template-card"
                    onClick={() => handleTemplateSelect(template)}
                  >
                    <div className="template-icon">{template.icon}</div>
                    <div className="template-info">
                      <h3>{template.display_name}</h3>
                      <p>{template.description}</p>
                    </div>
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="channel-form">
              <div className="channel-form-header">
                <div className="channel-form-icon">{selectedTemplate.icon}</div>
                <div>
                  <p className="channel-form-description">{selectedTemplate.description}</p>
                  {selectedTemplate.doc_url && (
                    <a
                      href={selectedTemplate.doc_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="channel-form-docs"
                    >
                      📚 Ver documentação
                    </a>
                  )}
                </div>
              </div>

              <div className="channel-form-fields">
                {selectedTemplate.fields?.map((field) => (
                  <div key={field.key} className="form-field">
                    <Input
                      label={field.label}
                      type={field.type === 'password' ? 'password' : field.type === 'number' ? 'number' : 'text'}
                      value={formValues[field.key] || ''}
                      onChange={(e) => handleFieldChange(field.key, e.target.value)}
                      placeholder={field.placeholder}
                      required={field.required}
                      fullWidth
                    />
                    {field.description && (
                      <p className="field-description">{field.description}</p>
                    )}
                  </div>
                ))}
              </div>

              <div className="modal-actions">
                <Button variant="ghost" onClick={() => setSelectedTemplate(null)} disabled={loading}>
                  Voltar
                </Button>
                <Button type="submit" loading={loading}>
                  Criar Canal
                </Button>
              </div>
            </form>
          )}
        </div>
      </div>
    </div>
  );
}
