/**
 * Media Detector - Detecção automática de tipo de mídia
 * 
 * Detecta o tipo de arquivo baseado em MIME type e extensão,
 * classificando em categorias para processamento apropriado.
 */

/**
 * Categorias de mídia suportadas
 */
export const MEDIA_CATEGORIES = {
  IMAGE: 'image',
  AUDIO: 'audio',
  DOCUMENT: 'document',
  DATA: 'data',
  VIDEO: 'video',
  UNKNOWN: 'unknown'
};

/**
 * Mapeamento de MIME types para categorias
 */
const MIME_TYPE_MAP = {
  // Imagens
  'image/jpeg': MEDIA_CATEGORIES.IMAGE,
  'image/jpg': MEDIA_CATEGORIES.IMAGE,
  'image/png': MEDIA_CATEGORIES.IMAGE,
  'image/gif': MEDIA_CATEGORIES.IMAGE,
  'image/webp': MEDIA_CATEGORIES.IMAGE,
  'image/svg+xml': MEDIA_CATEGORIES.IMAGE,
  'image/bmp': MEDIA_CATEGORIES.IMAGE,
  'image/tiff': MEDIA_CATEGORIES.IMAGE,
  'image/heic': MEDIA_CATEGORIES.IMAGE,
  'image/heif': MEDIA_CATEGORIES.IMAGE,
  
  // Áudio
  'audio/mpeg': MEDIA_CATEGORIES.AUDIO,
  'audio/mp3': MEDIA_CATEGORIES.AUDIO,
  'audio/wav': MEDIA_CATEGORIES.AUDIO,
  'audio/wave': MEDIA_CATEGORIES.AUDIO,
  'audio/x-wav': MEDIA_CATEGORIES.AUDIO,
  'audio/ogg': MEDIA_CATEGORIES.AUDIO,
  'audio/flac': MEDIA_CATEGORIES.AUDIO,
  'audio/aac': MEDIA_CATEGORIES.AUDIO,
  'audio/m4a': MEDIA_CATEGORIES.AUDIO,
  'audio/x-m4a': MEDIA_CATEGORIES.AUDIO,
  'audio/webm': MEDIA_CATEGORIES.AUDIO,
  'audio/opus': MEDIA_CATEGORIES.AUDIO,
  
  // Vídeo
  'video/mp4': MEDIA_CATEGORIES.VIDEO,
  'video/webm': MEDIA_CATEGORIES.VIDEO,
  'video/ogg': MEDIA_CATEGORIES.VIDEO,
  'video/quicktime': MEDIA_CATEGORIES.VIDEO,
  'video/x-msvideo': MEDIA_CATEGORIES.VIDEO,
  
  // Documentos
  'application/pdf': MEDIA_CATEGORIES.DOCUMENT,
  'application/msword': MEDIA_CATEGORIES.DOCUMENT,
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': MEDIA_CATEGORIES.DOCUMENT,
  'application/vnd.oasis.opendocument.text': MEDIA_CATEGORIES.DOCUMENT,
  'application/rtf': MEDIA_CATEGORIES.DOCUMENT,
  'text/plain': MEDIA_CATEGORIES.DOCUMENT,
  'text/markdown': MEDIA_CATEGORIES.DOCUMENT,
  'text/x-markdown': MEDIA_CATEGORIES.DOCUMENT,
  
  // Dados estruturados
  'application/json': MEDIA_CATEGORIES.DATA,
  'text/csv': MEDIA_CATEGORIES.DATA,
  'application/xml': MEDIA_CATEGORIES.DATA,
  'text/xml': MEDIA_CATEGORIES.DATA,
  'application/vnd.ms-excel': MEDIA_CATEGORIES.DATA,
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': MEDIA_CATEGORIES.DATA,
  'text/tab-separated-values': MEDIA_CATEGORIES.DATA,
  'application/x-yaml': MEDIA_CATEGORIES.DATA,
  'text/yaml': MEDIA_CATEGORIES.DATA,
};

/**
 * Mapeamento de extensões para categorias (fallback)
 */
const EXTENSION_MAP = {
  // Imagens
  '.jpg': MEDIA_CATEGORIES.IMAGE,
  '.jpeg': MEDIA_CATEGORIES.IMAGE,
  '.png': MEDIA_CATEGORIES.IMAGE,
  '.gif': MEDIA_CATEGORIES.IMAGE,
  '.webp': MEDIA_CATEGORIES.IMAGE,
  '.svg': MEDIA_CATEGORIES.IMAGE,
  '.bmp': MEDIA_CATEGORIES.IMAGE,
  '.tiff': MEDIA_CATEGORIES.IMAGE,
  '.heic': MEDIA_CATEGORIES.IMAGE,
  '.heif': MEDIA_CATEGORIES.IMAGE,
  
  // Áudio
  '.mp3': MEDIA_CATEGORIES.AUDIO,
  '.wav': MEDIA_CATEGORIES.AUDIO,
  '.ogg': MEDIA_CATEGORIES.AUDIO,
  '.flac': MEDIA_CATEGORIES.AUDIO,
  '.aac': MEDIA_CATEGORIES.AUDIO,
  '.m4a': MEDIA_CATEGORIES.AUDIO,
  '.wma': MEDIA_CATEGORIES.AUDIO,
  '.opus': MEDIA_CATEGORIES.AUDIO,
  '.webm': MEDIA_CATEGORIES.AUDIO, // Pode ser áudio ou vídeo
  
  // Vídeo
  '.mp4': MEDIA_CATEGORIES.VIDEO,
  '.avi': MEDIA_CATEGORIES.VIDEO,
  '.mov': MEDIA_CATEGORIES.VIDEO,
  '.mkv': MEDIA_CATEGORIES.VIDEO,
  '.wmv': MEDIA_CATEGORIES.VIDEO,
  
  // Documentos
  '.pdf': MEDIA_CATEGORIES.DOCUMENT,
  '.doc': MEDIA_CATEGORIES.DOCUMENT,
  '.docx': MEDIA_CATEGORIES.DOCUMENT,
  '.odt': MEDIA_CATEGORIES.DOCUMENT,
  '.rtf': MEDIA_CATEGORIES.DOCUMENT,
  '.txt': MEDIA_CATEGORIES.DOCUMENT,
  '.md': MEDIA_CATEGORIES.DOCUMENT,
  '.markdown': MEDIA_CATEGORIES.DOCUMENT,
  
  // Dados
  '.json': MEDIA_CATEGORIES.DATA,
  '.csv': MEDIA_CATEGORIES.DATA,
  '.xml': MEDIA_CATEGORIES.DATA,
  '.xls': MEDIA_CATEGORIES.DATA,
  '.xlsx': MEDIA_CATEGORIES.DATA,
  '.tsv': MEDIA_CATEGORIES.DATA,
  '.yaml': MEDIA_CATEGORIES.DATA,
  '.yml': MEDIA_CATEGORIES.DATA,
};

/**
 * Detecta a categoria de um arquivo
 * @param {File} file - Arquivo para detectar
 * @returns {Object} - { category, mimeType, extension, isSupported }
 */
export function detectMediaType(file) {
  if (!file) {
    return {
      category: MEDIA_CATEGORIES.UNKNOWN,
      mimeType: null,
      extension: null,
      isSupported: false,
      error: 'Arquivo não fornecido'
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
    if (mimeType.startsWith('image/')) category = MEDIA_CATEGORIES.IMAGE;
    else if (mimeType.startsWith('audio/')) category = MEDIA_CATEGORIES.AUDIO;
    else if (mimeType.startsWith('video/')) category = MEDIA_CATEGORIES.VIDEO;
    else if (mimeType.startsWith('text/')) category = MEDIA_CATEGORIES.DOCUMENT;
  }
  
  // Fallback para desconhecido
  if (!category) {
    category = MEDIA_CATEGORIES.UNKNOWN;
  }
  
  return {
    category,
    mimeType,
    extension,
    isSupported: category !== MEDIA_CATEGORIES.UNKNOWN,
    fileName: file.name,
    fileSize: file.size,
    fileSizeFormatted: formatFileSize(file.size)
  };
}

/**
 * Detecta tipos de múltiplos arquivos
 * @param {FileList|File[]} files - Lista de arquivos
 * @returns {Object[]} - Array de resultados de detecção
 */
export function detectMediaTypes(files) {
  return Array.from(files).map(file => detectMediaType(file));
}

/**
 * Agrupa arquivos por categoria
 * @param {FileList|File[]} files - Lista de arquivos
 * @returns {Object} - { image: [], audio: [], document: [], data: [], unknown: [] }
 */
export function groupByCategory(files) {
  const groups = {
    [MEDIA_CATEGORIES.IMAGE]: [],
    [MEDIA_CATEGORIES.AUDIO]: [],
    [MEDIA_CATEGORIES.VIDEO]: [],
    [MEDIA_CATEGORIES.DOCUMENT]: [],
    [MEDIA_CATEGORIES.DATA]: [],
    [MEDIA_CATEGORIES.UNKNOWN]: []
  };
  
  Array.from(files).forEach(file => {
    const detection = detectMediaType(file);
    groups[detection.category].push({
      file,
      ...detection
    });
  });
  
  return groups;
}

/**
 * Extrai extensão do nome do arquivo
 * @param {string} fileName 
 * @returns {string|null}
 */
function getExtension(fileName) {
  if (!fileName) return null;
  const lastDot = fileName.lastIndexOf('.');
  if (lastDot === -1) return null;
  return fileName.substring(lastDot).toLowerCase();
}

/**
 * Formata tamanho de arquivo para exibição
 * @param {number} bytes 
 * @returns {string}
 */
export function formatFileSize(bytes) {
  if (bytes === 0) return '0 B';
  
  const units = ['B', 'KB', 'MB', 'GB'];
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + units[i];
}

/**
 * Verifica se arquivo é uma imagem
 * @param {File} file 
 * @returns {boolean}
 */
export function isImage(file) {
  return detectMediaType(file).category === MEDIA_CATEGORIES.IMAGE;
}

/**
 * Verifica se arquivo é áudio
 * @param {File} file 
 * @returns {boolean}
 */
export function isAudio(file) {
  return detectMediaType(file).category === MEDIA_CATEGORIES.AUDIO;
}

/**
 * Verifica se arquivo é documento
 * @param {File} file 
 * @returns {boolean}
 */
export function isDocument(file) {
  return detectMediaType(file).category === MEDIA_CATEGORIES.DOCUMENT;
}

/**
 * Verifica se arquivo é dado estruturado
 * @param {File} file 
 * @returns {boolean}
 */
export function isData(file) {
  return detectMediaType(file).category === MEDIA_CATEGORIES.DATA;
}

/**
 * Verifica se arquivo é vídeo
 * @param {File} file 
 * @returns {boolean}
 */
export function isVideo(file) {
  return detectMediaType(file).category === MEDIA_CATEGORIES.VIDEO;
}

/**
 * Retorna ícone apropriado para a categoria
 * @param {string} category 
 * @returns {string}
 */
export function getCategoryIcon(category) {
  switch (category) {
    case MEDIA_CATEGORIES.IMAGE: return '🖼️';
    case MEDIA_CATEGORIES.AUDIO: return '🎵';
    case MEDIA_CATEGORIES.VIDEO: return '🎬';
    case MEDIA_CATEGORIES.DOCUMENT: return '📄';
    case MEDIA_CATEGORIES.DATA: return '📊';
    default: return '📁';
  }
}

/**
 * Retorna label apropriado para a categoria
 * @param {string} category 
 * @returns {string}
 */
export function getCategoryLabel(category) {
  switch (category) {
    case MEDIA_CATEGORIES.IMAGE: return 'Imagem';
    case MEDIA_CATEGORIES.AUDIO: return 'Áudio';
    case MEDIA_CATEGORIES.VIDEO: return 'Vídeo';
    case MEDIA_CATEGORIES.DOCUMENT: return 'Documento';
    case MEDIA_CATEGORIES.DATA: return 'Dados';
    default: return 'Arquivo';
  }
}

/**
 * Tipos MIME aceitos para input de arquivo (todos suportados)
 */
export const ALL_ACCEPTED_TYPES = [
  // Imagens
  'image/*',
  // Áudio
  'audio/*',
  // Vídeo
  'video/*',
  // Documentos
  'application/pdf',
  '.doc', '.docx', '.odt', '.rtf', '.txt', '.md',
  // Dados
  'application/json', '.csv', '.xml', '.xls', '.xlsx', '.yaml', '.yml'
].join(',');





