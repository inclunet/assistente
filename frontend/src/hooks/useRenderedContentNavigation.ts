import { useEffect, useRef } from 'react';
import { announce } from './useAnnouncer';

export type RenderedContentNavigationProfile = 'modal' | 'scoped';

interface RenderedContentNavigationBaseOptions {
  /** Container que delimita a superfície renderizada. */
  elementRef: React.RefObject<HTMLElement | null>;
  /** Ativa a semântica de documento e move o foco para o conteúdo. */
  isActive: boolean;
  /** Seletor preferencial do elemento que recebe role=document. */
  contentSelector?: string;
  /** Executado por Escape. No perfil modal normalmente desativa a leitura. */
  onEscape: () => void;
  openAnnouncement?: string;
  closeAnnouncement?: string;
  /** Impede que Escape concorra com um modal interno no topo da tela. */
  shouldHandleEscape?: () => boolean;
  /** Padrão: true no perfil modal e false no perfil scoped. */
  restoreFocusOnDeactivate?: boolean;
  /** Padrão true; false quando o consumidor controla role/tabIndex via React. */
  manageDocumentSemantics?: boolean;
}

export type UseRenderedContentNavigationOptions =
  RenderedContentNavigationBaseOptions
  & (
    | {
      /** Modal contém interação, exige nome acessível e prende Tab/inert. */
      profile: 'modal';
      dialogLabel: string;
    }
    | {
      /** Scoped permite que Tab e F6 saiam normalmente. */
      profile: 'scoped';
      dialogLabel?: never;
    }
  );

interface PreviousElementAttrs {
  role: string | null;
  ariaModal: string | null;
  ariaLabel: string | null;
  tabIndex: string | null;
}

interface PreviousContentAttrs {
  element: HTMLElement;
  role: string | null;
  tabIndex: string | null;
}

const FOCUSABLE_SELECTOR = [
  'button:not(:disabled):not([tabindex="-1"])',
  'a[href]:not([tabindex="-1"])',
  'input:not(:disabled):not([tabindex="-1"])',
  'select:not(:disabled):not([tabindex="-1"])',
  'textarea:not(:disabled):not([tabindex="-1"])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ');

/**
 * Contrato comum para leitura de HTML renderizado.
 *
 * O perfil modal é usado por mensagens individuais e contém Tab/inert. O
 * perfil scoped é usado por documentos do editor: aplica role=document e foco,
 * mas não impede que Tab e F6 alcancem o restante da tela.
 */
export function useRenderedContentNavigation({
  elementRef,
  isActive,
  profile,
  contentSelector,
  onEscape,
  openAnnouncement,
  closeAnnouncement,
  dialogLabel,
  shouldHandleEscape,
  restoreFocusOnDeactivate,
  manageDocumentSemantics = true,
}: UseRenderedContentNavigationOptions) {
  const previousActiveElement = useRef<HTMLElement | null>(null);
  const previousElementAttrs = useRef<PreviousElementAttrs | null>(null);
  const previousContentAttrs = useRef<PreviousContentAttrs | null>(null);
  const profileRef = useRef(profile);
  const contentSelectorRef = useRef(contentSelector);
  const onEscapeRef = useRef(onEscape);
  const openAnnouncementRef = useRef(openAnnouncement);
  const closeAnnouncementRef = useRef(closeAnnouncement);
  const dialogLabelRef = useRef(dialogLabel);
  const shouldHandleEscapeRef = useRef(shouldHandleEscape);
  const restoreFocusRef = useRef(restoreFocusOnDeactivate ?? profile === 'modal');
  const manageDocumentSemanticsRef = useRef(manageDocumentSemantics);

  profileRef.current = profile;
  contentSelectorRef.current = contentSelector;
  onEscapeRef.current = onEscape;
  openAnnouncementRef.current = openAnnouncement;
  closeAnnouncementRef.current = closeAnnouncement;
  dialogLabelRef.current = dialogLabel;
  shouldHandleEscapeRef.current = shouldHandleEscape;
  restoreFocusRef.current = restoreFocusOnDeactivate ?? profile === 'modal';
  manageDocumentSemanticsRef.current = manageDocumentSemantics;

  useEffect(() => {
    const element = elementRef.current;
    if (!element || !isActive) return;

    const activeProfile = profileRef.current;
    let focusFrame: number | null = null;
    let focusTimer: number | null = null;
    previousActiveElement.current = document.activeElement as HTMLElement | null;
    previousElementAttrs.current = {
      role: element.getAttribute('role'),
      ariaModal: element.getAttribute('aria-modal'),
      ariaLabel: element.getAttribute('aria-label'),
      tabIndex: element.getAttribute('tabindex'),
    };

    if (activeProfile === 'modal') {
      const accessibleLabel = dialogLabelRef.current?.trim();
      if (!accessibleLabel) {
        throw new Error('dialogLabel must be a non-empty string for modal rendered content');
      }
      element.setAttribute('role', 'dialog');
      element.setAttribute('aria-modal', 'true');
      element.setAttribute('aria-label', accessibleLabel);
      element.setAttribute('tabindex', '-1');
      applyInert(element);
    }

    const selector = contentSelectorRef.current;
    const contentElement = (
      selector ? element.querySelector<HTMLElement>(selector) : null
    )
      ?? element.querySelector<HTMLElement>('.chat-message__content')
      ?? element.querySelector<HTMLElement>('.chat-message__text');

    const focusTarget = contentElement && contentElement !== element
      ? contentElement
      : element;

    if (contentElement && contentElement !== element) {
      if (manageDocumentSemanticsRef.current) {
        previousContentAttrs.current = {
          element: contentElement,
          role: contentElement.getAttribute('role'),
          tabIndex: contentElement.getAttribute('tabindex'),
        };
        contentElement.setAttribute('role', 'document');
        contentElement.setAttribute('tabindex', '0');
      }
    }

    const focusAndAnnounce = () => {
      if (!focusTarget.isConnected) return;
      focusTarget.focus();
      if (openAnnouncementRef.current) announce(openAnnouncementRef.current);
    };

    if (activeProfile === 'scoped') {
      // O preview já está focado como `group` quando Enter ativa a leitura.
      // Remover esse foco e devolvê-lo após um frame + uma nova tarefa permite
      // que a árvore de acessibilidade do WebView2 publique `role=document`
      // antes do novo evento de foco consumido pelo NVDA.
      if (document.activeElement === focusTarget) focusTarget.blur();
      focusFrame = window.requestAnimationFrame(() => {
        focusTimer = window.setTimeout(focusAndAnnounce, 0);
      });
    } else {
      focusAndAnnounce();
    }

    return () => {
      if (focusFrame !== null) window.cancelAnimationFrame(focusFrame);
      if (focusTimer !== null) window.clearTimeout(focusTimer);
      if (previousElementAttrs.current) {
        restoreNavigationState(
          element,
          activeProfile,
          previousElementAttrs.current,
          previousContentAttrs.current,
        );
      }
      previousContentAttrs.current = null;
      previousElementAttrs.current = null;
      if (restoreFocusRef.current) previousActiveElement.current?.focus();
    };
  }, [elementRef, isActive]);

  useEffect(() => {
    if (!isActive) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      const element = elementRef.current;
      if (!element) return;

      if (event.key === 'Escape') {
        const activeProfile = profileRef.current;
        let isScopedDefaultArea = false;
        if (activeProfile === 'scoped') {
          const activeElement = document.activeElement;
          if (!activeElement || !element.contains(activeElement)) return;

          const selector = contentSelectorRef.current;
          const contentElement = (
            selector ? element.querySelector<HTMLElement>(selector) : null
          )
            ?? element.querySelector<HTMLElement>('.chat-message__content')
            ?? element.querySelector<HTMLElement>('.chat-message__text')
            ?? element;
          isScopedDefaultArea = activeElement === contentElement;
        }

        const explicitEscapePolicy = shouldHandleEscapeRef.current;
        if (explicitEscapePolicy) {
          if (!explicitEscapePolicy()) return;
        }
        if (isScopedDefaultArea) {
          event.preventDefault();
          event.stopPropagation();
          return;
        }
        event.preventDefault();
        event.stopPropagation();
        onEscapeRef.current();
        if (closeAnnouncementRef.current) announce(closeAnnouncementRef.current);
        return;
      }

      if (profileRef.current !== 'modal' || event.key !== 'Tab') return;

      const focusableElements = Array.from(
        element.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
      );
      if (focusableElements.length === 0) {
        event.preventDefault();
        return;
      }

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];

      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [elementRef, isActive]);
}

function restoreNavigationState(
  element: HTMLElement,
  profile: RenderedContentNavigationProfile,
  previousElement: PreviousElementAttrs,
  previousContent: PreviousContentAttrs | null,
) {
  if (profile === 'modal') {
    restoreAttribute(element, 'role', previousElement.role);
    restoreAttribute(element, 'aria-modal', previousElement.ariaModal);
    restoreAttribute(element, 'aria-label', previousElement.ariaLabel);
    restoreAttribute(element, 'tabindex', previousElement.tabIndex);
    removeInert();
  }

  if (previousContent) {
    restoreAttribute(previousContent.element, 'role', previousContent.role);
    restoreAttribute(previousContent.element, 'tabindex', previousContent.tabIndex);
  }
}

function restoreAttribute(element: HTMLElement, attribute: string, value: string | null) {
  if (value === null) {
    element.removeAttribute(attribute);
  } else {
    element.setAttribute(attribute, value);
  }
}

const INERT_ATTR = 'data-rendered-content-inert';

function applyInert(activeElement: HTMLElement) {
  let current: HTMLElement | null = activeElement;
  while (current && current !== document.body) {
    const parent: HTMLElement | null = current.parentElement;
    if (parent) {
      const currentElement = current;
      Array.from(parent.children).forEach((sibling) => {
        if (
          sibling !== currentElement
          && sibling instanceof HTMLElement
          && !sibling.hasAttribute('inert')
        ) {
          sibling.setAttribute('inert', '');
          sibling.setAttribute(INERT_ATTR, 'true');
        }
      });
    }
    current = parent;
  }
}

function removeInert() {
  document.querySelectorAll(`[${INERT_ATTR}]`).forEach((element) => {
    element.removeAttribute('inert');
    element.removeAttribute(INERT_ATTR);
  });
}
