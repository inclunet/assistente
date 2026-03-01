import { useEffect, useRef, useState } from 'react';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useEditorStore } from '../../store/editorStore';
import { useUIStore } from '../../store/uiStore';
import { EditorRenameFile } from '@wailsjs/go/main/App';
import { basenameFromPath } from '../../utils/path';
import './EditorTabs.css';

export function EditorTabs() {
  const { addToast } = useUIStore();
  const tabs = useEditorStore((s) => s.tabs);
  const activeTabId = useEditorStore((s) => s.activeTabId);
  const setActiveTab = useEditorStore((s) => s.setActiveTab);
  const closeTab = useEditorStore((s) => s.closeTab);
  const renameTab = useEditorStore((s) => s.renameTab);
  const setTabFilePath = useEditorStore((s) => s.setTabFilePath);

  const listRef = useRef<HTMLDivElement>(null);
  const editInputRef = useRef<HTMLInputElement>(null);
  const { announce } = useAnnouncer();

  const [editingTabId, setEditingTabId] = useState<string | null>(null);
  const [editingTitle, setEditingTitle] = useState('');

  useEffect(() => {
    if (!editingTabId) return;
    setTimeout(() => {
      editInputRef.current?.focus();
      editInputRef.current?.select();
    }, 10);
  }, [editingTabId]);

  const startRenaming = (tabId: string) => {
    const tab = tabs.find((t) => t.id === tabId);
    if (!tab) return;
    setEditingTabId(tabId);
    setEditingTitle(tab.filePath ? basenameFromPath(tab.filePath) : tab.title);
    announce(tab.filePath ? 'Renomeando arquivo. Enter confirma, Escape cancela.' : 'Editando título do documento. Enter confirma, Escape cancela.');
  };

  const cancelRenaming = () => {
    setEditingTabId(null);
    setEditingTitle('');
    announce('Edição cancelada');
  };

  const confirmRenaming = async (reason: 'enter' | 'blur') => {
    const tabIdToFocus = editingTabId;
    const nextTitle = editingTitle.trim();
    const tab = editingTabId ? tabs.find((t) => t.id === editingTabId) : null;

    if (!tab || !tabIdToFocus) {
      setEditingTabId(null);
      setEditingTitle('');
      return;
    }

    if (!nextTitle) {
      cancelRenaming();
      return;
    }

    // Renomeio de draft: só muda o título local
    if (!tab.filePath) {
      renameTab(tab.id, nextTitle);
      setEditingTabId(null);
      setEditingTitle('');
      announce(`Título alterado para: ${nextTitle}`);
      window.setTimeout(() => {
        const btn = listRef.current?.querySelector(`[data-tab-id="${tabIdToFocus}"]`) as HTMLButtonElement | null;
        btn?.focus();
      }, 10);
      return;
    }

    // Renomeio de arquivo no disco
    if (/[\\/]/.test(nextTitle)) {
      addToast('O nome não pode conter / ou \\.', 'error');
      announce('Nome inválido');
      if (reason === 'blur') cancelRenaming();
      return;
    }

    try {
      const oldPath = String(tab.filePath);
      const newPath = await EditorRenameFile(oldPath, nextTitle);
      const newBase = basenameFromPath(newPath);

      setTabFilePath(tab.id, newPath);
      renameTab(tab.id, newBase);

      window.dispatchEvent(
        new CustomEvent('assistente:file-renamed', {
          detail: { oldPath, newPath },
        })
      );

      setEditingTabId(null);
      setEditingTitle('');
      announce(`Arquivo renomeado para: ${newBase}`);

      window.setTimeout(() => {
        const btn = listRef.current?.querySelector(`[data-tab-id="${tabIdToFocus}"]`) as HTMLButtonElement | null;
        btn?.focus();
      }, 10);
    } catch (e: any) {
      const msg = String(e?.message || e || 'Falha ao renomear arquivo');
      addToast(msg, 'error');
      announce('Falha ao renomear arquivo');
      // No blur: não prende o usuário em campo de edição.
      if (reason === 'blur') cancelRenaming();
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent, tabId: string) => {
    const currentIndex = tabs.findIndex((t) => t.id === tabId);
    if (currentIndex === -1) return;

    let nextIndex = currentIndex;
    let handled = false;

    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowUp':
        nextIndex = Math.max(0, currentIndex - 1);
        handled = true;
        break;
      case 'ArrowRight':
      case 'ArrowDown':
        nextIndex = Math.min(tabs.length - 1, currentIndex + 1);
        handled = true;
        break;
      case 'Home':
        nextIndex = 0;
        handled = true;
        break;
      case 'End':
        nextIndex = tabs.length - 1;
        handled = true;
        break;
      case 'Delete':
        event.preventDefault();
        closeTab(tabId);
        handled = true;
        break;
      case 'F2':
        event.preventDefault();
        startRenaming(tabId);
        handled = true;
        break;
      case 'Enter':
        // Enter: ir para o editor (mantém navegação na lista de abas sem “pular” por setas)
        if (!event.ctrlKey && !event.metaKey && !event.altKey) {
          event.preventDefault();
          setActiveTab(tabId);
          window.dispatchEvent(new Event('assistente:focus-editor'));
          announce('Foco no editor');
          handled = true;
        }
        break;
    }

    if (!handled) return;

    if (nextIndex !== currentIndex) {
      event.preventDefault();
      const next = tabs[nextIndex];
      if (!next) return;
      setActiveTab(next.id);
      const btn = listRef.current?.querySelector(`[data-tab-id="${next.id}"]`) as HTMLButtonElement | null;
      btn?.focus();
      announce(`${next.title}, ${nextIndex + 1} de ${tabs.length}`);
    }
  };

  return (
    <div className="editor-tabs" role="tablist" aria-label="Abas do editor" ref={listRef}>
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;
        const isEditing = tab.id === editingTabId;
        const titleText = tab.isDirty ? `${tab.title} *` : tab.title;

        return (
          <div key={tab.id} className={`editor-tabs__tab ${isActive ? 'is-active' : ''}`}>
            {isEditing ? (
              <input
                ref={editInputRef}
                className="editor-tabs__edit"
                value={editingTitle}
                onChange={(e) => setEditingTitle(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    void confirmRenaming('enter');
                  }
                  if (e.key === 'Escape') {
                    e.preventDefault();
                    cancelRenaming();
                  }
                }}
                onBlur={() => void confirmRenaming('blur')}
                aria-label="Editar título da aba"
              />
            ) : (
              <button
                className="editor-tabs__button"
                role="tab"
                aria-selected={isActive}
                tabIndex={isActive ? 0 : -1}
                data-tab-id={tab.id}
                onClick={() => {
                  setActiveTab(tab.id);
                  window.dispatchEvent(new Event('assistente:focus-editor'));
                }}
                onDoubleClick={() => startRenaming(tab.id)}
                onKeyDown={(e) => handleKeyDown(e, tab.id)}
                title={titleText}
              >
                <span className="editor-tabs__title">{titleText}</span>
              </button>
            )}

            <button
              className="editor-tabs__close"
              onClick={() => {
                closeTab(tab.id);
                window.dispatchEvent(new Event('assistente:focus-editor'));
              }}
              aria-label={`Fechar ${tab.title}`}
              title="Fechar"
              tabIndex={-1}
            >
              ×
            </button>
          </div>
        );
      })}
    </div>
  );
}
