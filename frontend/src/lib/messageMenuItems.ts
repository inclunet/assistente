import { Message } from '../store/chatStore';
import { MenuItem } from '../components/menu';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import { visit } from 'unist-util-visit';
import type { Code, Link, Table } from 'mdast';
import { messageAudioService } from '../services/messageAudio';
import { stripMarkdown } from './stripMarkdown';

export interface MenuItemsOptions {
  onCopy?: (message: Message, asMarkdown: boolean) => void;
  onReadMessage?: (message: Message) => void;
  onSpeak?: (message: Message) => void;
  onEdit?: (message: Message) => void;
  onResend?: (message: Message) => void;
  onDelete?: (message: Message) => void;
  onPin?: (message: Message) => void;
  onAnnounce?: (text: string) => void;
  onSendToEditor?: (payload: {
    target: 'current' | 'new_document';
    format: 'markdown' | 'html' | 'plain';
    title?: string;
    content: string;
    kind: 'message' | 'code' | 'table' | 'link';
    index?: number;
  }) => void;
  onToggleReasoning?: (message: Message) => void; // Mostrar/ocultar reasoning
  isTTSDisabled?: boolean;
  isUser?: boolean;
  isReasoningExpanded?: boolean | ((messageId: string) => boolean); // Estado atual ou função
}

function fenceCodeBlock(params: { code: string; language?: string }) {
  const language = String(params.language || '').trim();
  const code = String(params.code || '');
  const langPart = language ? language : '';
  return `\n\n\`\`\`${langPart}\n${code}\n\`\`\`\n`;
}

function asMarkdownLink(params: { text: string; url: string }) {
  const text = String(params.text || '').trim() || String(params.url || '').trim();
  const url = String(params.url || '').trim();
  if (!url) return text;
  return `[${text}](${url})`;
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

    visit(tree, 'code', (node: Code) => {
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
    onReadMessage,
    onSpeak,
    onEdit,
    onResend,
    onDelete,
    onPin,
    onAnnounce,
    onSendToEditor,
    onToggleReasoning,
    isTTSDisabled = true,
    isUser = false,
    isReasoningExpanded = false,
  } = options;

  const items: MenuItem[] = [];
  const content = message.content || '';

  // Extrai elementos do markdown
  const codeBlocks = extractCodeBlocks(content);
  const links = extractLinks(content);
  const tables = extractTables(content);

  // 1. AÇÃO PRIMÁRIA: Modo de leitura (virtual modal)
  items.push({
    id: 'read-mode',
    label: 'Modo de leitura',
    icon: '📖',
    ariaLabel: 'Ativar modo de leitura da mensagem',
    shortcut: 'Enter',
    action: () => onReadMessage?.(message),
  });

  // 1.5 Ver/Ocultar Raciocínio (se a mensagem tem reasoning)
  type MessageFlags = Message & { reasoning?: string; pinned?: boolean };
  const messageFlags = message as MessageFlags;
  const hasReasoning = !!messageFlags.reasoning;
  if (hasReasoning && onToggleReasoning) {
    items.push({
      id: 'toggle-reasoning',
      label: isReasoningExpanded ? 'Ocultar raciocínio' : 'Ver raciocínio',
      icon: '🧠',
      ariaLabel: isReasoningExpanded ? 'Ocultar raciocínio do modelo' : 'Ver raciocínio do modelo',
      shortcut: 'R',
      action: () => onToggleReasoning(message),
    });
  }

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
    
    // 2.1 Baixar audio desta mensagem (usa DB se disponivel)
    items.push({
      id: 'download-audio',
      label: 'Baixar audio desta mensagem',
      icon: '💾',
      ariaLabel: 'Baixar audio desta mensagem',
      action: async () => {
        if (!message.content || !message.id) {
          onAnnounce?.('Mensagem sem conteudo');
          return;
        }

        try {
          const numericId = typeof message.id === 'string' ? parseInt(message.id, 10) : message.id;
          if (!numericId || isNaN(numericId)) {
            onAnnounce?.('Nao foi possivel identificar a mensagem');
            return;
          }

          onAnnounce?.('Gerando audio...');
          const audioBlob = await messageAudioService.getMessageAudioBlob(numericId);

          if (!audioBlob) {
            onAnnounce?.('Nao foi possivel gerar audio. Verifique a configuracao de voz no perfil ativo.');
            return;
          }

          const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
          const filename = `mensagem-${timestamp}.mp3`;
          messageAudioService.downloadAudioBlob(audioBlob, filename);
          onAnnounce?.('Audio baixado com sucesso');
        } catch (error) {
          console.error('Erro ao baixar audio:', error);
          onAnnounce?.('Erro ao gerar audio');
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

  // 4b. Enviar para o editor (mensagem inteira)
  if (onSendToEditor) {
    const contentMd = String(message.content || '');
    const contentPlain = stripMarkdown(contentMd);
    items.push({
      id: 'send-editor',
      label: 'Enviar ao editor',
      icon: '📝',
      ariaLabel: 'Enviar ao editor',
      submenu: [
        {
          id: 'send-editor-insert-md',
          label: 'Inserir no cursor (Markdown)',
          icon: '🧷',
          ariaLabel: 'Inserir no cursor do editor (Markdown)',
          action: () =>
            onSendToEditor({
              target: 'current',
              format: 'markdown',
              title: 'Mensagem (Markdown)',
              content: contentMd,
              kind: 'message',
            }),
        },
        {
          id: 'send-editor-new-md',
          label: 'Novo documento (Markdown)',
          icon: '📄',
          ariaLabel: 'Criar novo documento no editor (Markdown)',
          action: () =>
            onSendToEditor({
              target: 'new_document',
              format: 'markdown',
              title: 'Mensagem (Markdown)',
              content: contentMd,
              kind: 'message',
            }),
        },
        { id: 'send-editor-sep-plain', separator: true },
        {
          id: 'send-editor-insert-plain',
          label: 'Inserir no cursor (texto)',
          icon: '🧷',
          ariaLabel: 'Inserir no cursor do editor (texto)',
          action: () =>
            onSendToEditor({
              target: 'current',
              format: 'plain',
              title: 'Mensagem (texto)',
              content: contentPlain,
              kind: 'message',
            }),
        },
        {
          id: 'send-editor-new-plain',
          label: 'Novo documento (texto)',
          icon: '📄',
          ariaLabel: 'Criar novo documento no editor (texto)',
          action: () =>
            onSendToEditor({
              target: 'new_document',
              format: 'plain',
              title: 'Mensagem (texto)',
              content: contentPlain,
              kind: 'message',
            }),
        },
      ],
    });
  }

  // 5. Fixar/Desafixar
  if (onPin) {
    const isPinned = messageFlags.pinned || false;
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

  // Enviar blocos para o editor (acessibilidade: disponível no menu também)
  if (onSendToEditor && (codeBlocks.length > 0 || links.length > 0 || tables.length > 0)) {
    const submenu: MenuItem[] = [];

    if (codeBlocks.length > 0) {
      const codeMenu: MenuItem[] = codeBlocks.map((block, i) => {
        const language = String(block.language || 'text');
        const md = fenceCodeBlock({ code: block.code, language });
        return {
          id: `send-code-${i}`,
          label: codeBlocks.length === 1 ? `Código ${language}` : `${language} (${i + 1})`,
          icon: '💻',
          ariaLabel: `Enviar código ${language} ao editor`,
          submenu: [
            {
              id: `send-code-${i}-insert`,
              label: 'Inserir no cursor',
              icon: '🧷',
              ariaLabel: 'Inserir código no cursor do editor',
              action: () =>
                onSendToEditor({
                  target: 'current',
                  format: 'markdown',
                  title: `Código ${language}`,
                  content: md,
                  kind: 'code',
                  index: i,
                }),
            },
            {
              id: `send-code-${i}-new`,
              label: 'Novo documento',
              icon: '📄',
              ariaLabel: 'Criar novo documento com este código',
              action: () =>
                onSendToEditor({
                  target: 'new_document',
                  format: 'markdown',
                  title: `Código ${language}`,
                  content: md,
                  kind: 'code',
                  index: i,
                }),
            },
          ],
        };
      });

      submenu.push({
        id: 'send-code',
        label: codeBlocks.length === 1 ? 'Código' : `Códigos (${codeBlocks.length})`,
        icon: '💻',
        ariaLabel: codeBlocks.length === 1 ? 'Enviar código ao editor' : `Enviar ${codeBlocks.length} códigos ao editor`,
        submenu: codeMenu,
      });
    }

    if (tables.length > 0) {
      const tablesMenu: MenuItem[] = tables.map((table, i) => {
        const md = tableToMarkdown(table);
        const html = tableToHTML(table);
        return {
          id: `send-table-${i}`,
          label: tables.length === 1 ? 'Tabela' : `Tabela ${i + 1}`,
          icon: '📊',
          ariaLabel: `Enviar tabela ${i + 1} ao editor`,
          submenu: [
            {
              id: `send-table-${i}-md`,
              label: 'Inserir Markdown no cursor',
              icon: '🧷',
              ariaLabel: 'Inserir tabela (Markdown) no cursor do editor',
              action: () =>
                onSendToEditor({
                  target: 'current',
                  format: 'markdown',
                  title: tables.length === 1 ? 'Tabela (Markdown)' : `Tabela ${i + 1} (Markdown)`,
                  content: md,
                  kind: 'table',
                  index: i,
                }),
            },
            {
              id: `send-table-${i}-md-new`,
              label: 'Novo documento (Markdown)',
              icon: '📄',
              ariaLabel: 'Criar novo documento com a tabela (Markdown)',
              action: () =>
                onSendToEditor({
                  target: 'new_document',
                  format: 'markdown',
                  title: tables.length === 1 ? 'Tabela (Markdown)' : `Tabela ${i + 1} (Markdown)`,
                  content: md,
                  kind: 'table',
                  index: i,
                }),
            },
            { id: `send-table-${i}-sep`, separator: true },
            {
              id: `send-table-${i}-html`,
              label: 'Inserir HTML no cursor',
              icon: '🧷',
              ariaLabel: 'Inserir tabela (HTML) no cursor do editor',
              action: () =>
                onSendToEditor({
                  target: 'current',
                  format: 'html',
                  title: tables.length === 1 ? 'Tabela (HTML)' : `Tabela ${i + 1} (HTML)`,
                  content: html,
                  kind: 'table',
                  index: i,
                }),
            },
            {
              id: `send-table-${i}-html-new`,
              label: 'Novo documento (HTML)',
              icon: '📄',
              ariaLabel: 'Criar novo documento com a tabela (HTML)',
              action: () =>
                onSendToEditor({
                  target: 'new_document',
                  format: 'html',
                  title: tables.length === 1 ? 'Tabela (HTML)' : `Tabela ${i + 1} (HTML)`,
                  content: html,
                  kind: 'table',
                  index: i,
                }),
            },
          ],
        };
      });

      submenu.push({
        id: 'send-tables',
        label: tables.length === 1 ? 'Tabela' : `Tabelas (${tables.length})`,
        icon: '📊',
        ariaLabel: tables.length === 1 ? 'Enviar tabela ao editor' : `Enviar ${tables.length} tabelas ao editor`,
        submenu: tablesMenu,
      });
    }

    if (links.length > 0) {
      const linksMenu: MenuItem[] = links.map((link, i) => {
        const md = asMarkdownLink({ text: link.text, url: link.url });
        const label = link.text.substring(0, 28) + (link.text.length > 28 ? '…' : '');
        return {
          id: `send-link-${i}`,
          label,
          icon: '🔗',
          ariaLabel: `Enviar link ao editor: ${link.text}`,
          submenu: [
            {
              id: `send-link-${i}-insert`,
              label: 'Inserir no cursor',
              icon: '🧷',
              ariaLabel: 'Inserir link no cursor do editor',
              action: () =>
                onSendToEditor({
                  target: 'current',
                  format: 'markdown',
                  title: 'Link',
                  content: md,
                  kind: 'link',
                  index: i,
                }),
            },
            {
              id: `send-link-${i}-new`,
              label: 'Novo documento',
              icon: '📄',
              ariaLabel: 'Criar novo documento com este link',
              action: () =>
                onSendToEditor({
                  target: 'new_document',
                  format: 'markdown',
                  title: 'Link',
                  content: md,
                  kind: 'link',
                  index: i,
                }),
            },
          ],
        };
      });

      submenu.push({
        id: 'send-links',
        label: links.length === 1 ? 'Link' : `Links (${links.length})`,
        icon: '🔗',
        ariaLabel: links.length === 1 ? 'Enviar link ao editor' : `Enviar ${links.length} links ao editor`,
        submenu: linksMenu,
      });
    }

    items.push({
      id: 'send-blocks-editor',
      label: 'Enviar blocos ao editor',
      icon: '🧩',
      ariaLabel: 'Enviar blocos ao editor',
      submenu,
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
