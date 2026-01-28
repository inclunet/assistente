import React, { useEffect, useRef } from 'react';
import { Modal } from './Modal';
import { MarkdownRenderer } from './MarkdownRenderer';
import './MessageDetailModal.css';

export interface MediaItem {
  type: string;
  preview?: string;
  file?: { name: string };
  altText?: string;
}

export interface MessageDetailModalProps {
  open: boolean;
  content: string;
  role?: string;
  media?: MediaItem[];
  onClose: () => void;
  onImageClick?: (src: string, alt: string) => void;
}

export const MessageDetailModal: React.FC<MessageDetailModalProps> = ({
  open,
  content,
  role = 'Mensagem',
  media = [],
  onClose,
  onImageClick,
}) => {
  const contentRef = useRef<HTMLDivElement>(null);

  // Foca no conteúdo quando abre
  useEffect(() => {
    if (open && contentRef.current) {
      contentRef.current.focus();
    }
  }, [open]);

  if (!open) return null;

  const renderMedia = () => {
    // Garante que media seja um array válido
    if (!media || !Array.isArray(media) || media.length === 0) return null;

    return (
      <div className="message-detail-modal__media">
        {media.map((item, index) => {
          if (item.type === 'image' || item.type === 'screenshot' || item.type === 'webcam') {
            const imageDesc = item.altText || item.file?.name || 'Imagem';
            return (
              <figure key={index} className="message-detail-modal__image">
                <img src={item.preview} alt={imageDesc} />
                <figcaption>
                  <button
                    className="message-detail-modal__zoom-btn"
                    onClick={() => onImageClick?.(item.preview!, imageDesc)}
                    aria-label={`Ampliar imagem: ${imageDesc}`}
                  >
                    🔍 Ampliar
                  </button>
                  <span className="message-detail-modal__image-desc">{imageDesc}</span>
                </figcaption>
              </figure>
            );
          }

          if (item.type === 'audio') {
            return (
              <div key={index} className="message-detail-modal__audio">
                <span aria-hidden="true">🎵</span>
                <span>{item.file?.name || 'Áudio'}</span>
              </div>
            );
          }

          if (item.type === 'document') {
            return (
              <div key={index} className="message-detail-modal__document">
                <span aria-hidden="true">📄</span>
                <span>{item.file?.name || 'Documento'}</span>
              </div>
            );
          }

          return null;
        })}
      </div>
    );
  };

  return (
    <Modal id="message-detail" title={role} onClose={onClose}>
      <div
        ref={contentRef}
        className="message-detail-modal__content"
        tabIndex={0}
        aria-label="Conteúdo da mensagem. Use as setas para navegar."
      >
        {renderMedia()}
        {content && (
          <div className="message-detail-modal__text">
            <MarkdownRenderer content={content} />
          </div>
        )}
      </div>
    </Modal>
  );
};


