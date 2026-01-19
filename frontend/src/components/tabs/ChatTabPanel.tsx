/**
 * Painel de conteúdo da aba de chat
 * Renderiza o conteúdo da conversa para a aba ativa
 * TODO: Implementar ConversationController isolado por aba
 */

import { useTabsStore } from '../../store/tabsStore';

export function ChatTabPanel() {
  const { tabs, activeTabId } = useTabsStore();
  const activeTab = tabs.find(tab => tab.id === activeTabId);

  if (!activeTab) {
    return (
      <div className="chat-tab-panel--empty">
        <p>Nenhuma aba ativa</p>
      </div>
    );
  }

  return (
    <div
      id={`tabpanel-${activeTab.id}`}
      role="tabpanel"
      aria-labelledby={`tab-${activeTab.id}`}
      className="chat-tab-panel"
    >
      {/* 
        TODO: Implementar ConversationController isolado por aba
        Por enquanto, o conteúdo do chat vem direto do ChatPage
      */}
    </div>
  );
}
