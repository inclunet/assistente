import './style.css'

// Chat components theming
import './components/chat/styles/tokens.css'     // Base design tokens
import './components/chat/adapters/assistente.css' // Maps --color-* to --chat-*

import { waitLocale } from './lib/i18n.js'
import App from './App.svelte'

// Aguarda o idioma carregar antes de montar a aplicação
waitLocale().then(() => {
  new App({
    target: document.getElementById('app')
  });
});
