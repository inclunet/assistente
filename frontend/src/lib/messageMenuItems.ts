import { logger } from '../utils/logger';
import { Message } from '../store/chatStore';
import { MenuItem } from '../components/menu';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import { visit } from 'unist-util-visit';
import type { Code, Link, Table } from 'mdast';
import { messageAudioService } from '../services/messageAudio';
import { ttsService } from '../services/tts';
import { stripMarkdown } from './stripMarkdown';
import { isBackendId } from './idUtils';
import i18next from 'i18next';
import {
  buildEditorDestinationSubmenu,
  type EditorSendFormatOption,
  type EditorSendTargetOption,
  type SendToEditorPayload,
} from './editorSendMenu';

export interface MenuItemsOptions {
  sessionKey?: string;
  onCopy?: (message: Message, asMarkdown: boolean) => void;
  onReadMessage?: (message: Message) => void;
  onSpeak?: (message: Message) => void;
  onEdit?: (message: Message) => void;
  onResend?: (message: Message) => void;
  onContinue?: (message: Message) => void;
  shouldShowContinue?: (message: Message) => boolean;
  onDelete?: (message: Message) => void;
  onCancelStreaming?: (message: Message) => void;
  onPin?: (message: Message) => void;
  onAnnounce?: (text: string) => void;
  onSendToEditor?: (payload: SendToEditorPayload & {
    kind: 'message' | 'code' | 'table' | 'link';
    index?: number;
  }) => void;
  editorTargets?: EditorSendTargetOption[];
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
    logger.error('Erro ao extrair blocos de código:', error);
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
    logger.error('Erro ao extrair links:', error);
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
      logger.error('❌ Erro ao processar markdown:', error);
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
    logger.error('❌ Erro ao extrair tabelas:', error);
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
    onContinue,
    shouldShowContinue,
    onDelete,
    onCancelStreaming,
    onPin,
    onAnnounce,
    onSendToEditor,
    editorTargets = [],
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
  const sendToEditorTexts = onSendToEditor
    ? {
        newDocumentLabel: i18next.t('editor.fallback.newDoc'),
        fallbackDocumentTitle: i18next.t('editor.fallback.title'),
        sendToEditorActionLabel: i18next.t('editor.sendToEditor.action'),
        markdownFormatLabel: i18next.t('editor.sendToEditor.format.markdown'),
        plainTextFormatLabel: i18next.t('editor.sendToEditor.format.plainText'),
        htmlFormatLabel: i18next.t('editor.sendToEditor.format.html'),
        markdownMessageTitle: i18next.t('editor.sendToEditor.title.markdownMessage'),
        plainTextMessageTitle: i18next.t('editor.sendToEditor.title.plainTextMessage'),
        codeTitle: (language: string) => i18next.t('editor.sendToEditor.title.code', { language }),
        markdownTableTitle: (index?: number) =>
          typeof index === 'number'
            ? i18next.t('editor.sendToEditor.title.markdownTableIndexed', { index })
            : i18next.t('editor.sendToEditor.title.markdownTable'),
        htmlTableTitle: (index?: number) =>
          typeof index === 'number'
            ? i18next.t('editor.sendToEditor.title.htmlTableIndexed', { index })
            : i18next.t('editor.sendToEditor.title.htmlTable'),
        linkTitle: i18next.t('editor.sendToEditor.title.link'),
        codeSingleLabel: (language: string) => i18next.t('editor.sendToEditor.blocks.codeSingle', { language }),
        codeIndexedLabel: (language: string, index: number) =>
          i18next.t('editor.sendToEditor.blocks.codeIndexed', { language, index }),
        codeGroupSingleLabel: i18next.t('editor.sendToEditor.blocks.codeGroupSingle'),
        codeGroupMultipleLabel: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.codeGroupMultiple', { count }),
        codeGroupAriaSingle: i18next.t('editor.sendToEditor.blocks.codeGroupAriaSingle'),
        codeGroupAriaMultiple: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.codeGroupAriaMultiple', { count }),
        codeAriaLabel: (language: string) => i18next.t('editor.sendToEditor.blocks.codeAria', { language }),
        tableSingleLabel: i18next.t('editor.sendToEditor.blocks.tableSingle'),
        tableIndexedLabel: (index: number) => i18next.t('editor.sendToEditor.blocks.tableIndexed', { index }),
        tableGroupSingleLabel: i18next.t('editor.sendToEditor.blocks.tableGroupSingle'),
        tableGroupMultipleLabel: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.tableGroupMultiple', { count }),
        tableGroupAriaSingle: i18next.t('editor.sendToEditor.blocks.tableGroupAriaSingle'),
        tableGroupAriaMultiple: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.tableGroupAriaMultiple', { count }),
        tableAriaLabel: (index: number) => i18next.t('editor.sendToEditor.blocks.tableAria', { index }),
        linkGroupSingleLabel: i18next.t('editor.sendToEditor.blocks.linkGroupSingle'),
        linkGroupMultipleLabel: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.linkGroupMultiple', { count }),
        linkGroupAriaSingle: i18next.t('editor.sendToEditor.blocks.linkGroupAriaSingle'),
        linkGroupAriaMultiple: (count: number) =>
          i18next.t('editor.sendToEditor.blocks.linkGroupAriaMultiple', { count }),
        linkAriaLabel: (linkText: string) => i18next.t('editor.sendToEditor.blocks.linkAria', { linkText }),
        sendBlocksLabel: i18next.t('editor.sendToEditor.blocks.sendBlocks'),
      }
    : null;

  // 1. AÇÃO PRIMÁRIA: Modo de leitura (virtual modal)
  items.push({
    id: 'read-mode',
    label: 'Modo de leitura',
    icon: '📖',
    ariaLabel: 'Ativar modo de leitura da mensagem',
    shortcut: 'Enter',
    action: () => onReadMessage?.(message),
  });

  if (message.isStreaming && onCancelStreaming) {
    items.push({
      id: 'cancel-generation',
      label: i18next.t('chat.cancelGeneration'),
      icon: '⏹',
      ariaLabel: i18next.t('chat.cancelGenerationLabel'),
      shortcut: 'Esc',
      action: () => onCancelStreaming(message),
    });
  }

  if (onContinue && (shouldShowContinue?.(message) ?? false)) {
    items.push({
      id: 'continue-response',
      label: i18next.t('chat.continueResponse'),
      icon: '⏭',
      ariaLabel: i18next.t('chat.continueResponseLabel'),
      action: () => onContinue(message),
    });
  }

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
      label: i18next.t('chat.downloadAudio'),
      icon: '💾',
      ariaLabel: i18next.t('chat.downloadAudio'),
      action: async () => {
        if (!message.content || !message.id) {
          onAnnounce?.(i18next.t('chat.announce.noContent'));
          return;
        }

        try {
          const backendId = isBackendId(message.id) ? message.id : '';
          if (!backendId) {
            onAnnounce?.(i18next.t('chat.announce.cannotIdentifyMessage'));
            return;
          }

          onAnnounce?.(i18next.t('chat.announce.generatingAudio'));
          const role = message.role === 'user' ? 'user' : 'assistant';
          const voiceCtx = ttsService.getVoiceContext(role);
          const audioBlob = await messageAudioService.getMessageAudioBlob(backendId, voiceCtx);

          if (!audioBlob) {
            onAnnounce?.(i18next.t('chat.announce.cannotGenerateAudio'));
            return;
          }

          const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
          const prefix = i18next.t('chat.downloadAudioPrefix');
          const filename = `${prefix}-${timestamp}.mp3`;
          messageAudioService.downloadAudioBlob(audioBlob, filename);
          onAnnounce?.(i18next.t('chat.announce.audioDownloaded'));
        } catch (error) {
          logger.error('Erro ao baixar audio:', error);
          onAnnounce?.(i18next.t('chat.announce.audioError'));
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
  if (onSendToEditor && sendToEditorTexts) {
    const contentMd = String(message.content || '');
    const contentPlain = stripMarkdown(contentMd);
    const messageFormats: Array<EditorSendFormatOption<{ kind: 'message' }>> = [
      {
        id: 'markdown',
        label: sendToEditorTexts.markdownFormatLabel,
        payload: {
          format: 'markdown',
          title: sendToEditorTexts.markdownMessageTitle,
          content: contentMd,
          kind: 'message',
        },
      },
      {
        id: 'plain',
        label: sendToEditorTexts.plainTextFormatLabel,
        payload: {
          format: 'plain',
          title: sendToEditorTexts.plainTextMessageTitle,
          content: contentPlain,
          kind: 'message',
        },
      },
    ];
    items.push({
      id: 'send-editor',
      label: sendToEditorTexts.sendToEditorActionLabel,
      icon: '📝',
      ariaLabel: sendToEditorTexts.sendToEditorActionLabel,
      submenu: buildEditorDestinationSubmenu({
        baseId: 'send-editor',
        editorTargets,
        formats: messageFormats,
        onSendToEditor,
        newDocumentLabel: sendToEditorTexts.newDocumentLabel,
        fallbackDocumentTitle: sendToEditorTexts.fallbackDocumentTitle,
      }),
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
  if (onSendToEditor && sendToEditorTexts && (codeBlocks.length > 0 || links.length > 0 || tables.length > 0)) {
    const submenu: MenuItem[] = [];

    if (codeBlocks.length > 0) {
      const codeMenu: MenuItem[] = codeBlocks.map((block, i) => {
        const language = String(block.language || 'text');
        const md = fenceCodeBlock({ code: block.code, language });
        return {
          id: `send-code-${i}`,
          label: codeBlocks.length === 1 ? sendToEditorTexts.codeSingleLabel(language) : sendToEditorTexts.codeIndexedLabel(language, i + 1),
          icon: '💻',
          ariaLabel: sendToEditorTexts.codeAriaLabel(language),
          submenu: buildEditorDestinationSubmenu({
            baseId: `send-code-${i}`,
            editorTargets,
            formats: [{
              id: 'markdown',
              label: sendToEditorTexts.markdownFormatLabel,
              payload: {
                format: 'markdown',
                title: sendToEditorTexts.codeTitle(language),
                content: md,
                kind: 'code',
                index: i,
              },
            }],
            onSendToEditor,
            newDocumentLabel: sendToEditorTexts.newDocumentLabel,
            fallbackDocumentTitle: sendToEditorTexts.fallbackDocumentTitle,
          }),
        };
      });

      submenu.push({
        id: 'send-code',
        label: codeBlocks.length === 1 ? sendToEditorTexts.codeGroupSingleLabel : sendToEditorTexts.codeGroupMultipleLabel(codeBlocks.length),
        icon: '💻',
        ariaLabel: codeBlocks.length === 1 ? sendToEditorTexts.codeGroupAriaSingle : sendToEditorTexts.codeGroupAriaMultiple(codeBlocks.length),
        submenu: codeMenu,
      });
    }

    if (tables.length > 0) {
      const tablesMenu: MenuItem[] = tables.map((table, i) => {
        const md = tableToMarkdown(table);
        const html = tableToHTML(table);
        return {
          id: `send-table-${i}`,
          label: tables.length === 1 ? sendToEditorTexts.tableSingleLabel : sendToEditorTexts.tableIndexedLabel(i + 1),
          icon: '📊',
          ariaLabel: sendToEditorTexts.tableAriaLabel(i + 1),
          submenu: buildEditorDestinationSubmenu({
            baseId: `send-table-${i}`,
            editorTargets,
            formats: [
              {
                id: 'markdown',
                label: sendToEditorTexts.markdownFormatLabel,
                payload: {
                  format: 'markdown',
                  title: tables.length > 1 ? sendToEditorTexts.markdownTableTitle(i + 1) : sendToEditorTexts.markdownTableTitle(),
                  content: md,
                  kind: 'table',
                  index: i,
                },
              },
              {
                id: 'html',
                label: sendToEditorTexts.htmlFormatLabel,
                payload: {
                  format: 'html',
                  title: tables.length > 1 ? sendToEditorTexts.htmlTableTitle(i + 1) : sendToEditorTexts.htmlTableTitle(),
                  content: html,
                  kind: 'table',
                  index: i,
                },
              },
            ],
            onSendToEditor,
            newDocumentLabel: sendToEditorTexts.newDocumentLabel,
            fallbackDocumentTitle: sendToEditorTexts.fallbackDocumentTitle,
          }),
        };
      });

      submenu.push({
        id: 'send-tables',
        label: tables.length === 1 ? sendToEditorTexts.tableGroupSingleLabel : sendToEditorTexts.tableGroupMultipleLabel(tables.length),
        icon: '📊',
        ariaLabel: tables.length === 1 ? sendToEditorTexts.tableGroupAriaSingle : sendToEditorTexts.tableGroupAriaMultiple(tables.length),
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
          ariaLabel: sendToEditorTexts.linkAriaLabel(link.text),
          submenu: buildEditorDestinationSubmenu({
            baseId: `send-link-${i}`,
            editorTargets,
            formats: [{
              id: 'markdown',
              label: sendToEditorTexts.markdownFormatLabel,
              payload: {
                format: 'markdown',
                title: sendToEditorTexts.linkTitle,
                content: md,
                kind: 'link',
                index: i,
              },
            }],
            onSendToEditor,
            newDocumentLabel: sendToEditorTexts.newDocumentLabel,
            fallbackDocumentTitle: sendToEditorTexts.fallbackDocumentTitle,
          }),
        };
      });

      submenu.push({
        id: 'send-links',
        label: links.length === 1 ? sendToEditorTexts.linkGroupSingleLabel : sendToEditorTexts.linkGroupMultipleLabel(links.length),
        icon: '🔗',
        ariaLabel: links.length === 1 ? sendToEditorTexts.linkGroupAriaSingle : sendToEditorTexts.linkGroupAriaMultiple(links.length),
        submenu: linksMenu,
      });
    }

    items.push({
      id: 'send-blocks-editor',
      label: sendToEditorTexts.sendBlocksLabel,
      icon: '🧩',
      ariaLabel: sendToEditorTexts.sendBlocksLabel,
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
