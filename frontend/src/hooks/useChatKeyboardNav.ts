import { useEffect, useRef, RefObject } from 'react';

export interface UseChatKeyboardNavProps {
  enabled?: boolean;
  inputRef: RefObject<HTMLTextAreaElement>;
  messagesContainerRef: RefObject<HTMLDivElement>;
}

export const useChatKeyboardNav = ({
  enabled = true,
  inputRef,
  messagesContainerRef,
}: UseChatKeyboardNavProps) => {
  const focusedMessageIndex = useRef<number>(-1);

  useEffect(() => {
    if (!enabled) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const container = messagesContainerRef.current;
      if (!container) return;

      const messages = Array.from(
        container.querySelectorAll('.message-node')
      ) as HTMLElement[];

      // Eventos Enter e ContextMenu são tratados pelo MessageNode/ChatMessage
      // Este hook só gerencia navegação global com setas e Escape

      // Escape: Always return focus to input
      if (e.key === 'Escape') {
        e.preventDefault();
        inputRef.current?.focus();
        focusedMessageIndex.current = -1;
        return;
      }

      // If focus is on input and user presses ArrowUp, start navigating messages
      if (
        document.activeElement === inputRef.current &&
        e.key === 'ArrowUp' &&
        messages.length > 0
      ) {
        e.preventDefault();
        focusedMessageIndex.current = messages.length - 1;
        const lastMessage = messages[focusedMessageIndex.current];
        lastMessage?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        lastMessage?.focus();
        return;
      }

      // Navegação entre mensagens é tratada pelo próprio MessageNode
      // Este hook apenas inicia a navegação do input
    };

    document.addEventListener('keydown', handleKeyDown);
    
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [enabled, inputRef, messagesContainerRef]);

  return { focusedMessageIndex };
};
