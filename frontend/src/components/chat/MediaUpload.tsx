import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../ui/Button';
import './MediaUpload.css';

interface MediaFile {
  file: File;
  preview?: string;
  type: 'image' | 'audio' | 'document' | 'other';
}

interface MediaUploadProps {
  onFilesSelected: (files: MediaFile[]) => void;
  disabled?: boolean;
  maxFiles?: number;
  acceptedTypes?: string;
}

export const MediaUpload: React.FC<MediaUploadProps> = ({
  onFilesSelected,
  disabled = false,
  maxFiles = 5,
  acceptedTypes = 'image/*,audio/*,.pdf,.txt,.doc,.docx',
}) => {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [selectedFiles, setSelectedFiles] = useState<MediaFile[]>([]);

  const detectFileType = (file: File): MediaFile['type'] => {
    if (file.type.startsWith('image/')) return 'image';
    if (file.type.startsWith('audio/')) return 'audio';
    if (file.type === 'application/pdf' || file.type.startsWith('text/')) return 'document';
    return 'other';
  };

  const handleFileSelect = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files || []);
    if (files.length === 0) return;

    const mediaFiles: MediaFile[] = [];

    for (const file of files.slice(0, maxFiles)) {
      const type = detectFileType(file);
      const mediaFile: MediaFile = { file, type };

      // Generate preview for images
      if (type === 'image') {
        const reader = new FileReader();
        reader.onload = (e) => {
          mediaFile.preview = e.target?.result as string;
          setSelectedFiles((prev) => [...prev]);
        };
        reader.readAsDataURL(file);
      }

      mediaFiles.push(mediaFile);
    }

    setSelectedFiles(mediaFiles);
    onFilesSelected(mediaFiles);

    // Reset input
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const handleClick = () => {
    fileInputRef.current?.click();
  };

  const removeFile = (index: number) => {
    const newFiles = selectedFiles.filter((_, i) => i !== index);
    setSelectedFiles(newFiles);
    onFilesSelected(newFiles);
  };

  const clearAll = () => {
    setSelectedFiles([]);
    onFilesSelected([]);
  };

  return (
    <div className="media-upload">
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept={acceptedTypes}
        onChange={handleFileSelect}
        className="media-upload__input"
        aria-label={t('mediaUpload.selectFiles')}
        disabled={disabled}
      />

      <Button
        onClick={handleClick}
        disabled={disabled || selectedFiles.length >= maxFiles}
        variant="secondary"
        aria-label={t('mediaUpload.attachLabel')}
        title={t('mediaUpload.attachTooltip')}
      >
        {t('mediaUpload.attachBtn')}
      </Button>

      {selectedFiles.length > 0 && (
        <div className="media-upload__preview" role="region" aria-label={t('mediaUpload.filesSelected')}>
          <div className="media-upload__preview-header">
            <span>{selectedFiles.length} {t('mediaUpload.filesCount')}</span>
            <Button
              onClick={clearAll}
              variant="ghost"
              size="sm"
              aria-label={t('mediaUpload.clearAll')}
            >
              {t('mediaUpload.clearBtn')}
            </Button>
          </div>

          <div className="media-upload__preview-list">
            {selectedFiles.map((mediaFile, index) => (
              <div key={index} className="media-upload__preview-item">
                {mediaFile.type === 'image' && mediaFile.preview ? (
                  <img
                    src={mediaFile.preview}
                    alt={mediaFile.file.name}
                    className="media-upload__preview-image"
                  />
                ) : (
                  <div className="media-upload__preview-icon">
                    {mediaFile.type === 'audio' ? '🎵' : '📄'}
                  </div>
                )}

                <div className="media-upload__preview-info">
                  <span className="media-upload__preview-name">{mediaFile.file.name}</span>
                  <span className="media-upload__preview-size">
                    {(mediaFile.file.size / 1024).toFixed(2)} KB
                  </span>
                </div>

                <Button
                  onClick={() => removeFile(index)}
                  variant="ghost"
                  size="sm"
                  aria-label={`${t('mediaUpload.remove')} ${mediaFile.file.name}`}
                >
                  ✕
                </Button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
