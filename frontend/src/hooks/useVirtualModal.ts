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

      // Esconde o resto da página para leitores de tela
      const appRoot = document.getElementById('root');
      if (appRoot) {
        appRoot.setAttribute('aria-hidden', 'true');
      }

      // O elemento em si precisa estar visível para o leitor de tela,
      // então o movemos para fora do root (via portal) ou garantimos
      // que ele não está dentro de algo aria-hidden.
      // Como o chat já está dentro de #root, precisamos usar uma técnica:
      // marcar todos os irmãos do ancestral mais próximo como inert.
      applyInert(el);

      // Foca no conteúdo da mensagem
      const contentEl = el.querySelector('.chat-message__text') as HTMLElement
        || el.querySelector('.chat-message__content') as HTMLElement
        || el;
      
      if (contentEl) {
        contentEl.setAttribute('tabindex', '0');
        contentEl.focus();
      }

      // Anuncia para leitores de tela
      announce(openAnnouncement);
    } else if (previousAriaAttrs.current) {
      // Restaura atributos ARIA
      restoreAttribute(el, 'role', previousAriaAttrs.current.role);
      restoreAttribute(el, 'aria-modal', previousAriaAttrs.current.ariaModal);
      restoreAttribute(el, 'aria-label', previousAriaAttrs.current.ariaLabel);
      restoreAttribute(el, 'tabindex', previousAriaAttrs.current.tabIndex);

      // Remove aria-hidden do root
      const appRoot = document.getElementById('root');
      if (appRoot) {
        appRoot.removeAttribute('aria-hidden');
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
        const appRoot = document.getElementById('root');
        if (appRoot) {
          appRoot.removeAttribute('aria-hidden');
        }
        removeInert();
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
