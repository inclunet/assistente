import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useEditorStore } from '../../store/editorStore';
import { useUIStore } from '../../store/uiStore';
import { EditorRenameFile } from '@wailsjs/go/main/App';
import { basenameFromPath } from '../../utils/path';
import { Tabs, Tab, TabList } from '../ui/tabs';
import './EditorTabs.css';

export function EditorTabs() {
  const { t } = useTranslation();
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

  const focusTabButton = useCallback((tabId: string) => {
    const list = listRef.current;
    if (!list) return;
    const btn = list.querySelector(
      `button[role="tab"][data-tab-value="${CSS.escape(tabId)}"]`
    ) as HTMLButtonElement | null;
    btn?.focus();
  }, []);

  const pendingFocusTabIdRef = useRef<string | null>(null);

  useEffect(() => {
    const id = pendingFocusTabIdRef.current;
    if (!id) return;
    pendingFocusTabIdRef.current = null;

    const timer = window.setTimeout(() => {
      focusTabButton(id);
    }, 50);

    return () => window.clearTimeout(timer);
  }, [focusTabButton, tabs]);

  const startRenaming = (tabId: string) => {
    const tab = tabs.find((t) => t.id === tabId);
    if (!tab) return;
    setEditingTabId(tabId);
    setEditingTitle(tab.filePath ? basenameFromPath(tab.filePath) : tab.title);
    announce(tab.filePath ? t('editor.tabs.renaming') : t('editor.tabs.renamingDoc'));
  };

  const cancelRenaming = () => {
    setEditingTabId(null);
    setEditingTitle('');
    announce(t('editor.tabs.editCancelled'));
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
      announce(`${t('editor.tabs.titleChanged')} ${nextTitle}`);
      window.setTimeout(() => {
        focusTabButton(tabIdToFocus);
      }, 10);
      return;
    }

    // Renomeio de arquivo no disco
    if (/[\\/]/.test(nextTitle)) {
      addToast(t('editor.tabs.invalidName'), 'error');
      announce(t('editor.tabs.invalidNameLabel'));
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
      announce(`${t('editor.tabs.fileRenamed')} ${newBase}`);

      window.setTimeout(() => {
        focusTabButton(tabIdToFocus);
      }, 10);
    } catch (e: any) {
      const msg = String(e?.message || e || 'Falha ao renomear arquivo');
      addToast(msg, 'error');
      announce(t('editor.tabs.renameFailed'));
      // No blur: não prende o usuário em campo de edição.
      if (reason === 'blur') cancelRenaming();
    }
  };

  const requestClose = useCallback(
    (tabId: string, options?: { focusEditor?: boolean }) => {
      const currentIndex = tabs.findIndex((t) => t.id === tabId);
      if (currentIndex === -1) return;

      const focusEditor = options?.focusEditor ?? false;

      if (tabs.length > 1) {
        const nextFocusIndex = currentIndex < tabs.length - 1 ? currentIndex : currentIndex - 1;
        pendingFocusTabIdRef.current = tabs[nextFocusIndex]?.id ?? null;
      } else {
        pendingFocusTabIdRef.current = null;
      }

      if (activeTabId && tabId === activeTabId) {
        window.dispatchEvent(new Event('assistente:flush-rich-editor'));
      }

      closeTab(tabId);

      if (focusEditor) {
        window.dispatchEvent(new Event('assistente:focus-editor'));
      }
    },
    [activeTabId, closeTab, tabs]
  );

  const handleSelect = useCallback(
    (tabId: string) => {
      if (!tabId) return;
      window.dispatchEvent(new Event('assistente:flush-rich-editor'));
      setActiveTab(tabId);

      const idx = tabs.findIndex((t) => t.id === tabId);
      const tab = idx >= 0 ? tabs[idx] : null;
      if (tab) announce(`${tab.title}, ${idx + 1} de ${tabs.length}`);
    },
    [announce, setActiveTab, tabs]
  );

  const getFocusedTabId = useCallback(() => {
    const list = listRef.current;
    if (!list) return null;
    const focused = list.querySelector('button[role="tab"]:focus') as HTMLButtonElement | null;
    const v = focused?.getAttribute('data-tab-value');
    return v && v.trim() ? v : null;
  }, []);

  const handleListKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (editingTabId) return;
      const tabId = getFocusedTabId();
      if (!tabId) return;

      if (event.key === 'F2') {
        event.preventDefault();
        startRenaming(tabId);
        return;
      }

      if (event.key === 'Enter' && !event.ctrlKey && !event.metaKey && !event.altKey) {
        // Enter: ir para o editor.
        window.dispatchEvent(new Event('assistente:focus-editor'));
        announce(t('editor.tabs.focusEditor'));
      }
    },
    [announce, editingTabId, getFocusedTabId]
  );

  const handleDelete = useCallback(
    (tabId: string) => {
      requestClose(tabId);
    },
    [requestClose]
  );

  return (
    <Tabs
      value={activeTabId ?? ''}
      onValueChange={handleSelect}
      idBase="editor"
      onDelete={handleDelete}
      pageJump={10}
      activationMode="auto"
    >
      <TabList
        ariaLabel={t('editor.tabs.tabsLabel')}
        className="editor-tabs"
        listRef={listRef}
        onKeyDown={handleListKeyDown}
      >
        {tabs.map((tab) => {
          const isActive = tab.id === activeTabId;
          const isEditing = tab.id === editingTabId;
          const titleText = tab.isDirty ? `${tab.title} *` : tab.title;

          return (
            <div
              key={tab.id}
              className={`editor-tabs__tab${isActive ? ' is-active' : ''}`}
              role="presentation"
            >
              <div className="editor-tabs__main">
                <Tab
                  value={tab.id}
                  className="editor-tabs__button"
                  title={titleText}
                  controlsId={null}
                  onClick={() => {
                    window.dispatchEvent(new Event('assistente:focus-editor'));
                  }}
                  onDoubleClick={() => startRenaming(tab.id)}
                >
                  <span className="editor-tabs__title">{titleText}</span>
                </Tab>

                {isEditing && (
                  <input
                    ref={editInputRef}
                    className="editor-tabs__edit"
                    value={editingTitle}
                    onChange={(e) => setEditingTitle(e.target.value)}
                    onKeyDown={(e) => {
                      // Evita que o TabList capture setas/Home/End/Delete enquanto edita.
                      e.stopPropagation();

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
                    aria-label={t('editor.tabs.editTitle')}
                  />
                )}
              </div>

              <button
                className="editor-tabs__close"
                onClick={() => {
                  requestClose(tab.id, { focusEditor: true });
                }}
                aria-label={`${t('editor.tabs.close')} ${tab.title}`}
                title={t('editor.tabs.close')}
                tabIndex={-1}
                type="button"
              >
                ×
              </button>
            </div>
          );
        })}
      </TabList>
    </Tabs>
  );
}
