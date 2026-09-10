import {
  createContext,
  type KeyboardEvent,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useRenderedContentNavigation } from '../../hooks/useRenderedContentNavigation';

interface DocumentReadingRegionGroupValue {
  activeId: string | null;
  activate: (id: string) => void;
  deactivate: (id: string) => void;
}

const DocumentReadingRegionGroupContext = createContext<DocumentReadingRegionGroupValue | null>(
  null
);

export interface DocumentReadingRegionGroupProps {
  children: ReactNode;
}

/**
 * Coordena ilhas de leitura irmãs. Somente uma recebe role=document por vez.
 */
export function DocumentReadingRegionGroup({ children }: DocumentReadingRegionGroupProps) {
  const [activeId, setActiveId] = useState<string | null>(null);

  const value = useMemo<DocumentReadingRegionGroupValue>(
    () => ({
      activeId,
      activate: setActiveId,
      deactivate: (id) => setActiveId((current) => (current === id ? null : current)),
    }),
    [activeId]
  );

  return (
    <DocumentReadingRegionGroupContext.Provider value={value}>
      {children}
    </DocumentReadingRegionGroupContext.Provider>
  );
}

export interface DocumentReadingRegionProps {
  id: string;
  label: string;
  /** ID de heading/label já renderizado pelo consumidor. */
  labelledBy?: string;
  children: ReactNode;
  className?: string;
  headingClassName?: string;
  contentClassName?: string;
}

/**
 * Ilha documental delimitada para WebView2/NVDA.
 *
 * A âncora externa é estável. Ao receber foco, aguarda um frame para ativar a
 * semântica documental; o hook compartilhado aplica role=document antes de
 * focar o conteúdo. Tab não é capturado: o navegador segue para a próxima
 * ilha/ação e a ilha anterior é desativada no frame seguinte.
 */
export function DocumentReadingRegion({
  id,
  label,
  labelledBy,
  children,
  className,
  headingClassName,
  contentClassName,
}: DocumentReadingRegionProps) {
  const group = useContext(DocumentReadingRegionGroupContext);
  if (!group) {
    throw new Error('DocumentReadingRegion deve estar dentro de DocumentReadingRegionGroup');
  }

  const { t } = useTranslation();
  const reactId = useId();
  const headingId = labelledBy ?? `${reactId}-heading`;
  const anchorRef = useRef<HTMLDivElement>(null);
  const documentRef = useRef<HTMLDivElement>(null);
  const activationFrameRef = useRef<number | null>(null);
  const deactivationFrameRef = useRef<number | null>(null);
  const isActive = group.activeId === id;

  const cancelScheduledFrames = useCallback(() => {
    if (activationFrameRef.current !== null) {
      cancelAnimationFrame(activationFrameRef.current);
      activationFrameRef.current = null;
    }
    if (deactivationFrameRef.current !== null) {
      cancelAnimationFrame(deactivationFrameRef.current);
      deactivationFrameRef.current = null;
    }
  }, []);

  useEffect(() => cancelScheduledFrames, [cancelScheduledFrames]);

  useRenderedContentNavigation({
    elementRef: anchorRef,
    isActive,
    profile: 'scoped',
    contentSelector: '[data-document-reading-content="true"]',
    onEscape: () => undefined,
    handleEscape: false,
    restoreFocusOnDeactivate: false,
    openAnnouncement: t('ui.documentReadingRegion.opened', { label }),
  });

  const requestActivation = () => {
    if (isActive || activationFrameRef.current !== null) return;
    activationFrameRef.current = requestAnimationFrame(() => {
      activationFrameRef.current = null;
      if (anchorRef.current?.isConnected && document.activeElement === anchorRef.current) {
        group.activate(id);
      }
    });
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Tab' || !isActive) return;
    if (deactivationFrameRef.current !== null) {
      cancelAnimationFrame(deactivationFrameRef.current);
    }
    deactivationFrameRef.current = requestAnimationFrame(() => {
      deactivationFrameRef.current = null;
      group.deactivate(id);
    });
  };

  return (
    <section className={className} data-document-reading-region={id}>
      {!labelledBy && (
        <h2 id={headingId} className={headingClassName}>
          {label}
        </h2>
      )}
      <div
        ref={anchorRef}
        role="group"
        aria-labelledby={headingId}
        data-document-reading-anchor={id}
        tabIndex={isActive ? -1 : 0}
        onFocus={(event) => {
          if (event.target === event.currentTarget) requestActivation();
        }}
        onKeyDown={handleKeyDown}
      >
        <div
          ref={documentRef}
          className={contentClassName}
          data-document-reading-content="true"
          aria-labelledby={headingId}
        >
          {children}
        </div>
      </div>
    </section>
  );
}
