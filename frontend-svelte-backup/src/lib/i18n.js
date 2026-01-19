/**
 * Configuração do svelte-i18n
 * 
 * Uso nos componentes:
 *   import { _ } from 'svelte-i18n';
 *   <button>{$_('chat.copy')}</button>
 * 
 * Mudar idioma:
 *   import { locale } from 'svelte-i18n';
 *   locale.set('pt-BR');
 */

import { register, init, getLocaleFromNavigator, waitLocale } from 'svelte-i18n';

// Registra os idiomas disponíveis (lazy loading)
register('en', () => import('./locales/en.json'));
register('pt-BR', () => import('./locales/pt-BR.json'));
register('es', () => import('./locales/es.json'));

// Inicializa com inglês como padrão
init({
  fallbackLocale: 'en',
  initialLocale: getLocaleFromNavigator() || 'en',
});

// Exporta função para aguardar o carregamento
export { waitLocale };

