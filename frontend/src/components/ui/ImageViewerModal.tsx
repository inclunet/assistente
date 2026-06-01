import { useCallback, useEffect, useId, useState } from 'react';
import {
  ZoomInOutlined,
  ZoomOutOutlined,
  CompressOutlined,
  LeftOutlined,
  RightOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { Modal } from './Modal';
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

export function ImageViewerModal({
  isOpen,
  images,
  initialIndex = 0,
  onClose,
}: ImageViewerModalProps) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(initialIndex);
  const [zoom, setZoom] = useState(MIN_ZOOM);
  const captionId = useId();

  const total = images.length;

  // Reposiciona no índice inicial e zera o zoom toda vez que o modal abre
  useEffect(() => {
    if (isOpen) {
      setIndex(total > 0 ? Math.min(Math.max(initialIndex, 0), total - 1) : 0);
      setZoom(MIN_ZOOM);
    }
  }, [isOpen, initialIndex, total]);

  // Zera o zoom ao trocar de imagem
  useEffect(() => {
    setZoom(MIN_ZOOM);
  }, [index]);

  const hasMultiple = total > 1;
  const current = images[index];

  const goPrev = useCallback(() => {
    setIndex((i) => (total > 0 ? (i - 1 + total) % total : 0));
  }, [total]);

  const goNext = useCallback(() => {
    setIndex((i) => (total > 0 ? (i + 1) % total : 0));
  }, [total]);

  const zoomIn = useCallback(() => setZoom((z) => clampZoom(z + ZOOM_STEP)), []);
  const zoomOut = useCallback(() => setZoom((z) => clampZoom(z - ZOOM_STEP)), []);
  const resetZoom = useCallback(() => setZoom(MIN_ZOOM), []);

  // Navegação por setas e atalhos de zoom (ESC/Tab são tratados pelo Modal)
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
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
  }, [isOpen, hasMultiple, goPrev, goNext, zoomIn, zoomOut, resetZoom]);

  const handleWheel = useCallback(
    (e: React.WheelEvent) => {
      if (e.deltaY < 0) {
        zoomIn();
      } else {
        zoomOut();
      }
    },
    [zoomIn, zoomOut],
  );

  if (!isOpen || !current) return null;

  const altText = current.alt?.trim() || t('ui.imageViewer.defaultAlt');
  const caption = hasMultiple
    ? `${t('ui.imageViewer.counter', { current: index + 1, total })} — ${altText}`
    : altText;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('ui.imageViewer.title')}
      size="xl"
      className="image-viewer-modal"
      ariaDescribedBy={captionId}
    >
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
          <span className="image-viewer__zoom-level" aria-live="polite">
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

        <div className="image-viewer__stage" onWheel={handleWheel}>
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
    </Modal>
  );
}

export default ImageViewerModal;
