import { useEffect, useRef } from 'react';
import { announce } from './useAnnouncer';

export interface UseVirtualModalOptions {
  /** Referência ao elemento que será o "modal virtual" */
  elementRef: React.RefObject<HTMLElement | null>;
  /** Se o modo leitura está ativo */
  isActive: boolean;
  /** Callback para desativar o modo leitura */
  onClose: () => void;
  /** Label para anúncio ao abrir */
  openAnnouncement?: string;
  /** Label para anúncio ao fechar */
  closeAnnouncement?: string;
}

/**
 * Hook que transforma qualquer elemento em um "modal virtual".
 * 
 * Para leitores de tela: o elemento recebe role="dialog", aria-modal="true",
 * focus trap e o restante da página fica aria-hidden.
 * 
 * Para quem vê: nada muda na estrutura/layout — apenas uma classe CSS
 * é adicionada para indicação visual sutil.
 * 
 * ESC fecha o modo, restaurando o estado anterior.
 */
export function useVirtualModal({
  elementRef,
  isActive,
  onClose,
  openAnnouncement = 'Modo de leitura ativado. Pressione Escape para sair.',
  closeAnnouncement = 'Modo de leitura desativado.',
}: UseVirtualModalOptions) {
  const previousActiveElement = useRef<HTMLElement | null>(null);
  const previousAriaAttrs = useRef<{
    role: string | null;
    ariaModal: string | null;
    ariaLabel: string | null;
    tabIndex: string | null;
  } | null>(null);
  // Guarda os atributos do elemento de conteúdo (alvo do foco) para que o
  // `role="document"` aplicado durante a leitura seja restaurado ao sair.
  const previousContentAttrs = useRef<{
    element: HTMLElement;
    role: string | null;
    tabIndex: string | null;
  } | null>(null);

  // Ativa/desativa o modo dialog no elemento
  useEffect(() => {
    const el = elementRef.current;
    if (!el) return;

    if (isActive) {
      // Salva o elemento previamente focado
      previousActiveElement.current = document.activeElement as HTMLElement;

      // Salva atributos ARIA anteriores
      previousAriaAttrs.current = {
        role: el.getAttribute('role'),
        ariaModal: el.getAttribute('aria-modal'),
        ariaLabel: el.getAttribute('aria-label'),
        tabIndex: el.getAttribute('tabindex'),
      };

      // Transforma em dialog
      el.setAttribute('role', 'dialog');
      el.setAttribute('aria-modal', 'true');
      el.setAttribute('aria-label', 'Leitura de mensagem. Pressione Escape para sair.');
      el.setAttribute('tabindex', '-1');

      // Virtual modal vive dentro de #root, então não podemos marcar #root como
      // aria-hidden sem esconder também o próprio elemento focado. Em vez disso,
      // isolamos a interação usando inert nos irmãos ao longo da árvore.
      applyInert(el);

      // Foca no conteúdo da mensagem. O alvo precisa englobar a CADEIA INTEIRA
      // do turno (todos os segmentos de texto E as chamadas de ferramenta, em
      // ordem) — não apenas o primeiro `.chat-message__text`. `.chat-message__content`
      // é o container estável que envolve cabeçalho, segmentos e tool calls, então
      // `role="document"` aqui expõe o turno inteiro à navegação do leitor de tela
      // (Issue #163). Mantém o fallback para `.chat-message__text` (mensagens
      // simples e harness de teste).
      const contentEl = el.querySelector('.chat-message__content') as HTMLElement | null
        ?? el.querySelector('.chat-message__text') as HTMLElement | null;

      if (contentEl && contentEl !== el) {
        // Só aplicamos `role="document"`/`tabindex` (e salvamos para restaurar)
        // quando há um elemento de conteúdo REAL (filho). Aplicá-los no próprio
        // `el` sobrescreveria o `role="dialog"` recém-definido e deixaria
        // role/tabindex inconsistentes ao sair — a restauração do `el` já é
        // tratada por `previousAriaAttrs`.
        previousContentAttrs.current = {
          element: contentEl,
          role: contentEl.getAttribute('role'),
          tabIndex: contentEl.getAttribute('tabindex'),
        };
        contentEl.setAttribute('tabindex', '0');
        // `role="document"` faz o NVDA alternar do modo de foco (forçado pelo
        // `role="application"` do #root) para o modo de navegação enquanto o
        // usuário lê o conteúdo. Ao sair (ESC), o role é restaurado.
        contentEl.setAttribute('role', 'document');
        contentEl.focus();
      } else {
        // Sem elemento de conteúdo: foca o próprio dialog, preservando seu
        // `role="dialog"`. A restauração fica exclusivamente a cargo de
        // `previousAriaAttrs`.
        el.focus();
      }

      // Anuncia para leitores de tela
      announce(openAnnouncement);
    } else if (previousAriaAttrs.current) {
      // Restaura atributos ARIA
      restoreAttribute(el, 'role', previousAriaAttrs.current.role);
      restoreAttribute(el, 'aria-modal', previousAriaAttrs.current.ariaModal);
      restoreAttribute(el, 'aria-label', previousAriaAttrs.current.ariaLabel);
      restoreAttribute(el, 'tabindex', previousAriaAttrs.current.tabIndex);

      // Restaura atributos do elemento de conteúdo (role/tabindex).
      if (previousContentAttrs.current) {
        const { element, role, tabIndex } = previousContentAttrs.current;
        restoreAttribute(element, 'role', role);
        restoreAttribute(element, 'tabindex', tabIndex);
        previousContentAttrs.current = null;
      }

      // Remove inert dos irmãos
      removeInert();

      // Restaura foco
      previousActiveElement.current?.focus();
      previousAriaAttrs.current = null;
    }

    return () => {
      // Cleanup: se desmontar enquanto ativo
      if (isActive) {
        removeInert();
        if (previousContentAttrs.current) {
          const { element, role, tabIndex } = previousContentAttrs.current;
          restoreAttribute(element, 'role', role);
          restoreAttribute(element, 'tabindex', tabIndex);
          previousContentAttrs.current = null;
        }
      }
    };
  }, [isActive]);

  // Focus trap: impede Tab de sair do elemento
  useEffect(() => {
    if (!isActive) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const el = elementRef.current;
      if (!el) return;

      // ESC fecha o modo leitura
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        onClose();
        announce(closeAnnouncement);
        return;
      }

      // Focus trap via Tab
      if (e.key === 'Tab') {
        const focusableElements = el.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        );

        // Se não há elementos focáveis além do conteúdo, prender no próprio elemento
        if (focusableElements.length === 0) {
          e.preventDefault();
          return;
        }

        const firstElement = focusableElements[0];
        const lastElement = focusableElements[focusableElements.length - 1];

        if (e.shiftKey && document.activeElement === firstElement) {
          e.preventDefault();
          lastElement?.focus();
        } else if (!e.shiftKey && document.activeElement === lastElement) {
          e.preventDefault();
          firstElement?.focus();
        }
      }
    };

    // Usa capture para interceptar antes dos outros handlers
    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [isActive, onClose, closeAnnouncement]);
}

// --- Helpers ---

function restoreAttribute(el: HTMLElement, attr: string, value: string | null) {
  if (value !== null) {
    el.setAttribute(attr, value);
  } else {
    el.removeAttribute(attr);
  }
}

const INERT_ATTR = 'data-virtual-modal-inert';

/**
 * Aplica `inert` em todos os elementos que NÃO são ancestrais do elemento alvo.
 * Isso garante que apenas o "modal virtual" permanece interativo.
 */
function applyInert(activeElement: HTMLElement) {
  // Percorre de #root até o pai direto do elemento, marcando irmãos como inert
  let current: HTMLElement | null = activeElement;
  
  while (current && current !== document.body) {
    const parent: HTMLElement | null = current.parentElement;
    if (parent) {
      const currentEl = current; // Captura para o closure
      Array.from(parent.children).forEach(sibling => {
        if (sibling !== currentEl && sibling instanceof HTMLElement && !sibling.hasAttribute('inert')) {
          sibling.setAttribute('inert', '');
          sibling.setAttribute(INERT_ATTR, 'true');
        }
      });
    }
    current = parent;
  }
}

/**
 * Remove `inert` de todos os elementos marcados pelo applyInert.
 */
function removeInert() {
  document.querySelectorAll(`[${INERT_ATTR}]`).forEach(el => {
    el.removeAttribute('inert');
    el.removeAttribute(INERT_ATTR);
  });
}
