import { useState, useEffect } from 'react';
import { 
  GetAllFAQs, 
  CreateFAQ, 
  UpdateFAQ, 
  DeleteFAQ,
  RegenerateFAQEmbeddings,
  GetFAQEmbeddingStatus 
} from '../../wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Button } from '../components/ui/Button';
import './FAQPage.css';

interface FAQ {
  id: number;
  question: string;
  answer: string;
  tags?: string;
  category?: string;
  has_embedding?: boolean;
}

export default function FAQPage() {
  const { t } = useTranslation();
  const [faqs, setFaqs] = useState<FAQ[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [showModal, setShowModal] = useState(false);
  const [editingFAQ, setEditingFAQ] = useState<FAQ | null>(null);
  const [embeddingStatus, setEmbeddingStatus] = useState<any>(null);
  const [saving, setSaving] = useState(false);
  
  // Form state
  const [formQuestion, setFormQuestion] = useState('');
  const [formAnswer, setFormAnswer] = useState('');
  const [formTags, setFormTags] = useState('');
  const [formError, setFormError] = useState('');

  useEffect(() => {
    loadFAQs();
    loadEmbeddingStatus();
  }, []);

  // Atalho Ctrl+N para nova FAQ
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.key.toLowerCase() === 'n') {
        event.preventDefault();
        openNewForm();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadFAQs = async () => {
    setLoading(true);
    try {
      const result = await GetAllFAQs();
      const mapped = result.map((f: any) => ({
        id: f.id,
        question: f.question,
        answer: f.answer,
        tags: typeof f.tags === 'string' ? f.tags : (Array.isArray(f.tags) ? f.tags.join(',') : ''),
        category: f.category,
        has_embedding: f.has_embedding
      }));
      setFaqs(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar FAQs:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadEmbeddingStatus = async () => {
    try {
      const status = await GetFAQEmbeddingStatus();
      setEmbeddingStatus(status);
    } catch (error) {
      console.error('Erro ao carregar status de embeddings:', error);
    }
  };

  const openNewForm = () => {
    setEditingFAQ(null);
    setFormQuestion('');
    setFormAnswer('');
    setFormTags('');
    setFormError('');
    setShowModal(true);
  };

  const openEditForm = (faq: FAQ) => {
    setEditingFAQ(faq);
    setFormQuestion(faq.question);
    setFormAnswer(faq.answer);
    setFormTags(faq.tags || '');
    setFormError('');
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!formQuestion.trim() || !formAnswer.trim()) {
      setFormError('Pergunta e resposta são obrigatórias');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      if (editingFAQ) {
        await UpdateFAQ(editingFAQ.id, formQuestion, formAnswer, formTags);
      } else {
        await CreateFAQ(formQuestion, formAnswer, formTags);
      }
      await loadFAQs();
      setShowModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (faq: FAQ) => {
    if (!confirm(t('faq.confirmDelete', 'Tem certeza que deseja excluir esta FAQ?'))) return;
    
    try {
      await DeleteFAQ(faq.id);
      setFaqs(prev => prev.filter(f => f.id !== faq.id));
    } catch (error) {
      console.error('Erro ao deletar FAQ:', error);
      alert('Erro ao deletar FAQ');
    }
  };

  const handleRegenerateEmbeddings = async () => {
    if (!confirm('Deseja regenerar os embeddings de todas as FAQs? Isso pode levar alguns minutos.')) return;

    try {
      await RegenerateFAQEmbeddings();
      await loadEmbeddingStatus();
      alert(t('faq.embeddingsRegenerated', 'Embeddings regenerados com sucesso!'));
    } catch (error) {
      console.error('Erro ao regenerar embeddings:', error);
      alert('Erro ao regenerar embeddings');
    }
  };

  const filteredFAQs = faqs.filter(faq =>
    faq.question.toLowerCase().includes(searchTerm.toLowerCase()) ||
    faq.answer.toLowerCase().includes(searchTerm.toLowerCase()) ||
    (faq.tags || '').toLowerCase().includes(searchTerm.toLowerCase())
  );

  const columns: DataGridColumn<FAQ>[] = [
    { 
      key: 'question', 
      label: 'Pergunta',
      truncate: true,
    },
    { 
      key: 'answer', 
      label: 'Resposta',
      truncate: true,
      format: (value) => value?.length > 80 ? value.substring(0, 80) + '...' : value
    },
    { 
      key: 'tags', 
      label: 'Tags',
      width: '150px',
      format: (value) => value || '—'
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '✏️',
      actionLabel: 'Editar FAQ',
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️',
      actionLabel: 'Excluir FAQ',
    }
  ];

  const toolbarActions: ToolbarAction[] = [
    {
      key: 'new',
      label: 'Nova FAQ',
      icon: '➕',
      onClick: openNewForm,
      variant: 'primary',
      shortcut: 'Ctrl+N',
    },
    {
      key: 'regenerate',
      label: 'Regenerar Embeddings',
      icon: '🔄',
      onClick: handleRegenerateEmbeddings,
      variant: 'secondary',
      disabled: !embeddingStatus || embeddingStatus.total === 0,
    },
  ];

  if (loading) {
    return (
      <div className="faq-page">
        <Toolbar left={<h1 className="page-toolbar__title">Gerenciar FAQs</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando FAQs...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="faq-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Perguntas Frequentes</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar FAQs..."
      />

      <div className="page-content">
        {embeddingStatus && (
          <div className="embedding-status">
            <span>📊 Embeddings: {embeddingStatus.with_embeddings || 0}/{embeddingStatus.total || 0}</span>
          </div>
        )}

        <DataGrid
          items={filteredFAQs}
          columns={columns}
          label="Lista de FAQs"
          getItemId={(faq) => faq.id}
          onActivate={(faq) => openEditForm(faq)}
          onDelete={(faq) => handleDelete(faq)}
          onCellAction={(faq, column) => {
            if (column.key === 'edit') {
              openEditForm(faq);
            } else if (column.key === 'delete') {
              handleDelete(faq);
            }
          }}
        />
      </div>

      {showModal && (
        <SimpleModal
          isOpen={showModal}
          onClose={() => setShowModal(false)}
          title={editingFAQ ? 'Editar FAQ' : 'Nova FAQ'}
        >
          <div className="modal-form">
            {formError && (
              <div className="form-error">{formError}</div>
            )}

            <div className="form-group">
              <label htmlFor="question">Pergunta *</label>
              <Input
                id="question"
                value={formQuestion}
                onChange={(e) => setFormQuestion(e.target.value)}
                placeholder="Digite a pergunta..."
                autoFocus
              />
            </div>

            <div className="form-group">
              <label htmlFor="answer">Resposta *</label>
              <Textarea
                id="answer"
                value={formAnswer}
                onChange={(e) => setFormAnswer(e.target.value)}
                placeholder="Digite a resposta..."
                rows={6}
              />
            </div>

            <div className="form-group">
              <label htmlFor="tags">Tags (separadas por vírgula)</label>
              <Input
                id="tags"
                value={formTags}
                onChange={(e) => setFormTags(e.target.value)}
                placeholder="tag1, tag2, tag3"
              />
            </div>

            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setShowModal(false)}>
                Cancelar
              </Button>
              <Button variant="primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Salvando...' : 'Salvar'}
              </Button>
            </div>
          </div>
        </SimpleModal>
      )}
    </div>
  );
}
