import { Message } from '../store/chatStore';
import { MenuItem } from '../components/ui/ContextMenu';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import { visit } from 'unist-util-visit';
import type { Code, Link, Table } from 'mdast';
import { messageAudioService } from '../services/messageAudio';
import { ttsService } from '../services/tts';
import { stripMarkdown } from './stripMarkdown';

export interface MenuItemsOptions {
  onCopy?: (message: Message, asMarkdown: boolean) => void;
  onOpenDetail?: (message: Message) => void;
  onSpeak?: (message: Message) => void;
  onEdit?: (message: Message) => void;
  onResend?: (message: Message) => void;
  onDelete?: (message: Message) => void;
  onPin?: (message: Message) => void;
  onAnnounce?: (text: string) => void;
  isTTSDisabled?: boolean;
  isUser?: boolean;
}

// Extrai blocos de código do markdown usando AST
function extractCodeBlocks(content: string): Array<{ code: string; language: string }> {
  const blocks: Array<{ code: string; language: string }> = [];
  
  try {
    const tree = unified()
      .use(remarkParse)
      .use(remarkGfm)
      .parse(content);

    visit(tree, 'code', (node: Code) => {
      // Ignora blocos markdown/md que são usados para tabelas
      const lang = (node.lang || 'text').toLowerCase();
      if (lang === 'markdown' || lang === 'md') {
        return;
      }
      
      blocks.push({
        code: node.value,
        language: node.lang || 'text',
      });
    });
  } catch (error) {
    console.error('Erro ao extrair blocos de código:', error);
  }

  console.log('🔍 extractCodeBlocks:', { total: blocks.length, blocks });
  return blocks;
}

// Extrai links do markdown usando AST
function extractLinks(content: string): Array<{ url: string; text: string }> {
  const links: Array<{ url: string; text: string }> = [];
  
  try {
    const tree = unified()
      .use(remarkParse)
      .use(remarkGfm)
      .parse(content);

    visit(tree, 'link', (node: Link) => {
      // Extrair texto do link (pode ter múltiplos children)
      const text = node.children
        .map(child => ('value' in child ? child.value : ''))
        .join('')
        .trim();
      
      if (text && node.url) {
        links.push({
          text,
          url: node.url,
        });
      }
    });
  } catch (error) {
    console.error('Erro ao extrair links:', error);
  }

  console.log('🔗 extractLinks:', { total: links.length, links });
  return links;
}

// Extrai tabelas do markdown usando AST
function extractTables(content: string): Array<{ headers: string[]; rows: string[][] }> {
  const tables: Array<{ headers: string[]; rows: string[][] }> = [];
  
  const processMarkdown = (markdown: string) => {
    try {
      const tree = unified()
        .use(remarkParse)
        .use(remarkGfm)
        .parse(markdown);

      visit(tree, 'table', (node: Table) => {
        if (node.children.length === 0) return;

        // Primeira linha é o cabeçalho
        const headerRow = node.children[0];
        const headers = headerRow.children.map(cell => {
          return cell.children
            .map(child => ('value' in child ? child.value : ''))
            .join('')
            .trim();
        });

        // Demais linhas são os dados
        const rows = node.children.slice(1).map(row => {
          return row.children.map(cell => {
            return cell.children
              .map(child => ('value' in child ? child.value : ''))
              .join('')
              .trim();
          });
        });

        if (headers.length > 0 && rows.length > 0) {
          tables.push({ headers, rows });
        }
      });
    } catch (error) {
      console.error('❌ Erro ao processar markdown:', error);
    }
  };

  try {
    // Processa o markdown principal
    processMarkdown(content);

    // Também processa blocos de código markdown
    const tree = unified()
      .use(remarkParse)
      .use(remarkGfm)
      .parse(content);

    visit(tree, 'code', (node: any) => {
      if (node.lang === 'markdown' || node.lang === 'md') {
        processMarkdown(node.value);
      }
    });
  } catch (error) {
    console.error('❌ Erro ao extrair tabelas:', error);
  }

  return tables;
}

// Converte tabela para texto
function tableToText(table: { headers: string[]; rows: string[][] }): string {
  const colWidths = table.headers.map((h, i) => {
    const maxRowWidth = Math.max(
      ...table.rows.map((r) => (r[i] || '').length)
    );
    return Math.max(h.length, maxRowWidth);
  });
  
  const formatRow = (cells: string[]) =>
    cells.map((cell, i) => cell.padEnd(colWidths[i])).join(' | ');
  
  const header = formatRow(table.headers);
  const separator = colWidths.map((w) => '-'.repeat(w)).join('-+-');
  const rows = table.rows.map((row) => formatRow(row));
  
  return [header, separator, ...rows].join('\n');
}

// Converte tabela para CSV
function tableToCSV(table: { headers: string[]; rows: string[][] }): string {
  const escapeCSV = (value: string) => {
    if (value.includes(',') || value.includes('"') || value.includes('\n')) {
      return `"${value.replace(/"/g, '""')}"`;
    }
    return value;
  };
  
  const header = table.headers.map(escapeCSV).join(',');
  const rows = table.rows.map((row) => row.map(escapeCSV).join(','));
  
  return [header, ...rows].join('\n');
}

// Converte tabela para Markdown
function tableToMarkdown(table: { headers: string[]; rows: string[][] }): string {
  const header = '| ' + table.headers.join(' | ') + ' |';
  const separator = '| ' + table.headers.map(() => '---').join(' | ') + ' |';
  const rows = table.rows.map((row) => '| ' + row.join(' | ') + ' |');
  
  return [header, separator, ...rows].join('\n');
}

// Converte tabela para HTML
function tableToHTML(table: { headers: string[]; rows: string[][] }): string {
  const escapeHTML = (str: string) =>
    str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  
  const headerCells = table.headers
    .map((h) => `    <th>${escapeHTML(h)}</th>`)
    .join('\n');
  
  const bodyRows = table.rows
    .map((row) => {
      const cells = row.map((cell) => `      <td>${escapeHTML(cell)}</td>`).join('\n');
      return `    <tr>\n${cells}\n    </tr>`;
    })
    .join('\n');
  
  return `<table>
  <thead>
    <tr>
${headerCells}
    </tr>
  </thead>
  <tbody>
${bodyRows}
  </tbody>
</table>`;
}

export function getMessageMenuItems(
  message: Message,
  options: MenuItemsOptions = {}
): MenuItem[] {
  const {
    onCopy,
    onOpenDetail,
    onSpeak,
    onEdit,
    onResend,
    onDelete,
    onPin,
    onAnnounce,
    isTTSDisabled = true,
    isUser = false,
  } = options;

  const items: MenuItem[] = [];
  const content = message.content || '';

  // Extrai elementos do markdown
  const codeBlocks = extractCodeBlocks(content);
  const links = extractLinks(content);
  const tables = extractTables(content);
  
  console.log('📊 Elementos extraídos:', {
    content: content.substring(0, 100) + '...',
    codeBlocks: codeBlocks.length,
    codeBlocksData: codeBlocks,
    links: links.length,
    linksData: links,
    tables: tables.length,
    tablesData: tables,
  });

  // 1. AÇÃO PRIMÁRIA: Ver em tela cheia
  items.push({
    id: 'fullscreen',
    label: 'Ver em tela cheia',
    icon: '🔍',
    ariaLabel: 'Ver em tela cheia',
    shortcut: 'Enter',
    action: () => onOpenDetail?.(message),
  });

  // 2. AÇÃO SECUNDÁRIA: Ouvir mensagem (se TTS estiver habilitado)
  if (!isTTSDisabled) {
    items.push({
      id: 'speak',
      label: 'Ouvir mensagem',
      icon: '🔊',
      ariaLabel: 'Ouvir mensagem',
      shortcut: 'Espaço',
      action: () => onSpeak?.(message),
    });
    
    // 2.1 Baixar áudio desta mensagem (apenas para providers que suportam síntese)
    items.push({
      id: 'download-audio',
      label: 'Baixar áudio desta mensagem',
      icon: '💾',
      ariaLabel: 'Sintetizar e baixar áudio desta mensagem',
      action: async () => {
        if (!message.content) {
          onAnnounce?.('Mensagem sem conteúdo');
          return;
        }
        
        onAnnounce?.('Sintetizando áudio...');
        
        try {
          // Remove markdown do texto
          const cleanText = stripMarkdown(message.content);
          
          // Sintetiza o áudio
          const audioBlob = await ttsService.synthesizeForMessage(cleanText);
          
          if (!audioBlob) {
            onAnnounce?.('Não foi possível sintetizar o áudio. Verifique se o TTS está configurado corretamente.');
            return;
          }
          
          // Gera nome do arquivo com timestamp
          const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
          const filename = `mensagem-${timestamp}.mp3`;
          
          // Baixa o arquivo
          messageAudioService.downloadAudioBlob(audioBlob, filename);
          onAnnounce?.('Áudio baixado com sucesso');
        } catch (error) {
          console.error('Erro ao baixar áudio:', error);
          onAnnounce?.('Erro ao sintetizar áudio');
        }
      },
    });
  }

  // 3. Copiar mensagem
  items.push({
    id: 'copy',
    label: 'Copiar mensagem',
    icon: '📋',
    ariaLabel: 'Copiar mensagem',
    shortcut: 'Ctrl+C',
    action: () => onCopy?.(message, false),
  });

  // 4. Copiar como Markdown
  items.push({
    id: 'copy-md',
    label: 'Copiar como Markdown',
    icon: '📝',
    ariaLabel: 'Copiar como Markdown',
    shortcut: 'Ctrl+Shift+C',
    action: () => onCopy?.(message, true),
  });

  // 5. Fixar/Desafixar
  if (onPin) {
    const isPinned = (message as any).pinned || false;
    items.push({
      id: 'pin',
      label: isPinned ? 'Desafixar mensagem' : 'Fixar mensagem',
      icon: isPinned ? '📍' : '📌',
      ariaLabel: isPinned ? 'Desafixar mensagem' : 'Fixar mensagem',
      action: () => onPin(message),
    });
  }

  // Separador antes de elementos dinâmicos
  if (codeBlocks.length > 0 || links.length > 0 || tables.length > 0) {
    items.push({
      id: 'separator-markdown',
      separator: true,
    });
  }

  // Copiar blocos de código
  if (codeBlocks.length > 0) {
    const label =
      codeBlocks.length === 1
        ? `Copiar código ${codeBlocks[0].language}`
        : `Copiar código (${codeBlocks.length})`;

    const submenu: MenuItem[] = codeBlocks.map((block, i) => ({
      id: `code-${i}`,
      label:
        codeBlocks.length === 1
          ? `Código ${block.language}`
          : `${block.language} (${i + 1})`,
      icon: '💻',
      ariaLabel:
        codeBlocks.length === 1
          ? `Código ${block.language}`
          : `${block.language} bloco ${i + 1}`,
      action: () => {
        navigator.clipboard.writeText(block.code);
        onAnnounce?.('Código copiado');
      },
    }));

    if (codeBlocks.length > 1) {
      submenu.push({ id: 'code-sep', separator: true });
      submenu.push({
        id: 'code-all',
        label: 'Copiar todos',
        icon: '📦',
        ariaLabel: 'Copiar todos os códigos',
        action: () => {
          const allCode = codeBlocks
            .map((b) => `// ${b.language}\n${b.code}`)
            .join('\n\n');
          navigator.clipboard.writeText(allCode);
          onAnnounce?.('Todos os códigos copiados');
        },
      });
    }

    items.push({
      id: 'code',
      label,
      icon: '💻',
      ariaLabel: label,
      submenu,
    });
  }

  // Copiar links
  if (links.length > 0) {
    const submenu: MenuItem[] = links.map((link, i) => {
      const displayText =
        link.text.substring(0, 20) + (link.text.length > 20 ? '...' : '');
      return {
        id: `link-${i}`,
        label: displayText,
        icon: '🔗',
        ariaLabel: link.text,
        submenu: [
          {
            id: `link-${i}-open`,
            label: 'Abrir no navegador',
            icon: '🌐',
            ariaLabel: 'Abrir no navegador',
            action: () => window.open(link.url, '_blank'),
          },
          {
            id: `link-${i}-copy`,
            label: 'Copiar URL',
            icon: '📋',
            ariaLabel: 'Copiar URL',
            action: () => {
              navigator.clipboard.writeText(link.url);
              onAnnounce?.('URL copiada');
            },
          },
        ],
      };
    });

    items.push({
      id: 'links',
      label: links.length === 1 ? 'Link' : `Links (${links.length})`,
      icon: '🔗',
      ariaLabel: links.length === 1 ? 'Link' : `${links.length} links`,
      submenu,
    });
  }

  // Copiar tabelas
  if (tables.length > 0) {
    // Se há apenas uma tabela, criar submenu direto com os formatos
    if (tables.length === 1) {
      const table = tables[0];
      items.push({
        id: 'table-copy',
        label: 'Copiar tabela',
        icon: '📊',
        ariaLabel: 'Copiar tabela',
        submenu: [
          {
            id: 'table-text',
            label: 'Texto tabulado',
            icon: '📊',
            ariaLabel: 'Texto tabulado',
            action: () => {
              navigator.clipboard.writeText(tableToText(table));
              onAnnounce?.('Tabela copiada como texto');
            },
          },
          {
            id: 'table-csv',
            label: 'CSV',
            icon: '📄',
            ariaLabel: 'CSV',
            action: () => {
              navigator.clipboard.writeText(tableToCSV(table));
              onAnnounce?.('Tabela copiada como CSV');
            },
          },
          {
            id: 'table-md',
            label: 'Markdown',
            icon: '📝',
            ariaLabel: 'Markdown',
            action: () => {
              navigator.clipboard.writeText(tableToMarkdown(table));
              onAnnounce?.('Tabela copiada como Markdown');
            },
          },
          {
            id: 'table-html',
            label: 'HTML',
            icon: '🌐',
            ariaLabel: 'HTML',
            action: () => {
              navigator.clipboard.writeText(tableToHTML(table));
              onAnnounce?.('Tabela copiada como HTML');
            },
          },
        ],
      });
    } else {
      // Se há múltiplas tabelas, criar submenu com cada tabela tendo seus formatos
      const submenu: MenuItem[] = tables.map((table, i) => ({
        id: `table-${i}`,
        label: `Tabela ${i + 1}`,
        icon: '📊',
        ariaLabel: `Tabela ${i + 1}`,
        submenu: [
          {
            id: `table-${i}-text`,
            label: 'Texto tabulado',
            icon: '📊',
            ariaLabel: 'Texto tabulado',
            action: () => {
              navigator.clipboard.writeText(tableToText(table));
              onAnnounce?.('Tabela copiada como texto');
            },
          },
          {
            id: `table-${i}-csv`,
            label: 'CSV',
            icon: '📄',
            ariaLabel: 'CSV',
            action: () => {
              navigator.clipboard.writeText(tableToCSV(table));
              onAnnounce?.('Tabela copiada como CSV');
            },
          },
          {
            id: `table-${i}-md`,
            label: 'Markdown',
            icon: '📝',
            ariaLabel: 'Markdown',
            action: () => {
              navigator.clipboard.writeText(tableToMarkdown(table));
              onAnnounce?.('Tabela copiada como Markdown');
            },
          },
          {
            id: `table-${i}-html`,
            label: 'HTML',
            icon: '🌐',
            ariaLabel: 'HTML',
            action: () => {
              navigator.clipboard.writeText(tableToHTML(table));
              onAnnounce?.('Tabela copiada como HTML');
            },
          },
        ],
      }));

      items.push({
        id: 'tables',
        label: `Tabelas (${tables.length})`,
        icon: '📊',
        ariaLabel: `${tables.length} tabelas`,
        submenu,
      });
    }
  }

  // Separador antes de opções de usuário
  if (isUser && (onResend || onEdit || onDelete)) {
    items.push({
      id: 'separator-user',
      separator: true,
    });
  }

  // Opções exclusivas para mensagens do usuário
  if (isUser) {
    if (onEdit) {
      items.push({
        id: 'edit',
        label: 'Editar mensagem',
        icon: '✏️',
        ariaLabel: 'Editar mensagem',
        shortcut: 'F2',
        action: () => onEdit(message),
      });
    }

    if (onResend) {
      items.push({
        id: 'resend',
        label: 'Reenviar mensagem',
        icon: '🔄',
        ariaLabel: 'Reenviar mensagem',
        action: () => onResend(message),
      });
    }

    if (onDelete) {
      items.push({
        id: 'delete',
        label: 'Excluir mensagem',
        icon: '🗑️',
        ariaLabel: 'Excluir mensagem',
        shortcut: 'Delete',
        danger: true,
        action: () => onDelete(message),
      });
    }
  }

  return items;
}
