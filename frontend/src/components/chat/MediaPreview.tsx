import React from 'react';
import { MediaFile, MediaCategory } from '../../services/mediaService';
import './MediaPreview.css';

interface MediaPreviewProps {
  media: MediaFile[];
  onRemove: (id: string) => void;
}

export const MediaPreview: React.FC<MediaPreviewProps> = ({ media, onRemove }) => {
  if (media.length === 0) return null;

  return (
    <div className="pending-media" role="list" aria-label="Arquivos anexados">
      {media.map((item) => (
        <div 
          key={item.id} 
          className="media-preview" 
          role="listitem" 
          data-category={item.category}
        >
          {/* Imagem: thumbnail com indicador de geração de alt */}
          {item.category === MediaCategory.IMAGE && item.preview && (
            <div className="media-thumbnail-wrapper">
              <img
                src={item.preview}
                alt={item.altText || item.fileName}
                className="media-thumbnail"
                title={item.altText || item.fileName}
              />
              {item.generatingAlt && (
                <span className="alt-generating" aria-label="Carregando">✨</span>
              )}
            </div>
          )}

          {/* Áudio: mini player */}
          {item.category === MediaCategory.AUDIO && item.preview && (
            <div className="media-audio-preview">
              <span className="media-icon" aria-hidden="true">{item.icon}</span>
              <audio
                src={item.preview}
                controls
                className="audio-mini-player"
                title={item.fileName}
              >
                Seu navegador não suporta áudio.
              </audio>
            </div>
          )}

          {/* Outros: ícone baseado na categoria */}
          {item.category !== MediaCategory.IMAGE && item.category !== MediaCategory.AUDIO && (
            <span className="media-icon" aria-hidden="true">
              {item.icon}
            </span>
          )}

          {/* Nome e info do arquivo */}
          <div className="media-info">
            <span className="media-name" title={item.altText || item.fileName}>
              {item.generatingAlt ? (
                <>✨ Carregando...</>
              ) : item.altText && item.altText !== item.fileName ? (
                <>
                  {item.altText.substring(0, 40)}
                  {item.altText.length > 40 ? '...' : ''}
                </>
              ) : (
                item.fileName
              )}
            </span>
            {item.fileSizeFormatted && (
              <span className="media-size">{item.fileSizeFormatted}</span>
            )}
          </div>

          <button
            type="button"
            className="media-remove"
            onClick={() => onRemove(item.id)}
            aria-label={`Remover ${item.altText || item.fileName}`}
            title="Remover arquivo"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
};
