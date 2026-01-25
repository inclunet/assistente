import { useState, useEffect, useCallback } from 'react';
import { Combobox, ComboboxItem } from './Combobox';
import { GetConversations } from '../../../wailsjs/go/main/App';
import { database } from '../../../wailsjs/go/models';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';

export interface HistoryPickerProps {
  value?: number; // ID da conversa atual
  onChange: (conversationId: number, conversation: database.Conversation) => void;
  label?: string;
  disabled?: boolean;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
}

const NEW_CONVERSATION_VALUE = 'new';

export const HistoryPicker = ({
  value,
  onChange,
  label = 'Histórico (Ctrl+H)',
  disabled = false,
  maxWidth = '200px',
  onAnnounce
}: HistoryPickerProps) => {
  const [conversations, setConversations] = useState<database.Conversation[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  // Carrega conversas do backend
  const loadConversations = useCallback(async () => {
    try {
      setIsLoading(true);
      const result = await GetConversations();
      // Ordena por data de atualização (mais recente primeiro)
      const sorted = result.sort((a, b) => {
        // Converte time.Time para número (timestamp)
        const dateA = new Date((a.updated_at as any)).getTime();
        const dateB = new Date((b.updated_at as any)).getTime();
        return dateB - dateA;
      });
      setConversations(sorted);
      console.log('[HistoryPicker] Conversas carregadas:', sorted.length);
    } catch (error) {
      console.error('[HistoryPicker] Erro ao carregar conversas:', error);
    } finally {
      setIsLoading(false);
    }
  }, []);

  // Carrega conversas na montagem do componente
  useEffect(() => {
    loadConversations();
  }, [loadConversations]);

  // Escuta eventos de novas conversas criadas
  useEffect(() => {
    const handleConversationCreated = () => {
      console.log('[HistoryPicker] Nova conversa criada, recarregando lista...');
      loadConversations();
    };

    const handleTabTitleUpdated = () => {
      console.log('[HistoryPicker] Título de aba atualizado, recarregando lista...');
      loadConversations();
    };

    const handleConversationDeleted = () => {
      console.log('[HistoryPicker] Conversa deletada, recarregando lista...');
      loadConversations();
    };

    EventsOn('chat:conversation_created', handleConversationCreated);
    EventsOn('tab_title_updated', handleTabTitleUpdated);
    EventsOn('conversation:deleted', handleConversationDeleted);

    return () => {
      EventsOff('chat:conversation_created');
      EventsOff('tab_title_updated');
      EventsOff('conversation:deleted');
    };
  }, [loadConversations]);

  // Callback quando o picker é aberto - recarrega conversas
  const handleOpen = useCallback(() => {
    console.log('[HistoryPicker] Picker aberto, recarregando conversas...');
    loadConversations();
  }, [loadConversations]);

  // Formata data para exibição
  const formatDate = (dateValue: any): string => {
    const date = new Date(dateValue);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return 'agora';
    if (diffMins < 60) return `${diffMins}min atrás`;
    if (diffHours < 24) return `${diffHours}h atrás`;
    if (diffDays < 7) return `${diffDays}d atrás`;
    
    return date.toLocaleDateString('pt-BR', { 
      day: '2-digit', 
      month: '2-digit',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined
    });
  };

  // Converte conversas para items do Combobox
  const items: ComboboxItem[] = [
    {
      value: NEW_CONVERSATION_VALUE,
      label: '➕ Nova conversa',
      sublabel: 'Criar conversa em branco'
    },
    ...conversations.map(conv => ({
      value: conv.id.toString(),
      label: conv.title || 'Sem título',
      sublabel: `${conv.message_count || 0} msgs • ${formatDate(conv.updated_at)}`
    }))
  ];

  // Valor selecionado
  const selectedValue = value ? value.toString() : NEW_CONVERSATION_VALUE;

  const handleSelect = (selectedValue: string, _item: ComboboxItem) => {
    if (selectedValue === NEW_CONVERSATION_VALUE) {
      // Nova conversa - chama onChange com ID 0 (convenção para nova conversa)
      onChange(0, {} as database.Conversation);
    } else {
      const conversationId = parseInt(selectedValue, 10);
      const conversation = conversations.find(c => c.id === conversationId);
      if (conversation) {
        onChange(conversationId, conversation);
      }
    }
  };

  return (
    <Combobox
      icon="📜"
      label={label}
      items={items}
      selected={selectedValue}
      onSelect={handleSelect}
      placeholder={isLoading ? 'Carregando...' : 'Buscar conversa...'}
      disabled={disabled || isLoading}
      maxWidth={maxWidth}
      onAnnounce={onAnnounce}
      onOpen={handleOpen}
    />
  );
};
