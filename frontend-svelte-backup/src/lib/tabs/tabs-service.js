let Wails = null;
let Loaded = false;
async function ensureWails() {
  if (Loaded) return;
  try {
    const api = await import('../../wailsjs/go/main/App.js');
    Wails = api;
  } catch (e) {
    console.warn('[TabsService] Funções Wails não disponíveis:', e);
    Wails = {};
  }
  Loaded = true;
}

export const TabsService = {
  async getTabs() {
    await ensureWails();
    return Wails?.GetTabs ? Wails.GetTabs() : { tabs: [], active_tab_id: null };
  },
  async createTab(title, icon) {
    await ensureWails();
    return Wails?.CreateTab ? Wails.CreateTab(title, icon) : null;
  },
  async closeTab(tabId) {
    await ensureWails();
    return Wails?.CloseTab ? Wails.CloseTab(tabId) : null;
  },
  async setActiveTab(tabId) {
    await ensureWails();
    return Wails?.SetActiveTab ? Wails.SetActiveTab(tabId) : null;
  },
  async loadConversationInTab(tabId, conversationId) {
    await ensureWails();
    return Wails?.LoadConversationInTab ? Wails.LoadConversationInTab(tabId, conversationId) : null;
  }
};
