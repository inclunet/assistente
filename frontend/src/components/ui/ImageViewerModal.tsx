import { useCallback, useEffect, useId, useRef, useState } from 'react';
import {
  ZoomInOutlined,
  ZoomOutOutlined,
  CompressOutlined,
  LeftOutlined,
  RightOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Modal, useModalIsTopmost } from './Modal';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './ImageViewerModal.css';

export interface ImageViewerImage {
  src: string;
  alt?: string;
}

export interface ImageViewerModalProps {
  isOpen: boolean;
  images: ImageViewerImage[];
  initialIndex?: number;
  onClose: () => void;
}

const MIN_ZOOM = 1;
const MAX_ZOOM = 5;
const ZOOM_STEP = 0.5;

const clampZoom = (value: number) =>
  Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Number(value.toFixed(2))));

interface ImageViewerViewProps {
  images: ImageViewerImage[];
  initialIndex: number;
  captionId: string;
}

/**
 * Conteúdo do visualizador. Renderizado como filho do Modal para poder
 * consumir `useModalIsTopmost` e só reagir ao teclado quando o viewer for o
 * modal do topo da stack. Remonta a cada abertura, zerando índice e zoom.
 */
function ImageViewerView({ images, initialIndex, captionId }: ImageViewerViewProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const isTopmost = useModalIsTopmost();
  const stageRef = useRef<HTMLDivElement>(null);
  const didMountRef = useRef(false);
  const [index, setIndex] = useState(initialIndex);
  const [zoom, setZoom] = useState(MIN_ZOOM);

  const total = images.length;
  const hasMultiple = total > 1;
  const current = images[index];

  // Zera o zoom ao trocar de imagem
  useEffect(() => {
    setZoom(MIN_ZOOM);
  }, [index]);

  const goPrev = useCallback(() => {
    setIndex((i) => (total > 0 ? (i - 1 + total) % total : 0));
  }, [total]);

  const goNext = useCallback(() => {
    setIndex((i) => (total > 0 ? (i + 1) % total : 0));
  }, [total]);

  const zoomIn = useCallback(() => setZoom((z) => clampZoom(z + ZOOM_STEP)), []);
  const zoomOut = useCallback(() => setZoom((z) => clampZoom(z - ZOOM_STEP)), []);
  const resetZoom = useCallback(() => setZoom(MIN_ZOOM), []);

  useEffect(() => {
    if (!didMountRef.current) {
      didMountRef.current = true;
      return;
    }
    announce(t('ui.imageViewer.zoomLevel', 'Zoom {{percent}}%', { percent: Math.round(zoom * 100) }));
  }, [announce, t, zoom]);

  // Navegação por setas e atalhos de zoom (ESC/Tab são tratados pelo Modal).
  // Só age quando este viewer é o modal do topo da stack, evitando controlar
  // um viewer que ficou "por trás" de outro modal aberto sobre ele.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!isTopmost()) return;
      if (e.key === 'ArrowLeft' && hasMultiple) {
        e.preventDefault();
        goPrev();
      } else if (e.key === 'ArrowRight' && hasMultiple) {
        e.preventDefault();
        goNext();
      } else if (e.key === '+' || e.key === '=') {
        e.preventDefault();
        zoomIn();
      } else if (e.key === '-' || e.key === '_') {
        e.preventDefault();
        zoomOut();
      } else if (e.key === '0') {
        e.preventDefault();
        resetZoom();
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [isTopmost, hasMultiple, goPrev, goNext, zoomIn, zoomOut, resetZoom]);

  // Zoom via scroll do mouse. Registrado manualmente como listener NÃO passivo
  // para que `preventDefault()` tenha efeito — listeners de `wheel` adicionados
  // pelo React (onWheel) são passivos, e o gesto rolaria o container scrollável
  // ao mesmo tempo em que aplica zoom. Aqui o scroll é cancelado durante o zoom.
  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      if (e.deltaY < 0) {
        zoomIn();
      } else {
        zoomOut();
      }
    };
    stage.addEventListener('wheel', onWheel, { passive: false });
    return () => stage.removeEventListener('wheel', onWheel);
  }, [zoomIn, zoomOut]);

  if (!current) return null;

  const altText = current.alt?.trim() || t('ui.imageViewer.defaultAlt');
  const caption = hasMultiple
    ? `${t('ui.imageViewer.counter', { current: index + 1, total })} — ${altText}`
    : altText;

  return (
    <div className="image-viewer">
      <div
        className="image-viewer__toolbar"
        role="toolbar"
        aria-label={t('ui.imageViewer.toolbarLabel')}
      >
        <button
          type="button"
          className="image-viewer__btn"
          onClick={zoomOut}
          disabled={zoom <= MIN_ZOOM}
          aria-label={t('ui.imageViewer.zoomOut')}
        >
          <ZoomOutOutlined aria-hidden="true" />
        </button>
        <span className="image-viewer__zoom-level">
          {Math.round(zoom * 100)}%
        </span>
        <button
          type="button"
          className="image-viewer__btn"
          onClick={zoomIn}
          disabled={zoom >= MAX_ZOOM}
          aria-label={t('ui.imageViewer.zoomIn')}
        >
          <ZoomInOutlined aria-hidden="true" />
        </button>
        <button
          type="button"
          className="image-viewer__btn"
          onClick={resetZoom}
          disabled={zoom === MIN_ZOOM}
          aria-label={t('ui.imageViewer.resetZoom')}
        >
          <CompressOutlined aria-hidden="true" />
        </button>
      </div>

      <div className="image-viewer__stage" ref={stageRef}>
        {hasMultiple && (
          <button
            type="button"
            className="image-viewer__nav image-viewer__nav--prev"
            onClick={goPrev}
            aria-label={t('ui.imageViewer.previous')}
          >
            <LeftOutlined aria-hidden="true" />
          </button>
        )}
        <img
          key={current.src}
          src={current.src}
          alt={altText}
          className="image-viewer__image"
          style={{ transform: `scale(${zoom})` }}
          draggable={false}
        />
        {hasMultiple && (
          <button
            type="button"
            className="image-viewer__nav image-viewer__nav--next"
            onClick={goNext}
            aria-label={t('ui.imageViewer.next')}
          >
            <RightOutlined aria-hidden="true" />
          </button>
        )}
      </div>

      <p id={captionId} className="image-viewer__caption">
        {caption}
      </p>
    </div>
  );
}

export function ImageViewerModal({
  isOpen,
  images,
  initialIndex = 0,
  onClose,
}: ImageViewerModalProps) {
  const { t } = useTranslation();
  const captionId = useId();

  if (!isOpen || images.length === 0) return null;

  const safeIndex = Math.min(Math.max(initialIndex, 0), images.length - 1);

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('ui.imageViewer.title')}
      size="xl"
      className="image-viewer-modal"
      ariaDescribedBy={captionId}
    >
      <ImageViewerView images={images} initialIndex={safeIndex} captionId={captionId} />
    </Modal>
  );
}

export default ImageViewerModal;
