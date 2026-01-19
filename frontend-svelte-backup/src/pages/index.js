// ====================
// Pages
// ====================
// 
// Páginas da aplicação. Cada página pode ter seus componentes
// específicos organizados em subpastas.
//
// Estrutura:
//   pages/
//   ├── chat/       - Página principal de chat
//   ├── history/    - Histórico de conversas
//   ├── agents/     - Gerenciamento de agentes (HTTP, MCP, File)
//   ├── settings/   - Configurações da aplicação
//   ├── faq/        - Gerenciamento de FAQs
//   ├── memory/     - Gerenciamento de memórias
//   └── oauth/      - Gerenciamento de OAuth
//

// Páginas principais
export { Chat } from './chat';
export { ConversationList } from './history';
export { AgentManager } from './agents';
export { Settings } from './settings';
export { FAQManager } from './faq';
export { MemoryManager } from './memory';
export { OAuthManager } from './oauth';

