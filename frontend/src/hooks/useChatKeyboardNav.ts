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
        container.querySelectorAll('.chat-message')
      ) as HTMLElement[];

      // Escape: Always return focus to input
      if (e.key === 'Escape') {
        e.preventDefault();
        inputRef.current?.focus();
        focusedMessageIndex.current = -1;
        
        // Remove focus highlights
        messages.forEach((msg) => msg.classList.remove('chat-message--focused'));
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
        lastMessage?.classList.add('chat-message--focused');
        lastMessage?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        lastMessage?.focus();
        return;
      }

      // Navigate through messages with arrow keys
      if (focusedMessageIndex.current >= 0 && messages.length > 0) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          
          // Remove current focus
          messages[focusedMessageIndex.current]?.classList.remove('chat-message--focused');

          focusedMessageIndex.current++;
          
          if (focusedMessageIndex.current >= messages.length) {
            // Reached bottom, return to input
            focusedMessageIndex.current = -1;
            inputRef.current?.focus();
          } else {
            // Focus next message
            const nextMessage = messages[focusedMessageIndex.current];
            nextMessage?.classList.add('chat-message--focused');
            nextMessage?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            nextMessage?.focus();
          }
        } else if (e.key === 'ArrowUp') {
          e.preventDefault();
          
          // Remove current focus
          messages[focusedMessageIndex.current]?.classList.remove('chat-message--focused');

          focusedMessageIndex.current--;
          
          if (focusedMessageIndex.current < 0) {
            // Reached top, stay at first message
            focusedMessageIndex.current = 0;
          }
          
          // Focus previous message
          const prevMessage = messages[focusedMessageIndex.current];
          prevMessage?.classList.add('chat-message--focused');
          prevMessage?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
          prevMessage?.focus();
        }
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [enabled, inputRef, messagesContainerRef]);

  return { focusedMessageIndex };
};
