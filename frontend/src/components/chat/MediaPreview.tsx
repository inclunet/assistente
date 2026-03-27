import React from 'react';
import { CloseOutlined, LoadingOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { MediaFile, MediaCategory } from '../../services/mediaService';
import './MediaPreview.css';

interface MediaPreviewProps {
  media: MediaFile[];
  onRemove: (id: string) => void;
}

export const MediaPreview: React.FC<MediaPreviewProps> = ({ media, onRemove }) => {
  const { t } = useTranslation();
  if (media.length === 0) return null;

  return (
    <div className="pending-media" role="list" aria-label={t('mediaPreview.label')}>
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
                <span className="alt-generating" aria-label={t('mediaPreview.loading')}><LoadingOutlined spin /></span>
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
                {t('mediaPreview.audioUnsupported')}
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
                <>{t('mediaPreview.loadingFancy')}</>
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
            aria-label={`${t('mediaPreview.remove')} ${item.altText || item.fileName}`}
          >
            <CloseOutlined aria-hidden="true" />
          </button>
        </div>
      ))}
    </div>
  );
};
