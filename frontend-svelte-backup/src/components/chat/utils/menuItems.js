/**
 * @typedef {Object} ExtraMenuItem
 * @property {string} id - Identificador único
 * @property {string} label - Texto exibido
 * @property {string} [icon] - Emoji ou ícone
 * @property {string} [shortcut] - Atalho de teclado
 * @property {string} [event] - Nome do evento a disparar (ex: 'speak', 'customAction')
 * @property {Function} [action] - Função a executar (alternativa a event)
 * @property {boolean} [danger] - Estilo de ação perigosa
 * @property {string} [position] - 'start' | 'afterCopy' | 'beforeDelete' | 'end' (default: 'end')
 * @property {Function} [condition] - (message, options) => boolean - Se deve mostrar
 */

/**
 * Gera itens de menu de contexto para uma mensagem.
 * 
 * @param {Object} message - A mensagem
 * @param {Object} options - Opções de configuração
 * @param {number} options.index - Índice da mensagem
 * @param {number} options.level - Nível de profundidade (0 = raiz)
 * @param {Object} options.config - Configuração de quais ações mostrar
 * @param {Object} options.handlers - Handlers para ações internas
 * @param {Function} options.t - Função de tradução (ex: $_)
 * @param {ExtraMenuItem[]} options.extraItems - Itens extras injetados
 * @returns {Array} Array de itens de menu
 * 
 * @example
 * // Injetando opção de TTS
 * getMessageMenuItems(message, {
 *   extraItems: [
 *     {
 *       id: 'speak',
 *       label: 'Ouvir mensagem',
 *       icon: '🔊',
 *       shortcut: 'Space',
 *       event: 'speak', // Dispara evento com { message, content }
 *       position: 'afterCopy',
 *       condition: (msg, opts) => !!msg.content && !opts.config.ttsDisabled
 *     }
 *   ],
 *   handlers: {
 *     onEvent: (eventName, payload) => {
 *       // Trata eventos customizados
 *       if (eventName === 'speak') synthesize(payload.content);
 *     }
 *   }
 * });
 */
export function getMessageMenuItems(message, options = {}) {
  const {
    index = -1,
    level = 0,
    config = {},
    handlers = {},
    extraItems = [],
    t = (key) => key,
  } = options;
  
  const {
    showCopy = true,
    showCopyMarkdown = true,
    showFullscreen = true,
    showEdit = true,
    showDelete = true,
    showPin = false,
    showResend = false,
  } = config;
  
  if (!message) return [];
  
  const items = [];
  const isUser = message.role === 'user';
  const isPinned = message.pinned || message.isPinned;
  const hasContent = !!message.content;
  
  // Helper para criar action de item extra
  const createExtraAction = (extraItem) => {
    return () => {
      if (extraItem.action) {
        extraItem.action({ message, index, level, content: message.content });
      } else if (extraItem.event && handlers.onEvent) {
        handlers.onEvent(extraItem.event, { 
          message, 
          index, 
          level, 
          content: message.content,
          itemId: extraItem.id
        });
      }
    };
  };
  
  // Helper para filtrar e adicionar extras por posição
  const addExtrasAtPosition = (position) => {
    const extras = extraItems.filter(item => {
      const pos = item.position || 'end';
      if (pos !== position) return false;
      if (item.condition && !item.condition(message, options)) return false;
      return true;
    });
    
    extras.forEach(extra => {
      items.push({
        id: extra.id,
        label: extra.label,
        icon: extra.icon,
        shortcut: extra.shortcut,
        danger: extra.danger,
        submenu: extra.submenu,
        action: createExtraAction(extra)
      });
    });
  };
  
  // === Extras no início ===
  addExtrasAtPosition('start');
  
  // =====================
  // Ações de Cópia
  // =====================
  
  if (showCopy && hasContent) {
    items.push({
      id: 'copy',
      label: t('chat.copy'),
      icon: '📋',
      shortcut: 'Ctrl+C',
      action: () => {
        navigator.clipboard.writeText(message.content);
        handlers.onCopied?.({ message, format: 'text' });
        handlers.onAnnounce?.(t('chat.copied'));
      }
    });
  }
  
  if (showCopyMarkdown && hasContent) {
    items.push({
      id: 'copyMarkdown',
      label: t('chat.copyMarkdown'),
      icon: '📝',
      action: () => {
        navigator.clipboard.writeText(message.content);
        handlers.onCopied?.({ message, format: 'markdown' });
        handlers.onAnnounce?.(t('chat.copied'));
      }
    });
  }
  
  // Ver em tela cheia / detalhes
  if (showFullscreen) {
    items.push({
      id: 'fullscreen',
      label: t('chat.viewFullSize'),
      icon: '🔍',
      shortcut: 'Enter',
      action: () => {
        handlers.onDetail?.({ message, index });
      }
    });
  }
  
  // === Extras após cópias (ex: TTS) ===
  addExtrasAtPosition('afterCopy');
  
  // Separador após cópias
  if (items.length > 0) {
    items.push({ separator: true });
  }
  
  // =====================
  // Ações de Edição
  // =====================
  
  if (showEdit && isUser && level === 0) {
    items.push({
      id: 'edit',
      label: t('chat.edit'),
      icon: '✏️',
      shortcut: 'E',
      action: () => {
        handlers.onEditStart?.({ message, index });
      }
    });
  }
  
  // =====================
  // Ações de Gerenciamento
  // =====================
  
  // === Extras antes de ações de gerenciamento ===
  addExtrasAtPosition('beforeManagement');
  
  const hasManagementActions = showPin || showResend || showDelete;
  if (hasManagementActions && items.length > 0) {
    items.push({ separator: true });
  }
  
  if (showPin) {
    items.push({
      id: 'pin',
      label: isPinned ? t('chat.unpin') : t('chat.pin'),
      icon: isPinned ? '📌' : '📍',
      action: () => {
        handlers.onPin?.({ message, pinned: !isPinned });
      }
    });
  }
  
  if (showResend && isUser) {
    items.push({
      id: 'resend',
      label: t('chat.resend'),
      icon: '🔄',
      action: () => {
        handlers.onResend?.({ message });
      }
    });
  }
  
  // === Extras antes do delete ===
  addExtrasAtPosition('beforeDelete');
  
  if (showDelete && level === 0) {
    items.push({
      id: 'delete',
      label: t('chat.delete'),
      icon: '🗑️',
      danger: true,
      action: () => {
        handlers.onDelete?.({ message, index });
      }
    });
  }
  
  // === Extras no final ===
  addExtrasAtPosition('end');
  
  return items;
}

/**
 * Gera itens de menu para código dentro de uma mensagem.
 * 
 * @param {string} code - O código
 * @param {string} language - Linguagem do código
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Array} Array de itens de menu
 */
export function getCodeMenuItems(code, language, handlers = {}, t = (key) => key) {
  return [
    {
      id: 'copyCode',
      label: t('chat.copyCode'),
      icon: '📋',
      action: () => {
        navigator.clipboard.writeText(code);
        handlers.onAnnounce?.(t('chat.copied'));
      }
    }
  ];
}

/**
 * Gera itens de menu para tabela dentro de uma mensagem.
 * 
 * @param {Object} tableData - Dados da tabela
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Array} Array de itens de menu
 */
export function getTableMenuItems(tableData, handlers = {}, t = (key) => key) {
  return [
    {
      id: 'copyTable',
      label: 'Copiar tabela',
      icon: '📋',
      submenu: [
        {
          id: 'copyTableText',
          label: 'Como texto',
          action: () => {
            handlers.onCopyTable?.({ table: tableData, format: 'text' });
          }
        },
        {
          id: 'copyTableCSV',
          label: 'Como CSV',
          action: () => {
            handlers.onCopyTable?.({ table: tableData, format: 'csv' });
          }
        },
        {
          id: 'copyTableMarkdown',
          label: 'Como Markdown',
          action: () => {
            handlers.onCopyTable?.({ table: tableData, format: 'markdown' });
          }
        }
      ]
    }
  ];
}

/**
 * Gera itens de menu para imagem.
 * 
 * @param {Object} imageData - Dados da imagem
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Array} Array de itens de menu
 */
export function getImageMenuItems(imageData, handlers = {}, t = (key) => key) {
  return [
    {
      id: 'imageZoom',
      label: t('chat.viewFullSize'),
      icon: '🔍',
      action: () => {
        handlers.onImageZoom?.(imageData);
      }
    },
    {
      id: 'imageCopy',
      label: t('chat.copy'),
      icon: '📋',
      action: () => {
        handlers.onImageCopy?.(imageData);
      }
    },
    {
      id: 'imageDownload',
      label: t('chat.download'),
      icon: '💾',
      action: () => {
        handlers.onImageDownload?.(imageData);
      }
    }
  ];
}

/**
 * Gera itens de menu para link.
 * 
 * @param {Object} linkData - { url, text }
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Array} Array de itens de menu
 */
export function getLinkMenuItems(linkData, handlers = {}, t = (key) => key) {
  const { url, text } = linkData;
  const displayText = text?.substring(0, 25) + (text?.length > 25 ? '...' : '');
  
  return [
    {
      id: 'linkOpen',
      label: 'Abrir no navegador',
      icon: '🌐',
      action: () => {
        if (handlers.onOpenLink) {
          handlers.onOpenLink({ url });
        } else {
          window.open(url, '_blank');
        }
      }
    },
    {
      id: 'linkCopy',
      label: 'Copiar URL',
      icon: '📋',
      action: () => {
        navigator.clipboard.writeText(url);
        handlers.onAnnounce?.(t('chat.copied'));
      }
    }
  ];
}

/**
 * Gera item de menu com submenu para múltiplos links.
 * 
 * @param {Array} links - Array de { url, text }
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Object|Array} Item de menu ou array de itens
 */
export function getLinksMenuItem(links, handlers = {}, t = (key) => key) {
  if (!links || links.length === 0) return null;
  
  if (links.length === 1) {
    const link = links[0];
    const displayText = link.text?.substring(0, 25) + (link.text?.length > 25 ? '...' : '');
    return {
      id: 'link',
      label: `Link: ${displayText}`,
      icon: '🔗',
      submenu: getLinkMenuItems(link, handlers, t)
    };
  }
  
  // Múltiplos links
  return links.map((link, i) => {
    const displayText = link.text?.substring(0, 20) + (link.text?.length > 20 ? '...' : '');
    return {
      id: `link-${i}`,
      label: `Link: ${displayText}`,
      icon: '🔗',
      submenu: getLinkMenuItems(link, handlers, t)
    };
  });
}

/**
 * Gera item de menu com submenu para múltiplas imagens.
 * 
 * @param {Array} images - Array de dados de imagem
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Object|Array} Item de menu ou array de itens
 */
export function getImagesMenuItem(images, handlers = {}, t = (key) => key) {
  if (!images || images.length === 0) return null;
  
  if (images.length === 1) {
    const img = images[0];
    const imgName = img.file?.name || 'imagem.png';
    return {
      id: 'image',
      label: 'Imagem anexada',
      icon: '🖼️',
      submenu: getImageMenuItems({ ...img, name: imgName }, handlers, t)
    };
  }
  
  // Múltiplas imagens
  return images.map((img, i) => {
    const imgName = img.file?.name || `imagem-${i + 1}.png`;
    return {
      id: `image-${i}`,
      label: `Imagem ${i + 1}`,
      icon: '🖼️',
      submenu: getImageMenuItems({ ...img, name: imgName }, handlers, t)
    };
  });
}

/**
 * Gera item de menu com submenu para múltiplos blocos de código.
 * 
 * @param {Array} codeBlocks - Array de { code, language }
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Object|null} Item de menu
 */
export function getCodeBlocksMenuItem(codeBlocks, handlers = {}, t = (key) => key) {
  if (!codeBlocks || codeBlocks.length === 0) return null;
  
  const submenu = [];
  
  // Separar Mermaid dos outros
  const mermaidBlocks = codeBlocks.filter(b => b.language?.toLowerCase() === 'mermaid');
  const otherBlocks = codeBlocks.filter(b => b.language?.toLowerCase() !== 'mermaid');
  
  // Adiciona Mermaid se existir
  mermaidBlocks.forEach((block, i) => {
    submenu.push({
      id: `mermaid-${i}`,
      label: mermaidBlocks.length === 1 ? 'Diagrama Mermaid' : `Mermaid ${i + 1}`,
      icon: '📐',
      action: () => {
        navigator.clipboard.writeText(block.code);
        handlers.onAnnounce?.(t('chat.copied'));
        handlers.onCopyCode?.({ code: block.code, language: 'mermaid' });
      }
    });
  });
  
  // Separador
  if (mermaidBlocks.length > 0 && otherBlocks.length > 0) {
    submenu.push({ separator: true });
  }
  
  // Outros blocos
  otherBlocks.forEach((block, i) => {
    submenu.push({
      id: `code-${i}`,
      label: otherBlocks.length === 1 ? `Código ${block.language}` : `${block.language} (${i + 1})`,
      icon: '💻',
      action: () => {
        navigator.clipboard.writeText(block.code);
        handlers.onAnnounce?.(t('chat.copied'));
        handlers.onCopyCode?.({ code: block.code, language: block.language });
      }
    });
  });
  
  // Copiar todos (se mais de um)
  if (otherBlocks.length > 1) {
    submenu.push({ separator: true });
    submenu.push({
      id: 'code-all',
      label: 'Copiar todos',
      icon: '📦',
      action: () => {
        const allCode = otherBlocks.map(b => `// ${b.language}\n${b.code}`).join('\n\n');
        navigator.clipboard.writeText(allCode);
        handlers.onAnnounce?.(t('chat.copied'));
      }
    });
  }
  
  const label = codeBlocks.length === 1
    ? `Copiar código ${codeBlocks[0].language}`
    : `Copiar código (${codeBlocks.length})`;
  
  return {
    id: 'code',
    label,
    icon: '💻',
    submenu
  };
}

/**
 * Gera item de menu com submenu para múltiplas tabelas.
 * 
 * @param {Array} tables - Array de dados de tabela
 * @param {Object} handlers - Handlers
 * @param {Function} t - Função de tradução
 * @returns {Object|Array|null} Item de menu ou array de itens
 */
export function getTablesMenuItem(tables, handlers = {}, t = (key) => key) {
  if (!tables || tables.length === 0) return null;
  
  const makeSubmenu = (table, index) => [
    {
      id: `table-${index}-text`,
      label: 'Texto tabulado',
      icon: '📊',
      action: () => handlers.onCopyTable?.({ table, format: 'text' })
    },
    {
      id: `table-${index}-csv`,
      label: 'CSV',
      icon: '📄',
      action: () => handlers.onCopyTable?.({ table, format: 'csv' })
    },
    {
      id: `table-${index}-md`,
      label: 'Markdown',
      icon: '📝',
      action: () => handlers.onCopyTable?.({ table, format: 'markdown' })
    }
  ];
  
  if (tables.length === 1) {
    return {
      id: 'table',
      label: 'Copiar tabela',
      icon: '📊',
      submenu: makeSubmenu(tables[0], 0)
    };
  }
  
  // Múltiplas tabelas
  return tables.map((table, i) => ({
    id: `table-${i}`,
    label: `Copiar tabela ${i + 1}`,
    icon: '📊',
    submenu: makeSubmenu(table, i)
  }));
}

