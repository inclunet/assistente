import { Fragment, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ApiOutlined,
  AudioOutlined,
  BulbOutlined,
  ConsoleSqlOutlined,
  ExportOutlined,
  FolderOutlined,
  HistoryOutlined,
  InteractionOutlined,
  KeyOutlined,
  MessageOutlined,
  MobileOutlined,
  PaperClipOutlined,
  ReadOutlined,
  SafetyOutlined,
  SaveOutlined,
  SettingOutlined,
  SoundOutlined,
  ToolOutlined,
  UserSwitchOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import './HelpPage.css';

type HelpListItem = string | { text: string; sub: string[] };

type HelpBlock =
  | { type: 'p'; text: string }
  | { type: 'h4'; text: string }
  | { type: 'ul'; items: HelpListItem[] }
  | { type: 'ol'; items: HelpListItem[] }
  | { type: 'table'; headers: string[]; rows: string[][] };

interface HelpSectionData {
  title: string;
  blocks: HelpBlock[];
}

interface HelpSection extends HelpSectionData {
  id: string;
  icon: ReactNode;
}

const SECTION_DEFS: { id: string; key: string; icon: ReactNode }[] = [
  { id: 'commands', key: 'commands', icon: <SoundOutlined aria-hidden="true" /> },
  { id: 'overview', key: 'overview', icon: <ReadOutlined aria-hidden="true" /> },
  { id: 'chat', key: 'chat', icon: <MessageOutlined aria-hidden="true" /> },
  { id: 'voice', key: 'voice', icon: <AudioOutlined aria-hidden="true" /> },
  { id: 'files', key: 'files', icon: <FolderOutlined aria-hidden="true" /> },
  { id: 'profiles', key: 'profiles', icon: <UserSwitchOutlined aria-hidden="true" /> },
  { id: 'settings', key: 'settings', icon: <SettingOutlined aria-hidden="true" /> },
  { id: 'history', key: 'history', icon: <HistoryOutlined aria-hidden="true" /> },
  { id: 'terminal', key: 'terminal', icon: <ConsoleSqlOutlined aria-hidden="true" /> },
  { id: 'mcp', key: 'mcp', icon: <ApiOutlined aria-hidden="true" /> },
  { id: 'skills', key: 'skills', icon: <BulbOutlined aria-hidden="true" /> },
  { id: 'allowlists', key: 'allowlists', icon: <SafetyOutlined aria-hidden="true" /> },
  { id: 'channels', key: 'channels', icon: <MobileOutlined aria-hidden="true" /> },
  { id: 'keyboard', key: 'keyboard', icon: <KeyOutlined aria-hidden="true" /> },
  { id: 'accessibility', key: 'accessibility', icon: <InteractionOutlined aria-hidden="true" /> },
  { id: 'export-import', key: 'exportImport', icon: <ExportOutlined aria-hidden="true" /> },
  { id: 'data-storage', key: 'dataStorage', icon: <SaveOutlined aria-hidden="true" /> },
  { id: 'troubleshooting', key: 'troubleshooting', icon: <ToolOutlined aria-hidden="true" /> },
];

const INLINE_ICONS: Record<string, ReactNode> = {
  audio: <AudioOutlined aria-hidden="true" />,
  paperclip: <PaperClipOutlined aria-hidden="true" />,
  warning: <WarningOutlined aria-hidden="true" />,
};

// Mini-marcacao usada nos locales para preservar a formatacao original sem
// strings hardcoded no JSX:
//   **negrito**  -> <strong>   *italico* -> <em>   `codigo` -> <code>
//   [[Ctrl]]     -> <kbd>      [icon:nome] -> icone decorativo (aria-hidden)
const INLINE_PATTERN = '(\\[\\[[^\\]]+\\]\\]|\\[icon:[a-zA-Z]+\\]|\\*\\*[^*]+\\*\\*|\\*[^*]+\\*|`[^`]+`)';

function renderInline(text: string): ReactNode[] {
  const re = new RegExp(INLINE_PATTERN, 'g');
  const nodes: ReactNode[] = [];
  let last = 0;
  let key = 0;
  let match: RegExpExecArray | null;
  while ((match = re.exec(text)) !== null) {
    if (match.index > last) {
      nodes.push(text.slice(last, match.index));
    }
    const token = match[0];
    if (token.startsWith('[[')) {
      nodes.push(<kbd key={key++}>{token.slice(2, -2)}</kbd>);
    } else if (token.startsWith('[icon:')) {
      const icon = INLINE_ICONS[token.slice(6, -1)];
      // Icone desconhecido: cai para o texto original do token (nunca descarta conteudo).
      nodes.push(icon ? <Fragment key={key++}>{icon}</Fragment> : token);
    } else if (token.startsWith('**')) {
      nodes.push(<strong key={key++}>{renderInline(token.slice(2, -2))}</strong>);
    } else if (token.startsWith('`')) {
      nodes.push(<code key={key++}>{token.slice(1, -1)}</code>);
    } else {
      nodes.push(<em key={key++}>{renderInline(token.slice(1, -1))}</em>);
    }
    last = re.lastIndex;
  }
  if (last < text.length) {
    nodes.push(text.slice(last));
  }
  return nodes;
}

function renderListItem(item: HelpListItem, key: number): ReactNode {
  if (typeof item === 'string') {
    return <li key={key}>{renderInline(item)}</li>;
  }
  return (
    <li key={key}>
      {renderInline(item.text)}
      <ul>
        {item.sub.map((sub, i) => (
          <li key={i}>{renderInline(sub)}</li>
        ))}
      </ul>
    </li>
  );
}

function renderBlock(block: HelpBlock, key: number): ReactNode {
  switch (block.type) {
    case 'p':
      return <p key={key}>{renderInline(block.text)}</p>;
    case 'h4':
      return <h4 key={key}>{renderInline(block.text)}</h4>;
    case 'ul':
      return <ul key={key}>{block.items.map((item, i) => renderListItem(item, i))}</ul>;
    case 'ol':
      return <ol key={key}>{block.items.map((item, i) => renderListItem(item, i))}</ol>;
    case 'table':
      return (
        <table key={key} className="help-shortcuts">
          <thead>
            <tr>
              {block.headers.map((header, i) => (
                <th key={i}>{renderInline(header)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {block.rows.map((row, i) => (
              <tr key={i}>
                {row.map((cell, j) => (
                  <td key={j}>{renderInline(cell)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      );
    default:
      return null;
  }
}

// Valida em runtime o retorno de t(..., returnObjects): se a chave estiver ausente
// o i18next devolve a string da chave, e um cast cego deixaria blocks undefined.
function isHelpSectionData(value: unknown): value is HelpSectionData {
  return (
    typeof value === 'object' &&
    value !== null &&
    typeof (value as HelpSectionData).title === 'string' &&
    Array.isArray((value as HelpSectionData).blocks)
  );
}

export default function HelpPage() {
  const { t, i18n } = useTranslation();
  useContentPageLandmarks({ pageClass: 'help-page' });
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['commands']));

  const sections = useMemo<HelpSection[]>(() => {
    return SECTION_DEFS.map((def) => {
      const raw = t(`help.sections.${def.key}`, { returnObjects: true }) as unknown;
      // Fallback seguro para secao vazia se os locales divergirem (evita crash em blocks.map).
      const data: HelpSectionData = isHelpSectionData(raw) ? raw : { title: '', blocks: [] };
      return {
        id: def.id,
        icon: def.icon,
        title: data.title,
        blocks: data.blocks,
      };
    });
    // i18n.language garante recomputo ao trocar de idioma
  }, [t, i18n.language]);

  const toggleSection = (id: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const expandAll = () => {
    setExpandedSections(new Set(sections.map((s) => s.id)));
  };

  const collapseAll = () => {
    setExpandedSections(new Set());
  };

  return (
    <div className="help-page">
      <div className="help-header">
        <h1>{t('help.title')}</h1>
        <p>{t('help.subtitle')}</p>
        <div className="help-header-actions">
          <button className="help-expand-btn" onClick={expandAll}>
            {t('help.expandAll')}
          </button>
          <button className="help-expand-btn" onClick={collapseAll}>
            {t('help.collapseAll')}
          </button>
        </div>
      </div>

      <main className="help-main">
        {sections.map((section) => {
          const isExpanded = expandedSections.has(section.id);
          return (
            <section
              key={section.id}
              id={`help-${section.id}`}
              className={`help-section ${isExpanded ? 'expanded' : ''}`}
            >
              <button
                className="help-section-header"
                onClick={() => toggleSection(section.id)}
                aria-expanded={isExpanded}
                aria-controls={`help-content-${section.id}`}
              >
                <span className="help-section-icon" aria-hidden="true">
                  {section.icon}
                </span>
                <h3>{section.title}</h3>
                <span className="help-section-chevron" aria-hidden="true">
                  {isExpanded ? '▼' : '▶'}
                </span>
              </button>
              {isExpanded && (
                <div id={`help-content-${section.id}`} className="help-section-body">
                  <div className="help-content">
                    {section.blocks.map((block, i) => renderBlock(block, i))}
                  </div>
                </div>
              )}
            </section>
          );
        })}
      </main>

      <footer className="help-footer">
        <p>{t('help.footer')}</p>
      </footer>
    </div>
  );
}
