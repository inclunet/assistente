import { logger } from '../../utils/logger';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { GetChannelTemplates, CreateChannelFromTemplate } from '@wailsjs/go/app/App';
import { channels } from '../../../wailsjs/go/models';
import { Button, Input } from '..';
import { Modal } from '../ui/Modal';
import './CreateChannelModal.css';

interface CreateChannelModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  initialTemplateType?: string | null;
}

export default function CreateChannelModal({ isOpen, onClose, onSuccess, initialTemplateType }: CreateChannelModalProps) {
  const { t } = useTranslation();
  const [templates, setTemplates] = useState<channels.ChannelTemplate[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState<channels.ChannelTemplate | null>(null);
  const [formValues, setFormValues] = useState<Record<string, unknown>>({});
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
      logger.error('Erro ao carregar templates:', err);
      setError(t('channels.createModal.loadError'));
    }
  };

  const handleTemplateSelect = (template: channels.ChannelTemplate) => {
    setSelectedTemplate(template);
    // Inicializa valores padrão
    const defaults: Record<string, unknown> = {};
    template.fields?.forEach((field) => {
      if (field.default_value !== undefined && field.default_value !== null) {
        defaults[field.key] = field.default_value;
      }
    });
    setFormValues(defaults);
    setError('');
  };

  const handleFieldChange = (key: string, value: unknown) => {
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
    } catch (err: unknown) {
      logger.error('Erro ao criar canal:', err);
      const errMessage = (err as { message?: unknown } | null)?.message;
      setError(String(errMessage || err || 'Erro ao criar canal'));
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

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={selectedTemplate ? `${t('channels.createModal.configure')} ${selectedTemplate.display_name}` : t('channels.createModal.title')}
      size="md"
    >
      <div className="create-channel-modal">
        {error && (
          <div className="error-message">
            {error}
          </div>
        )}

        {!selectedTemplate ? (
          <div className="template-selection">
            <p className="template-selection-description">
              {t('channels.createModal.selectType')}
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
                    {t('channels.createModal.viewDocs')}
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
                    value={typeof formValues[field.key] === 'string' || typeof formValues[field.key] === 'number'
                      ? String(formValues[field.key])
                      : ''}
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
              <Button
                variant="ghost"
                onClick={() => {
                  setSelectedTemplate(null);
                  setError('');
                }}
                disabled={loading}
              >
                {t('channels.createModal.back')}
              </Button>
              <Button type="submit" loading={loading}>
                {t('channels.createModal.create')}
              </Button>
            </div>
          </form>
        )}
      </div>
    </Modal>
  );
}

