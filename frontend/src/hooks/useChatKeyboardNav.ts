import { useEffect, useRef, RefObject } from 'react';
import { announce } from './useAnnouncer';

export interface UseChatKeyboardNavProps {
  enabled?: boolean;
  inputRef: RefObject<HTMLTextAreaElement>;
  messagesContainerRef: RefObject<HTMLDivElement>;
}

/**
 * Returns true when `key` is a single printable character that should
 * trigger type-ahead (letters, digits, punctuation, space-like chars).
 */
function isPrintableKey(e: KeyboardEvent): boolean {
  if (e.key.length !== 1) return false;
  if (e.ctrlKey || e.altKey || e.metaKey) return false;
  return true;
}

const nativeTextAreaSetter = Object.getOwnPropertyDescriptor(
  window.HTMLTextAreaElement.prototype,
  'value',
)?.set;

/**
 * Inserts `char` at the caret position of a React-controlled textarea,
 * triggering onChange correctly.
 */
function insertCharIntoTextarea(textarea: HTMLTextAreaElement, char: string) {
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;
  const before = textarea.value.slice(0, start);
  const after = textarea.value.slice(end);
  const newValue = before + char + after;

  if (nativeTextAreaSetter) {
    nativeTextAreaSetter.call(textarea, newValue);
  } else {
    textarea.value = newValue;
  }

  textarea.selectionStart = textarea.selectionEnd = start + char.length;
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
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

      // Escape is handled by useLandmarkNavigation (→ default area)

      // If focus is on input and user presses ArrowUp, start navigating messages
      if (
        document.activeElement === inputRef.current &&
        e.key === 'ArrowUp' &&
        messages.length > 0
      ) {
        // #region agent log
        fetch('http://127.0.0.1:7271/ingest/fb09268b-5fc3-4325-9bc8-e9411ee258d2',{method:'POST',headers:{'Content-Type':'application/json','X-Debug-Session-Id':'eb006c'},body:JSON.stringify({sessionId:'eb006c',runId:'chat-history-nav-pre-fix-1',hypothesisId:'H3',location:'frontend/src/hooks/useChatKeyboardNav.ts:67',message:'chat history entry from input',data:{messageCount:messages.length,activeElement:(document.activeElement as HTMLElement | null)?.className ?? null,targetMessageId:messages[messages.length - 1]?.getAttribute('data-message-id') ?? null},timestamp:Date.now()})}).catch(()=>{});
        // #endregion
        e.preventDefault();
        focusedMessageIndex.current = messages.length - 1;
        const lastMessage = messages[focusedMessageIndex.current];
        lastMessage?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        lastMessage?.focus();
        return;
      }

      // Type-ahead: when browsing messages and a printable character is typed,
      // redirect focus to the input and insert the character there.
      if (
        container.contains(document.activeElement) &&
        document.activeElement !== inputRef.current &&
        isPrintableKey(e)
      ) {
        e.preventDefault();
        const textarea = inputRef.current;
        if (textarea) {
          textarea.focus();
          insertCharIntoTextarea(textarea, e.key);
          announce(textarea.getAttribute('aria-label') || '');
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
