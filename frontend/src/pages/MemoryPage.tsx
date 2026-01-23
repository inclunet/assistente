import { useState, useEffect } from 'react';
import { 
  GetAllMemories, 
  CreateMemory, 
  UpdateMemory, 
  DeleteMemory 
} from '../../wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import './MemoryPage.css';

interface Memory {
  id: number;
  title: string;
  content: string;
  category: string;
  created_at: string;
}

const CATEGORIES = ['core', 'user', 'preference', 'project', 'context', 'general'];

export default function MemoryPage() {
  const { t } = useTranslation();
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterCategory, setFilterCategory] = useState('all');
  const [showModal, setShowModal] = useState(false);
  const [editingMemory, setEditingMemory] = useState<Memory | null>(null);

  useEffect(() => {
    loadMemories();
  }, []);

  const loadMemories = async () => {
    setLoading(true);
    try {
      const result = await GetAllMemories();
      const mapped = result.map((m: any) => ({
        id: m.id,
        title: m.title,
        content: m.content,
        category: m.category || 'general',
        created_at: m.created_at
      }));
      setMemories(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar memórias:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateOrUpdate = async (memory: Memory) => {
    try {
      if (memory.id) {
        await UpdateMemory(memory.id, memory.content, memory.category, '');
      } else {
        await CreateMemory(memory.content, memory.category, '');
      }
      await loadMemories();
      setShowModal(false);
      setEditingMemory(null);
    } catch (error) {
      console.error('Erro ao salvar memória:', error);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm(t('memory.confirmDelete'))) return;
    
    try {
      await DeleteMemory(id);
      setMemories(prev => prev.filter(m => m.id !== id));
    } catch (error) {
      console.error('Erro ao deletar memória:', error);
    }
  };

  const filteredMemories = memories
    .filter(mem => filterCategory === 'all' || mem.category === filterCategory)
    .filter(mem =>
      mem.title.toLowerCase().includes(searchTerm.toLowerCase()) ||
      mem.content.toLowerCase().includes(searchTerm.toLowerCase())
    );

  return (
    <div className="memory-page">
      <header className="memory-header">
        <h1>{t('memory.title', 'Gerenciar Memórias')}</h1>
        <div className="header-actions">
          <input
            type="text"
            placeholder={t('memory.search', 'Buscar memórias...')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="search-input"
          />
          <select value={filterCategory} onChange={(e) => setFilterCategory(e.target.value)}>
            <option value="all">Todas categorias</option>
            {CATEGORIES.map(cat => (
              <option key={cat} value={cat}>{cat}</option>
            ))}
          </select>
          <button onClick={() => { setEditingMemory(null); setShowModal(true); }} className="btn-primary">
            {t('memory.new', 'Nova Memória')}
          </button>
        </div>
      </header>

      {loading ? (
        <div className="loading">Carregando memórias...</div>
      ) : (
        <div className="memory-grid">
          {filteredMemories.map(memory => (
            <div key={memory.id} className="memory-card">
              <div className="memory-content">
                <div className="memory-header-card">
                  <h3>{memory.title}</h3>
                  <span className={`category-badge ${memory.category}`}>{memory.category}</span>
                </div>
                <p>{memory.content.substring(0, 200)}{memory.content.length > 200 ? '...' : ''}</p>
                <small>{new Date(memory.created_at).toLocaleDateString()}</small>
              </div>
              <div className="memory-actions">
                <button onClick={() => { setEditingMemory(memory); setShowModal(true); }}>✏️</button>
                <button onClick={() => handleDelete(memory.id)} className="delete-btn">🗑️</button>
              </div>
            </div>
          ))}
          {filteredMemories.length === 0 && (
            <div className="empty-state">
              <p>{t('memory.empty', 'Nenhuma memória encontrada')}</p>
            </div>
          )}
        </div>
      )}

      {showModal && (
        <MemoryModal
          memory={editingMemory}
          onSave={handleCreateOrUpdate}
          onClose={() => { setShowModal(false); setEditingMemory(null); }}
        />
      )}
    </div>
  );
}

interface MemoryModalProps {
  memory: Memory | null;
  onSave: (memory: Memory) => void;
  onClose: () => void;
}

function MemoryModal({ memory, onSave, onClose }: MemoryModalProps) {
  const [title, setTitle] = useState(memory?.title || '');
  const [content, setContent] = useState(memory?.content || '');
  const [category, setCategory] = useState(memory?.category || 'general');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave({
      id: memory?.id || 0,
      title,
      content,
      category,
      created_at: memory?.created_at || new Date().toISOString(),
    });
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={e => e.stopPropagation()}>
        <h2>{memory ? 'Editar Memória' : 'Nova Memória'}</h2>
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label>Título*</label>
            <input
              type="text"
              value={title}
              onChange={e => setTitle(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Conteúdo*</label>
            <textarea
              value={content}
              onChange={e => setContent(e.target.value)}
              rows={8}
              required
            />
          </div>
          <div className="form-group">
            <label>Categoria*</label>
            <select value={category} onChange={e => setCategory(e.target.value)} required>
              {CATEGORIES.map(cat => (
                <option key={cat} value={cat}>{cat}</option>
              ))}
            </select>
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
