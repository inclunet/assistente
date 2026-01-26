import React, { useState } from 'react';
import { ChatMessage } from './ChatMessage';
import { MessageNode as MessageNodeType } from '../../store/chatStore';
import { useChatStore } from '../../store/chatStore';
import { playBumpSound } from '../../services/audioFeedback';
import './MessageNode.css';

export interface MessageNodeProps {
  node: MessageNodeType;
  level?: number;
  siblingIndex?: number;
  siblingCount?: number;
  onLoadChildren?: (messageId: string) => Promise<MessageNodeType[]>;
  onReachEnd?: () => void; // Chamado quando tenta ir além do último item no level 0
  onContextMenu?: (e: React.MouseEvent, message: any) => void;
  onOpenDetail?: (message: any) => void;
  onSpeak?: (message: any) => void;
}

export const MessageNode: React.FC<MessageNodeProps> = ({
  node,
  level = 0,
  siblingIndex = 0,
  siblingCount = 1,
  onLoadChildren,
  onReachEnd,
  onContextMenu,
  onOpenDetail,
  onSpeak,
}) => {
  const nodeRef = React.useRef<HTMLDivElement>(null);
  const toggleThreadExpanded = useChatStore(state => state.toggleThreadExpanded);
  const isThreadExpanded = useChatStore(state => state.isThreadExpanded);
  const [isLoading, setIsLoading] = useState(false);
  
  // Usa APENAS o estado da store para garantir reatividade
  const isExpanded = isThreadExpanded(node.message.id);
  
  // SIMPLIFICADO: Usa apenas node.children da store
  // - loadMessageChildren atualiza node.children na store
  // - addInternalMessage também atualiza node.children na store
  // - Não precisamos de estado local duplicado
  const children = node.children || [];

  const hasChildren = node.childCount > 0 || children.length > 0;

  const handleToggle = async () => {
    console.log('[MessageNode] 🔵 Toggle clicked for:', node.message.id, { 
      hasChildren, 
      isExpanded, 
      currentChildren: children.length,
      childCount: node.childCount 
    });
    
    if (!hasChildren) {
      console.log('[MessageNode] ⚠️ No children to toggle');
      return;
    }

    const wasExpanded = isExpanded;
    
    // Alterna expansão na store
    console.log('[MessageNode] 🔄 Toggling thread expanded state...');
    toggleThreadExpanded(node.message.id);
    
    // Aguarda um tick para garantir atualização do estado
    await new Promise(resolve => setTimeout(resolve, 0));
    
    const nowExpanded = isThreadExpanded(node.message.id);
    console.log('[MessageNode] ✅ After toggle:', { wasExpanded, nowExpanded });
    
    // Se estava fechado e tem filhos para carregar (childCount > 0 mas children.length === 0)
    // Isso acontece quando os filhos ainda não foram carregados do banco
    if (!wasExpanded && children.length === 0 && node.childCount > 0 && onLoadChildren) {
      console.log('[MessageNode] 📥 Loading children for:', node.message.id);
      setIsLoading(true);
      try {
        // onLoadChildren atualiza node.children na store, causando re-render automático
        await onLoadChildren(node.message.id);
        console.log('[MessageNode] ✅ Children loaded (store updated)');
      } catch (error) {
        console.error('[MessageNode] ❌ Error loading children:', error);
      } finally {
        setIsLoading(false);
      }
    }
  };

  const isInternal = node.message.internal || level > 0;
  
  // Funções de navegação por DOM (como no Svelte)
  const focusSibling = (idx: number) => {
    if (!nodeRef.current) return;
    
    const parent = nodeRef.current.parentElement;
    if (parent) {
      const siblings = Array.from(parent.children);
      const sibling = siblings[idx] as HTMLElement;
      if (sibling) {
        sibling.focus();
        return;
      }
    }
  };

  const focusParent = () => {
    if (!nodeRef.current || level === 0) return;
    
    // Estrutura: div > div.children > div (filho)
    const parentContainer = nodeRef.current.parentElement;
    if (parentContainer && parentContainer.classList.contains('message-node__children')) {
      const parentNode = parentContainer.parentElement;
      if (parentNode) {
        parentNode.focus();
      }
    }
  };

  const focusFirstChild = () => {
    if (!nodeRef.current) return;
    
    const childrenContainer = nodeRef.current.querySelector('.message-node__children');
    if (childrenContainer) {
      const firstChild = childrenContainer.firstElementChild as HTMLElement;
      if (firstChild) {
        firstChild.focus();
      }
    }
  };

  const expandAndFocusFirst = async () => {
    if (!hasChildren) return;
    
    if (!isExpanded) {
      await handleToggle();
      // Aguarda renderização dos filhos
      setTimeout(() => {
        focusFirstChild();
      }, 100);
    } else {
      // Já expandido, apenas foca no primeiro filho
      focusFirstChild();
    }
  };
  
  // Navegação por teclado (como no Svelte)
  const handleKeyDown = async (e: React.KeyboardEvent) => {
    const key = e.key;
    
    // Espaço: reproduz TTS da mensagem
    if (key === ' ' && !node.message.isStreaming) {
      e.preventDefault();
      e.stopPropagation();
      if (onSpeak) {
        onSpeak(node.message);
      }
      return;
    }
    
    // Enter abre modal de detalhes (delega para o handler)
    if (key === 'Enter' && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      if (onOpenDetail) {
        onOpenDetail(node.message);
      }
      return;
    }
    
    // ArrowDown: navega para próximo irmão
    if (key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      if (siblingIndex < siblingCount - 1) {
        focusSibling(siblingIndex + 1);
      } else if (level === 0 && onReachEnd) {
        // No nível principal, ao chegar no fim, vai para o input
        onReachEnd();
      } else if (level > 0) {
        // Em threads (level > 0), toca som ao tentar ir além
        playBumpSound();
      }
      // Nota: no level 0 não toca som porque vai para o input
      return;
    }
    
    // ArrowUp: navega para irmão anterior
    if (key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      if (siblingIndex > 0) {
        focusSibling(siblingIndex - 1);
      } else {
        // Bateu no primeiro irmão
        playBumpSound();
      }
      return;
    }
    
    // ArrowRight: expande E foca no primeiro filho
    if (key === 'ArrowRight') {
      e.preventDefault();
      e.stopPropagation();
      if (hasChildren) {
        await expandAndFocusFirst();
      }
      return;
    }
    
    // ArrowLeft/Escape: colapsa ou volta para o pai
    if (key === 'ArrowLeft' || key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      if (isExpanded && hasChildren) {
        // Colapsa se estiver expandido
        toggleThreadExpanded(node.message.id);
      } else if (level > 0) {
        // Volta para o pai se estiver em um nível interno
        focusParent();
      }
      return;
    }
    
    // Home: foca no primeiro irmão
    if (key === 'Home' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      focusSibling(0);
      return;
    }
    
    // End: foca no último irmão
    if (key === 'End' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      focusSibling(siblingCount - 1);
      return;
    }
    
    // Page Down: pula 10 mensagens para baixo
    if (key === 'PageDown' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      const targetIndex = Math.min(siblingIndex + 10, siblingCount - 1);
      focusSibling(targetIndex);
      if (targetIndex === siblingCount - 1 && siblingIndex === targetIndex) {
        // Já estava no último, toca som
        playBumpSound();
      }
      return;
    }
    
    // Page Up: pula 10 mensagens para cima
    if (key === 'PageUp' && !e.ctrlKey) {
      e.preventDefault();
      e.stopPropagation();
      const targetIndex = Math.max(siblingIndex - 10, 0);
      focusSibling(targetIndex);
      if (targetIndex === 0 && siblingIndex === 0) {
        // Já estava no primeiro, toca som
        playBumpSound();
      }
      return;
    }
  };

  // Handler para onKeyUp - captura ContextMenu key ou Shift+F10
  const handleKeyUp = (e: React.KeyboardEvent) => {
    if ((e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) && !node.message.internal) {
      e.preventDefault();
      e.stopPropagation();
      if (onContextMenu && nodeRef.current) {
        // Simula evento de mouse no centro do elemento
        const rect = nodeRef.current.getBoundingClientRect();
        const syntheticEvent = {
          preventDefault: () => {},
          stopPropagation: () => {},
          clientX: rect.left + rect.width / 2,
          clientY: rect.top + rect.height / 2,
          currentTarget: nodeRef.current, // Para restaurar foco após fechar menu
          target: nodeRef.current,
        } as React.MouseEvent;
        onContextMenu(syntheticEvent, node.message);
      }
    }
  };

  return (
    <div
      ref={nodeRef}
      className={`message-node message-node--level-${level} ${isInternal ? 'message-node--internal' : ''}`}
      data-level={level}
      data-sibling-index={siblingIndex}
      data-message-node
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      tabIndex={-1}
      role="listitem"
      aria-expanded={hasChildren ? isExpanded : undefined}
    >
      <div className="message-node__content">
        <ChatMessage 
          message={node.message}
          hasThreadIndicator={hasChildren}
          threadChildCount={node.childCount || children.length}
          isThreadExpanded={isExpanded}
          isThreadLoading={isLoading}
          onThreadToggle={handleToggle}
          onContextMenu={onContextMenu}
          onOpenDetail={onOpenDetail}
          onSpeak={onSpeak}
        />
      </div>

      {isExpanded && children.length > 0 && (
        <div className="message-node__children" role="list" aria-label="Respostas internas">
          {children.map((childNode, index) => (
            <MessageNode
              key={childNode.message.id || index}
              node={childNode}
              level={level + 1}
              siblingIndex={index}
              siblingCount={children.length}
              onLoadChildren={onLoadChildren}
              onContextMenu={onContextMenu}
              onOpenDetail={onOpenDetail}
              onSpeak={onSpeak}
              // Não passa onReachEnd para threads internas
            />
          ))}
        </div>
      )}
    </div>
  );
};
