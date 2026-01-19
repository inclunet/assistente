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
import './FAQPage.css';

interface FAQ {
  id: string;
  question: string;
  answer: string;
  tags?: string[];
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

  useEffect(() => {
    loadFAQs();
    loadEmbeddingStatus();
  }, []);

  const loadFAQs = async () => {
    setLoading(true);
    try {
      const result = await GetAllFAQs();
      setFaqs(result || []);
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

  const handleCreateOrUpdate = async (faq: FAQ) => {
    try {
      if (faq.id) {
        await UpdateFAQ(faq);
      } else {
        await CreateFAQ(faq);
      }
      await loadFAQs();
      setShowModal(false);
      setEditingFAQ(null);
    } catch (error) {
      console.error('Erro ao salvar FAQ:', error);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm(t('faq.confirmDelete'))) return;
    
    try {
      await DeleteFAQ(id);
      setFaqs(prev => prev.filter(f => f.id !== id));
    } catch (error) {
      console.error('Erro ao deletar FAQ:', error);
    }
  };

  const handleRegenerateEmbeddings = async () => {
    try {
      await RegenerateFAQEmbeddings();
      await loadEmbeddingStatus();
      alert(t('faq.embeddingsRegenerated'));
    } catch (error) {
      console.error('Erro ao regenerar embeddings:', error);
    }
  };

  const filteredFAQs = faqs.filter(faq =>
    faq.question.toLowerCase().includes(searchTerm.toLowerCase()) ||
    faq.answer.toLowerCase().includes(searchTerm.toLowerCase())
  );

  return (
    <div className="faq-page">
      <header className="faq-header">
        <h1>{t('faq.title', 'Gerenciar FAQs')}</h1>
        <div className="header-actions">
          <input
            type="text"
            placeholder={t('faq.search', 'Buscar FAQs...')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="search-input"
          />
          <button onClick={() => { setEditingFAQ(null); setShowModal(true); }} className="btn-primary">
            {t('faq.new', 'Nova FAQ')}
          </button>
          {embeddingStatus && (
            <button onClick={handleRegenerateEmbeddings} className="btn-secondary">
              🔄 Embeddings ({embeddingStatus.with_embeddings}/{embeddingStatus.total})
            </button>
          )}
        </div>
      </header>

      {loading ? (
        <div className="loading">Carregando FAQs...</div>
      ) : (
        <div className="faq-grid">
          {filteredFAQs.map(faq => (
            <div key={faq.id} className="faq-card">
              <div className="faq-content">
                <h3>{faq.question}</h3>
                <p>{faq.answer.substring(0, 150)}{faq.answer.length > 150 ? '...' : ''}</p>
                {faq.tags && faq.tags.length > 0 && (
                  <div className="tags">
                    {faq.tags.map((tag, i) => (
                      <span key={i} className="tag">{tag}</span>
                    ))}
                  </div>
                )}
              </div>
              <div className="faq-actions">
                <button onClick={() => { setEditingFAQ(faq); setShowModal(true); }}>✏️</button>
                <button onClick={() => handleDelete(faq.id)} className="delete-btn">🗑️</button>
              </div>
            </div>
          ))}
          {filteredFAQs.length === 0 && (
            <div className="empty-state">
              <p>{t('faq.empty', 'Nenhuma FAQ encontrada')}</p>
            </div>
          )}
        </div>
      )}

      {showModal && (
        <FAQModal
          faq={editingFAQ}
          onSave={handleCreateOrUpdate}
          onClose={() => { setShowModal(false); setEditingFAQ(null); }}
        />
      )}
    </div>
  );
}

interface FAQModalProps {
  faq: FAQ | null;
  onSave: (faq: FAQ) => void;
  onClose: () => void;
}

function FAQModal({ faq, onSave, onClose }: FAQModalProps) {
  const [question, setQuestion] = useState(faq?.question || '');
  const [answer, setAnswer] = useState(faq?.answer || '');
  const [tags, setTags] = useState(faq?.tags?.join(', ') || '');
  const [category, setCategory] = useState(faq?.category || '');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: faq?.id || '',
      question,
      answer,
      tags: tags.split(',').map(t => t.trim()).filter(Boolean),
      category: category || undefined,
    });
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={e => e.stopPropagation()}>
        <h2>{faq ? 'Editar FAQ' : 'Nova FAQ'}</h2>
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Pergunta*</label>
            <input
              type="text"
              value={question}
              onChange={e => setQuestion(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Resposta*</label>
            <textarea
              value={answer}
              onChange={e => setAnswer(e.target.value)}
              rows={6}
              required
            />
          </div>
          <div className="form-group">
            <label>Tags (separadas por vírgula)</label>
            <input
              type="text"
              value={tags}
              onChange={e => setTags(e.target.value)}
            />
          </div>
          <div className="form-group">
            <label>Categoria</label>
            <input
              type="text"
              value={category}
              onChange={e => setCategory(e.target.value)}
            />
          </div>
          <div className="modal-actions">
            <button type="button" onClick={onClose} className="btn-secondary">
              Cancelar
            </button>
            <button type="submit" className="btn-primary">
              Salvar
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
