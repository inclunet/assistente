/**
 * Chat Components - Utility Functions
 */

/**
 * Gera um ID único para mensagem
 * @returns {string}
 */
export function generateMessageId() {
  return `msg_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
}

/**
 * Formata timestamp para exibição
 * @param {Date|string|number} timestamp
 * @param {Object} labels - Labels de i18n
 * @returns {string}
 */
export function formatTimestamp(timestamp, labels = {}) {
  if (!timestamp) return '';
  
  const date = timestamp instanceof Date ? timestamp : new Date(timestamp);
  const now = new Date();
  const diffMs = now - date;
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);
  
  if (diffMins < 1) {
    return labels.justNow || 'Agora';
  }
  if (diffMins < 60) {
    const template = labels.minutesAgo || '{n} min atrás';
    return template.replace('{n}', diffMins);
  }
  if (diffHours < 24) {
    const template = labels.hoursAgo || '{n}h atrás';
    return template.replace('{n}', diffHours);
  }
  if (diffDays === 1) {
    return labels.yesterday || 'Ontem';
  }
  
  // Formato completo para datas mais antigas
  return date.toLocaleDateString('pt-BR', {
    day: '2-digit',
    month: '2-digit',
    year: diffDays > 365 ? 'numeric' : undefined,
    hour: '2-digit',
    minute: '2-digit'
  });
}

/**
 * Trunca texto com reticências
 * @param {string} text
 * @param {number} maxLength
 * @returns {string}
 */
export function truncateText(text, maxLength) {
  if (!text || text.length <= maxLength) return text;
  return text.substring(0, maxLength) + '...';
}

/**
 * Verifica se mídia é do tipo imagem
 * @param {Object} media
 * @returns {boolean}
 */
export function isImageMedia(media) {
  if (!media) return false;
  const type = media.type?.toLowerCase();
  return type === 'image' || type === 'screenshot' || type === 'webcam';
}

/**
 * Verifica se mídia é do tipo áudio
 * @param {Object} media
 * @returns {boolean}
 */
export function isAudioMedia(media) {
  if (!media) return false;
  return media.type?.toLowerCase() === 'audio';
}

/**
 * Verifica se mídia é do tipo documento
 * @param {Object} media
 * @returns {boolean}
 */
export function isDocumentMedia(media) {
  if (!media) return false;
  return media.type?.toLowerCase() === 'document';
}

/**
 * Formata nome do autor baseado no role
 * @param {Object} author
 * @param {Object} labels
 * @returns {string}
 */
export function formatAuthorName(author, labels = {}) {
  if (!author) return labels.system || 'Sistema';
  
  if (author.name) return author.name;
  
  const roleLabels = {
    user: labels.you || 'Você',
    assistant: labels.assistant || 'Assistente',
    agent: labels.agent || 'Agente',
    tool: labels.tool || 'Ferramenta',
    system: labels.system || 'Sistema'
  };
  
  return roleLabels[author.role] || author.role || labels.system || 'Sistema';
}

/**
 * Formata nome de agente (snake_case para Title Case)
 * @param {string} name
 * @returns {string}
 */
export function formatAgentName(name) {
  if (!name) return 'Agente';
  return name
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * Extrai texto puro de conteúdo markdown
 * @param {string} markdown
 * @returns {string}
 */
export function stripMarkdown(markdown) {
  if (!markdown) return '';
  return markdown
    // Remove headers
    .replace(/^#{1,6}\s+/gm, '')
    // Remove bold/italic
    .replace(/\*\*([^*]+)\*\*/g, '$1')
    .replace(/\*([^*]+)\*/g, '$1')
    .replace(/__([^_]+)__/g, '$1')
    .replace(/_([^_]+)_/g, '$1')
    // Remove links
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
    // Remove images
    .replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
    // Remove code blocks
    .replace(/```[\s\S]*?```/g, '')
    // Remove inline code
    .replace(/`([^`]+)`/g, '$1')
    // Remove blockquotes
    .replace(/^>\s+/gm, '')
    // Remove horizontal rules
    .replace(/^[-*_]{3,}\s*$/gm, '')
    // Normalize whitespace
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

/**
 * Debounce simples
 * @param {Function} fn
 * @param {number} delay
 * @returns {Function}
 */
export function debounce(fn, delay) {
  let timeoutId;
  return function (...args) {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn.apply(this, args), delay);
  };
}

/**
 * Copia texto para clipboard
 * @param {string} text
 * @returns {Promise<boolean>}
 */
export async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (err) {
    // Fallback para navegadores mais antigos
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    try {
      document.execCommand('copy');
      return true;
    } catch (e) {
      return false;
    } finally {
      document.body.removeChild(textarea);
    }
  }
}

/**
 * Download de arquivo a partir de base64
 * @param {string} base64Data
 * @param {string} filename
 * @param {string} mimeType
 */
export function downloadBase64(base64Data, filename, mimeType = 'image/png') {
  const link = document.createElement('a');
  link.href = `data:${mimeType};base64,${base64Data}`;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}

/**
 * Converte blob para base64
 * @param {Blob} blob
 * @returns {Promise<string>}
 */
export function blobToBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result;
      // Remove o prefixo data:...;base64,
      const base64 = result.split(',')[1] || result;
      resolve(base64);
    };
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

