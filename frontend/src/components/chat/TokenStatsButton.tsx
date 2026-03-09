import React, { useState, useEffect } from 'react';
import { GetConversationTokenStats } from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import './TokenStatsButton.css';

interface TokenStats {
  conversationId: number;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  messageCount: number;
  mostUsedModel: string;
  contextUsage: number;
  contextLimit: number;
  isNearLimit: boolean;
  isCritical: boolean;
}

interface TokenStatsButtonProps {
  conversationId?: number;
  onOpenModal: () => void;
}

export const TokenStatsButton: React.FC<TokenStatsButtonProps> = ({
  conversationId,
  onOpenModal,
}) => {
  const [stats, setStats] = useState<TokenStats | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (!conversationId) {
      setStats(null);
      return;
    }

    // Função para carregar stats do backend
    const loadStats = () => {
      setIsLoading(true);
      GetConversationTokenStats(conversationId)
        .then((data) => {
          console.log('[TokenStatsButton] Stats carregadas:', data);
          setStats(data);
        })
        .catch((error) => {
          console.error('[TokenStatsButton] Erro ao carregar stats:', error);
          // Define stats padrão em caso de erro
          setStats({
            conversationId,
            promptTokens: 0,
            completionTokens: 0,
            totalTokens: 0,
            messageCount: 0,
            mostUsedModel: '',
            contextUsage: 0,
            contextLimit: 128000,
            isNearLimit: false,
            isCritical: false,
          });
        })
        .finally(() => setIsLoading(false));
    };

    // Carrega stats iniciais
    loadStats();

    // Escuta atualizações de tokens (emitido após streaming)
    const unsubscribeTokens = EventsOn('chat:token_stats', (data: TokenStats) => {
      if (data.conversationId === conversationId) {
        console.log('[TokenStatsButton] Stats atualizadas via evento chat:token_stats:', data);
        setStats(data);
      }
    });

    // Escuta quando mensagens do usuário são adicionadas (antes do streaming)
    const unsubscribeMessages = EventsOn('chat:messages_ready', (data: any) => {
      if (data.conversationId === conversationId) {
        console.log('[TokenStatsButton] Mensagens prontas, recarregando stats...');
        loadStats();
      }
    });

    // Escuta quando o streaming termina (garantia extra de atualização)
    const unsubscribeDone = EventsOn('chat:done', (data: any) => {
      if (data.conversationId === conversationId) {
        console.log('[TokenStatsButton] Chat finalizado, recarregando stats...');
        loadStats();
      }
    });

    return () => {
      unsubscribeTokens();
      unsubscribeMessages();
      unsubscribeDone();
    };
  }, [conversationId]);

  // Não renderiza se não houver conversationId
  if (!conversationId) {
    console.log('[TokenStatsButton] Não renderizando: sem conversationId');
    return null;
  }

  // Mostra loading ou valores padrão enquanto carrega
  if (isLoading || !stats) {
    return (
      <button
        className="token-stats-button token-stats-button--loading"
        disabled
        aria-label="Carregando estatísticas de tokens"
      >
        <span className="token-stats-button__icon" aria-hidden="true">⏳</span>
        <span className="token-stats-button__text">...</span>
      </button>
    );
  }

  const formatNumber = (num: number): string => {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(1)}M`;
    }
    if (num >= 1000) {
      return `${(num / 1000).toFixed(1)}K`;
    }
    return num.toString();
  };

  const getStatusColor = (): string => {
    if (stats.isCritical) return 'critical';
    if (stats.isNearLimit) return 'warning';
    return 'normal';
  };

  const getStatusIcon = (): string => {
    if (stats.isCritical) return '🔴';
    if (stats.isNearLimit) return '🟡';
    return '📊';
  };

  // Se não houver limite de contexto configurado, mostra apenas o total
  const hasContextLimit = stats.contextLimit > 0;
  const ariaLabel = hasContextLimit
    ? `${formatNumber(stats.totalTokens)} de ${formatNumber(stats.contextLimit)} tokens`
    : `${formatNumber(stats.totalTokens)} tokens`;

  return (
    <button
      className={`token-stats-button token-stats-button--${getStatusColor()}`}
      onClick={onOpenModal}
      aria-label={`${ariaLabel}, Ctrl+T para ver detalhes`}
      title="Ver estatísticas detalhadas de tokens (Ctrl+T)"
    >
      <span className="token-stats-button__icon" aria-hidden="true">
        {getStatusIcon()}
      </span>
      <span className="token-stats-button__text">
        {formatNumber(stats.totalTokens)}
        {hasContextLimit && (
          <>
            <span className="token-stats-button__separator">/</span>
            {formatNumber(stats.contextLimit)}
          </>
        )}
      </span>
    </button>
  );
};
