/**
 * Media Service - Detecção e processamento de arquivos de mídia para chat
 * 
 * Serviço agnóstico completo para:
 * - Detectar tipo de arquivo (MIME type e extensão)
 * - Processar arquivos (gerar preview, validar)
 * - Capturar mídia (screenshot, webcam)
 * 
 * Pode ser usado em qualquer aplicação que use o componente de chat.
 */

// ========================================
// Constantes e Mapeamentos
// ========================================

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

// ========================================
// Funções de Detecção
// ========================================

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
 * Detecta a categoria de um arquivo
 * @param {File} file - Arquivo para detectar
 * @returns {Object} - { category, mimeType, extension, isSupported, fileName, fileSize, fileSizeFormatted }
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

// ========================================
// Funções de Verificação de Tipo
// ========================================

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

// ========================================
// Funções de Preview e Conversão
// ========================================

/**
 * Cria preview base64 de uma imagem
 * @param {File} file - Arquivo de imagem
 * @returns {Promise<string>} - Data URL base64
 */
export function createImagePreview(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(new Error('Erro ao ler arquivo'));
    reader.readAsDataURL(file);
  });
}

/**
 * Converte um arquivo para base64 data URL
 * @param {File} file - Arquivo qualquer
 * @returns {Promise<string>} - Data URL base64
 */
export function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(new Error('Erro ao converter arquivo'));
    reader.readAsDataURL(file);
  });
}

/**
 * Cria URL de preview para áudio/vídeo
 * @param {File} file - Arquivo de áudio ou vídeo
 * @returns {string} - Object URL (lembre de revogar depois!)
 */
export function createMediaPreview(file) {
  return URL.createObjectURL(file);
}

/**
 * Revoga URLs de preview criadas com createMediaPreview
 * @param {string} url - URL para revogar
 */
export function revokeMediaPreview(url) {
  if (url && url.startsWith('blob:')) {
    URL.revokeObjectURL(url);
  }
}

// ========================================
// Processamento de Arquivos
// ========================================

/**
 * Processa um arquivo para ser adicionado ao chat
 * 
 * @param {File} file - Arquivo a processar
 * @param {Object} options - Opções de processamento
 * @param {string} options.source - Fonte do arquivo (paste, drop, screenshot, webcam)
 * @param {boolean} options.generatePreview - Se deve gerar preview (default: true)
 * @returns {Promise<ProcessedMedia>} - Mídia processada
 * 
 * @typedef {Object} ProcessedMedia
 * @property {File} file - Arquivo original
 * @property {string} type - Tipo/fonte (image, audio, screenshot, etc.)
 * @property {string} category - Categoria detectada
 * @property {string|null} preview - Preview base64 ou URL
 * @property {string} altText - Texto alternativo (nome do arquivo)
 * @property {string} icon - Ícone da categoria
 * @property {string} sizeFormatted - Tamanho formatado
 * @property {boolean} isSupported - Se o tipo é suportado
 * @property {string|null} error - Mensagem de erro, se houver
 */
export async function processMediaFile(file, options = {}) {
  const { source = null, generatePreview = true } = options;
  
  if (!file) {
    return {
      file: null,
      type: 'unknown',
      category: MEDIA_CATEGORIES.UNKNOWN,
      preview: null,
      altText: '',
      icon: getCategoryIcon(MEDIA_CATEGORIES.UNKNOWN),
      sizeFormatted: '0 B',
      isSupported: false,
      error: 'Arquivo não fornecido'
    };
  }
  
  // Detecta o tipo
  const detection = detectMediaType(file);
  const { category, isSupported, fileSizeFormatted } = detection;
  
  if (!isSupported) {
    return {
      file,
      type: source || category,
      category,
      preview: null,
      altText: file.name,
      icon: getCategoryIcon(category),
      sizeFormatted: fileSizeFormatted,
      isSupported: false,
      error: `Tipo de arquivo não suportado: ${file.name}`
    };
  }
  
  let preview = null;
  
  if (generatePreview) {
    try {
      if (category === MEDIA_CATEGORIES.IMAGE) {
        preview = await createImagePreview(file);
      } else if (category === MEDIA_CATEGORIES.AUDIO || category === MEDIA_CATEGORIES.VIDEO) {
        preview = createMediaPreview(file);
      }
      // Documentos e dados não têm preview
    } catch (err) {
      console.warn('Erro ao gerar preview:', err);
      // Continua sem preview
    }
  }
  
  return {
    file,
    type: source || category,
    category,
    preview,
    altText: file.name,
    icon: getCategoryIcon(category),
    sizeFormatted: fileSizeFormatted,
    isSupported: true,
    error: null
  };
}

/**
 * Processa múltiplos arquivos
 * 
 * @param {File[]} files - Array de arquivos
 * @param {Object} options - Opções de processamento
 * @returns {Promise<ProcessedMedia[]>} - Array de mídias processadas
 */
export async function processMediaFiles(files, options = {}) {
  const results = [];
  
  for (const file of files) {
    const processed = await processMediaFile(file, options);
    results.push(processed);
  }
  
  return results;
}

/**
 * Filtra apenas arquivos suportados
 * 
 * @param {ProcessedMedia[]} mediaList - Lista de mídias processadas
 * @returns {ProcessedMedia[]} - Apenas mídias suportadas
 */
export function filterSupported(mediaList) {
  return mediaList.filter(m => m.isSupported);
}

/**
 * Filtra apenas arquivos com erro
 * 
 * @param {ProcessedMedia[]} mediaList - Lista de mídias processadas
 * @returns {ProcessedMedia[]} - Apenas mídias com erro
 */
export function filterUnsupported(mediaList) {
  return mediaList.filter(m => !m.isSupported);
}

// ========================================
// Captura de Mídia
// ========================================

/**
 * Captura screenshot da tela
 * @returns {Promise<File>} - Arquivo de imagem PNG
 */
export async function captureScreen() {
  const stream = await navigator.mediaDevices.getDisplayMedia({
    video: { mediaSource: 'screen' }
  });
  
  const video = document.createElement('video');
  video.srcObject = stream;
  await video.play();
  await new Promise(resolve => setTimeout(resolve, 100));
  
  const canvas = document.createElement('canvas');
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0);
  
  stream.getTracks().forEach(track => track.stop());
  
  const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/png'));
  return new File([blob], `screenshot-${Date.now()}.png`, { type: 'image/png' });
}

/**
 * Captura foto da webcam
 * @param {Object} options - Opções de captura
 * @param {string} options.facingMode - Modo da câmera ('user' ou 'environment')
 * @param {number} options.delay - Delay antes de capturar (ms)
 * @param {number} options.quality - Qualidade JPEG (0-1)
 * @returns {Promise<File>} - Arquivo de imagem JPEG
 */
export async function captureWebcam(options = {}) {
  const { facingMode = 'user', delay = 500, quality = 0.9 } = options;
  
  const stream = await navigator.mediaDevices.getUserMedia({ 
    video: { facingMode } 
  });
  
  const video = document.createElement('video');
  video.srcObject = stream;
  await video.play();
  await new Promise(resolve => setTimeout(resolve, delay));
  
  const canvas = document.createElement('canvas');
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  canvas.getContext('2d').drawImage(video, 0, 0);
  
  stream.getTracks().forEach(track => track.stop());
  
  const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', quality));
  return new File([blob], `webcam-${Date.now()}.jpg`, { type: 'image/jpeg' });
}

/**
 * Verifica se o navegador suporta captura de tela
 * @returns {boolean}
 */
export function supportsScreenCapture() {
  return !!navigator.mediaDevices?.getDisplayMedia;
}

/**
 * Verifica se o navegador suporta webcam
 * @returns {boolean}
 */
export function supportsWebcam() {
  return !!navigator.mediaDevices?.getUserMedia;
}

// ========================================
// Operações de Clipboard e Download
// ========================================

/**
 * Copia uma imagem (base64 ou blob URL) para a área de transferência
 * @param {string} imageUrl - URL base64 ou blob da imagem
 * @returns {Promise<boolean>} - true se copiou com sucesso
 */
export async function copyImageToClipboard(imageUrl) {
  try {
    const response = await fetch(imageUrl);
    const blob = await response.blob();
    
    await navigator.clipboard.write([
      new ClipboardItem({ [blob.type]: blob })
    ]);
    
    return true;
  } catch (err) {
    console.error('Erro ao copiar imagem:', err);
    return false;
  }
}

/**
 * Faz download de uma imagem base64 como arquivo
 * @param {string} base64Url - URL base64 da imagem
 * @param {string} filename - Nome do arquivo para salvar
 * @returns {boolean} - true se iniciou o download
 */
export function downloadImage(base64Url, filename = 'imagem.png') {
  try {
    const link = document.createElement('a');
    link.href = base64Url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    return true;
  } catch (err) {
    console.error('Erro ao salvar imagem:', err);
    return false;
  }
}

/**
 * Copia texto para a área de transferência
 * @param {string} text - Texto para copiar
 * @returns {Promise<boolean>} - true se copiou com sucesso
 */
export async function copyTextToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (err) {
    console.error('Erro ao copiar texto:', err);
    return false;
  }
}

/**
 * Faz download de um blob como arquivo
 * @param {Blob} blob - Blob para download
 * @param {string} filename - Nome do arquivo
 */
export function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

/**
 * Converte base64 data URL para Blob
 * @param {string} base64Url - URL base64 (ex: "data:image/png;base64,...")
 * @returns {Promise<Blob>}
 */
export async function base64ToBlob(base64Url) {
  const response = await fetch(base64Url);
  return response.blob();
}
