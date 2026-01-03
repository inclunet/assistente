/**
 * Media Service - Processamento de arquivos de mídia para chat
 * 
 * Serviço agnóstico para processar arquivos antes de adicionar ao chat.
 * Pode ser usado em qualquer aplicação que use o componente de chat.
 */

import { 
  detectMediaType, 
  MEDIA_CATEGORIES, 
  getCategoryIcon,
  getCategoryLabel 
} from '../media-detector.js';

// Re-exporta para conveniência
export { MEDIA_CATEGORIES, getCategoryIcon, getCategoryLabel };

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

