import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { GetConversations, DeleteConversation } from '../../wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import './HistoryPage.css';

interface Conversation {
  id: number;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');

  useEffect(() => {
    loadConversations();
  }, []);

  const loadConversations = async () => {
    setLoading(true);
    try {
      const result = await GetConversations();
      const mapped = result.map((c: any) => ({
        id: c.id,
        title: c.title,
        created_at: c.created_at,
        updated_at: c.updated_at,
        message_count: c.message_count || 0
      }));
      setConversations(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar conversas:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleOpenConversation = (conversationId: number) => {
    navigate(`/?conversation=${conversationId}`);
  };

  const handleDeleteConversation = async (conversationId: number) => {
    if (!confirm(t('history.confirmDelete'))) return;
    
    try {
      await DeleteConversation(conversationId);
      setConversations(prev => prev.filter(c => c.id !== conversationId));
    } catch (error) {
      console.error('Erro ao deletar conversa:', error);
    }
  };

  const filteredConversations = conversations.filter(conv =>
    conv.title.toLowerCase().includes(searchTerm.toLowerCase())
  );

  if (loading) {
    return (
      <div className="history-page">
        <div className="loading">Carregando histórico...</div>
      </div>
    );
  }

  return (
    <div className="history-page">
      <header className="history-header">
        <h1>{t('history.title', 'Histórico de Conversas')}</h1>
        <div className="search-box">
          <input
            type="text"
            placeholder={t('history.search', 'Buscar conversas...')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
        </div>
      </header>

      <div className="conversations-grid">
        {filteredConversations.length === 0 ? (
          <div className="empty-state">
            <p>{t('history.empty', 'Nenhuma conversa encontrada')}</p>
          </div>
        ) : (
          filteredConversations.map((conv) => (
            <div key={conv.id} className="conversation-card">
              <div className="card-content" onClick={() => handleOpenConversation(conv.id)}>
                <h3>{conv.title || t('history.untitled', 'Sem título')}</h3>
                <p className="meta">
                  {t('history.messages', { count: conv.message_count })} • 
                  {new Date(conv.updated_at).toLocaleDateString()}
                </p>
              </div>
              <div className="card-actions">
                <button
                  onClick={() => handleOpenConversation(conv.id)}
                  title={t('history.open', 'Abrir conversa')}
                >
                  📂
                </button>
                <button
                  onClick={() => handleDeleteConversation(conv.id)}
                  title={t('history.delete', 'Deletar conversa')}
                  className="delete-btn"
                >
                  🗑️
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
