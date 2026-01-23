/**
 * Media Service - Detecção e processamento de arquivos de mídia para chat
 * 
 * Serviço completo para:
 * - Detectar tipo de arquivo (MIME type e extensão)
 * - Processar arquivos (gerar preview, validar)
 * - Formatar informações de arquivo
 */

// ========================================
// Tipos e Constantes
// ========================================

export enum MediaCategory {
  IMAGE = 'image',
  AUDIO = 'audio',
  VIDEO = 'video',
  DOCUMENT = 'document',
  DATA = 'data',
  UNKNOWN = 'unknown',
}

export interface MediaFile {
  id: string;
  file: File;
  category: MediaCategory;
  mimeType: string;
  extension: string | null;
  fileName: string;
  fileSize: number;
  fileSizeFormatted: string;
  preview?: string;
  altText?: string;
  generatingAlt?: boolean;
  icon: string;
}

export interface MediaDetection {
  category: MediaCategory;
  mimeType: string;
  extension: string | null;
  isSupported: boolean;
  fileName: string;
  fileSize: number;
  fileSizeFormatted: string;
}

// ========================================
// Mapeamentos
// ========================================

const MIME_TYPE_MAP: Record<string, MediaCategory> = {
  // Imagens
  'image/jpeg': MediaCategory.IMAGE,
  'image/jpg': MediaCategory.IMAGE,
  'image/png': MediaCategory.IMAGE,
  'image/gif': MediaCategory.IMAGE,
  'image/webp': MediaCategory.IMAGE,
  'image/svg+xml': MediaCategory.IMAGE,
  'image/bmp': MediaCategory.IMAGE,
  'image/tiff': MediaCategory.IMAGE,
  'image/heic': MediaCategory.IMAGE,
  'image/heif': MediaCategory.IMAGE,
  
  // Áudio
  'audio/mpeg': MediaCategory.AUDIO,
  'audio/mp3': MediaCategory.AUDIO,
  'audio/wav': MediaCategory.AUDIO,
  'audio/wave': MediaCategory.AUDIO,
  'audio/x-wav': MediaCategory.AUDIO,
  'audio/ogg': MediaCategory.AUDIO,
  'audio/flac': MediaCategory.AUDIO,
  'audio/aac': MediaCategory.AUDIO,
  'audio/m4a': MediaCategory.AUDIO,
  'audio/x-m4a': MediaCategory.AUDIO,
  'audio/webm': MediaCategory.AUDIO,
  'audio/opus': MediaCategory.AUDIO,
  
  // Vídeo
  'video/mp4': MediaCategory.VIDEO,
  'video/webm': MediaCategory.VIDEO,
  'video/ogg': MediaCategory.VIDEO,
  'video/quicktime': MediaCategory.VIDEO,
  'video/x-msvideo': MediaCategory.VIDEO,
  
  // Documentos
  'application/pdf': MediaCategory.DOCUMENT,
  'application/msword': MediaCategory.DOCUMENT,
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': MediaCategory.DOCUMENT,
  'application/vnd.oasis.opendocument.text': MediaCategory.DOCUMENT,
  'application/rtf': MediaCategory.DOCUMENT,
  'text/plain': MediaCategory.DOCUMENT,
  'text/markdown': MediaCategory.DOCUMENT,
  'text/x-markdown': MediaCategory.DOCUMENT,
  
  // Dados estruturados
  'application/json': MediaCategory.DATA,
  'text/csv': MediaCategory.DATA,
  'application/xml': MediaCategory.DATA,
  'text/xml': MediaCategory.DATA,
  'application/vnd.ms-excel': MediaCategory.DATA,
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': MediaCategory.DATA,
  'text/tab-separated-values': MediaCategory.DATA,
  'application/x-yaml': MediaCategory.DATA,
  'text/yaml': MediaCategory.DATA,
};

const EXTENSION_MAP: Record<string, MediaCategory> = {
  // Imagens
  '.jpg': MediaCategory.IMAGE,
  '.jpeg': MediaCategory.IMAGE,
  '.png': MediaCategory.IMAGE,
  '.gif': MediaCategory.IMAGE,
  '.webp': MediaCategory.IMAGE,
  '.svg': MediaCategory.IMAGE,
  '.bmp': MediaCategory.IMAGE,
  '.tiff': MediaCategory.IMAGE,
  '.heic': MediaCategory.IMAGE,
  '.heif': MediaCategory.IMAGE,
  
  // Áudio
  '.mp3': MediaCategory.AUDIO,
  '.wav': MediaCategory.AUDIO,
  '.ogg': MediaCategory.AUDIO,
  '.flac': MediaCategory.AUDIO,
  '.aac': MediaCategory.AUDIO,
  '.m4a': MediaCategory.AUDIO,
  '.wma': MediaCategory.AUDIO,
  '.opus': MediaCategory.AUDIO,
  '.webm': MediaCategory.AUDIO,
  
  // Vídeo
  '.mp4': MediaCategory.VIDEO,
  '.avi': MediaCategory.VIDEO,
  '.mov': MediaCategory.VIDEO,
  '.mkv': MediaCategory.VIDEO,
  '.wmv': MediaCategory.VIDEO,
  
  // Documentos
  '.pdf': MediaCategory.DOCUMENT,
  '.doc': MediaCategory.DOCUMENT,
  '.docx': MediaCategory.DOCUMENT,
  '.odt': MediaCategory.DOCUMENT,
  '.rtf': MediaCategory.DOCUMENT,
  '.txt': MediaCategory.DOCUMENT,
  '.md': MediaCategory.DOCUMENT,
  '.markdown': MediaCategory.DOCUMENT,
  
  // Dados
  '.json': MediaCategory.DATA,
  '.csv': MediaCategory.DATA,
  '.xml': MediaCategory.DATA,
  '.xls': MediaCategory.DATA,
  '.xlsx': MediaCategory.DATA,
  '.tsv': MediaCategory.DATA,
  '.yaml': MediaCategory.DATA,
  '.yml': MediaCategory.DATA,
};

const CATEGORY_ICONS: Record<MediaCategory, string> = {
  [MediaCategory.IMAGE]: '🖼️',
  [MediaCategory.AUDIO]: '🎵',
  [MediaCategory.VIDEO]: '🎬',
  [MediaCategory.DOCUMENT]: '📄',
  [MediaCategory.DATA]: '📊',
  [MediaCategory.UNKNOWN]: '📎',
};

export const ALL_ACCEPTED_TYPES = [
  'image/*',
  'audio/*',
  'video/*',
  'application/pdf',
  '.doc', '.docx', '.odt', '.rtf', '.txt', '.md',
  'application/json', '.csv', '.xml', '.xls', '.xlsx', '.yaml', '.yml'
].join(',');

// ========================================
// Funções Utilitárias
// ========================================

function getExtension(fileName: string): string | null {
  if (!fileName) return null;
  const lastDot = fileName.lastIndexOf('.');
  if (lastDot === -1) return null;
  return fileName.substring(lastDot).toLowerCase();
}

export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  
  const units = ['B', 'KB', 'MB', 'GB'];
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + units[i];
}

function getCategoryIcon(category: MediaCategory): string {
  return CATEGORY_ICONS[category] || '📎';
}

// ========================================
// Funções de Detecção
// ========================================

export function detectMediaType(file: File): MediaDetection {
  if (!file) {
    return {
      category: MediaCategory.UNKNOWN,
      mimeType: '',
      extension: null,
      isSupported: false,
      fileName: '',
      fileSize: 0,
      fileSizeFormatted: '0 B',
    };
  }

  const mimeType = file.type?.toLowerCase() || '';
  const extension = getExtension(file.name);
  
  // Tenta detectar por MIME type primeiro
  let category = MIME_TYPE_MAP[mimeType];
  
  // Se não encontrou, tenta por extensão
  if (!category && extension) {
    category = EXTENSION_MAP[extension];
  }
  
  // Se ainda não encontrou, tenta por prefixo do MIME
  if (!category) {
    if (mimeType.startsWith('image/')) category = MediaCategory.IMAGE;
    else if (mimeType.startsWith('audio/')) category = MediaCategory.AUDIO;
    else if (mimeType.startsWith('video/')) category = MediaCategory.VIDEO;
    else if (mimeType.startsWith('text/')) category = MediaCategory.DOCUMENT;
  }
  
  // Fallback para desconhecido
  if (!category) {
    category = MediaCategory.UNKNOWN;
  }
  
  return {
    category,
    mimeType,
    extension,
    isSupported: category !== MediaCategory.UNKNOWN,
    fileName: file.name,
    fileSize: file.size,
    fileSizeFormatted: formatFileSize(file.size),
  };
}

// ========================================
// Funções de Processamento
// ========================================

export async function createMediaPreview(file: File, category: MediaCategory): Promise<string | undefined> {
  if (category === MediaCategory.IMAGE) {
    return new Promise((resolve) => {
      const reader = new FileReader();
      reader.onload = (e) => resolve(e.target?.result as string);
      reader.onerror = () => resolve(undefined);
      reader.readAsDataURL(file);
    });
  }
  
  if (category === MediaCategory.AUDIO || category === MediaCategory.VIDEO) {
    return URL.createObjectURL(file);
  }
  
  return undefined;
}

export async function processMediaFile(file: File): Promise<MediaFile> {
  const detection = detectMediaType(file);
  const preview = await createMediaPreview(file, detection.category);
  
  return {
    id: `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
    file,
    category: detection.category,
    mimeType: detection.mimeType,
    extension: detection.extension,
    fileName: detection.fileName,
    fileSize: detection.fileSize,
    fileSizeFormatted: detection.fileSizeFormatted,
    preview,
    altText: detection.fileName,
    generatingAlt: false,
    icon: getCategoryIcon(detection.category),
  };
}

export async function processMediaFiles(files: File[]): Promise<MediaFile[]> {
  return Promise.all(files.map(file => processMediaFile(file)));
}

// ========================================
// Funções de Verificação
// ========================================

export function isImage(file: File): boolean {
  return detectMediaType(file).category === MediaCategory.IMAGE;
}

export function isAudio(file: File): boolean {
  return detectMediaType(file).category === MediaCategory.AUDIO;
}

export function isDocument(file: File): boolean {
  return detectMediaType(file).category === MediaCategory.DOCUMENT;
}
