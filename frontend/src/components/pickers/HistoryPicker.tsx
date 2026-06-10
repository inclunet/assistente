import { logger } from '../../utils/logger';
import { useState, useEffect, useCallback, forwardRef, useImperativeHandle } from 'react';
import { useTranslation } from 'react-i18next';
import { HistoryOutlined } from '@ant-design/icons';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { GetConversations } from '@wailsjs/go/app/App';
import { database } from '../../../wailsjs/go/models';
import { EventsOn } from '@wailsjs/runtime/runtime';

export interface HistoryPickerProps {
  value?: string; // ID da conversa atual
  onChange: (conversationId: string, conversation: database.Conversation) => void;
  label?: string;
  description?: string;
  disabled?: boolean;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
  /**
   * Itens extras fixados no topo da lista (ex.: "Nenhuma"/desvincular).
   * O valor de cada item NÃO deve colidir com um ID de conversa.
   */
  extraItems?: ComboboxItem[];
  /** Chamado ao selecionar um extraItem (valor que não corresponde a uma conversa). */
  onSelectExtra?: (value: string) => void;
}

export interface HistoryPickerRef {
  reload: () => Promise<void>;
  getSelectedConversation: () => database.Conversation | undefined;
}

export const HistoryPicker = forwardRef<HistoryPickerRef, HistoryPickerProps>(({
  value,
  onChange,
  label,
  description,
  disabled = false,
  maxWidth = '200px',
  onAnnounce,
  extraItems,
  onSelectExtra
}, ref) => {
  const { t } = useTranslation();
  const resolvedLabel = label ?? t('history.pickerLabel', 'Histórico (Ctrl+H)');
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
        const dateA = new Date(a.updatedAt as string | number | Date).getTime();
        const dateB = new Date(b.updatedAt as string | number | Date).getTime();
        return dateB - dateA;
      });
      setConversations(sorted);
    } catch (error) {
      logger.error('[HistoryPicker] Erro ao carregar conversas:', error);
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
      loadConversations();
    };

    const handleConversationRenamed = () => {
      loadConversations();
    };

    const handleConversationDeleted = () => {
      loadConversations();
    };

    const unsubCreated = EventsOn('chat:conversation_created', handleConversationCreated);
    const unsubRenamed = EventsOn('conversation:renamed', handleConversationRenamed);
    const unsubDeleted = EventsOn('conversation:deleted', handleConversationDeleted);

    return () => {
      unsubCreated();
      unsubRenamed();
      unsubDeleted();
    };
  }, [loadConversations]);

  // Callback quando o picker é aberto - recarrega conversas
  const handleOpen = useCallback(() => {
    loadConversations();
  }, [loadConversations]);

  useImperativeHandle(ref, () => ({
    reload: loadConversations,
    getSelectedConversation: () => conversations.find(c => c.id === value),
  }));

  // Formata data para exibição
  const formatDate = (dateValue: string | number | Date): string => {
    const date = new Date(dateValue);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return t('history.relativeNow', 'agora');
    if (diffMins < 60) return t('history.relativeMinutes', '{{count}}min atrás', { count: diffMins });
    if (diffHours < 24) return t('history.relativeHours', '{{count}}h atrás', { count: diffHours });
    if (diffDays < 7) return t('history.relativeDays', '{{count}}d atrás', { count: diffDays });

    return date.toLocaleDateString(undefined, {
      day: '2-digit',
      month: '2-digit',
      year: date.getFullYear() !== now.getFullYear() ? 'numeric' : undefined
    });
  };

  // Converte conversas para items do Combobox. extraItems (ex.: "Nenhuma")
  // ficam fixados no topo da lista.
  const items: ComboboxItem[] = [
    ...(extraItems ?? []),
    ...conversations.map(conv => ({
      value: conv.id.toString(),
      label: conv.title || t('history.untitled', 'Sem título'),
      sublabel: `${t('history.messagesCount', '{{count}} msgs', { count: conv.message_count || 0 })} • ${formatDate(conv.updatedAt)}`
    })),
  ];

  // Valor selecionado (string vazia se não houver conversa selecionada)
  const selectedValue = value ? value.toString() : '';

  const handleSelect = (selectedValue: string) => {
    const conversation = conversations.find(c => String(c.id) === selectedValue);
    if (conversation) {
      onChange(selectedValue, conversation);
      return;
    }
    if (extraItems?.some(item => item.value === selectedValue)) {
      onSelectExtra?.(selectedValue);
    }
  };

  return (
    <BasePicker
      variant="toolbar"
      items={items}
      selected={selectedValue}
      onSelect={handleSelect}
      label={resolvedLabel}
      description={description}
      icon={<HistoryOutlined />}
      placeholder={isLoading ? t('history.loadingShort', 'Carregando...') : t('history.searchPlaceholder', 'Buscar conversa...')}
      disabled={disabled || isLoading}
      maxWidth={maxWidth}
      onAnnounce={onAnnounce}
      onOpen={handleOpen}
      showLoadingState={false}
      showEmptyState={false}
      wrapCombobox={false}
    />
  );
});

HistoryPicker.displayName = 'HistoryPicker';
