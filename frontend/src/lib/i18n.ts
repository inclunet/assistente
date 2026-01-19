import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

// Traduções iniciais (expandir depois)
const resources = {
  'pt-BR': {
    translation: {
      common: {
        loading: 'Carregando...',
        error: 'Erro',
        success: 'Sucesso',
        cancel: 'Cancelar',
        save: 'Salvar',
        delete: 'Excluir',
        edit: 'Editar',
        close: 'Fechar',
        confirm: 'Confirmar',
      },
      settings: {
        title: 'Configurações',
        apiKey: 'Chave da API',
        baseURL: 'URL Base',
        model: 'Modelo',
        temperature: 'Temperatura',
        maxTokens: 'Máximo de Tokens',
        theme: 'Tema',
        language: 'Idioma',
      },
      chat: {
        title: 'Chat',
        placeholder: 'Digite sua mensagem...',
        send: 'Enviar',
        newChat: 'Novo Chat',
      },
    },
  },
  'en-US': {
    translation: {
      common: {
        loading: 'Loading...',
        error: 'Error',
        success: 'Success',
        cancel: 'Cancel',
        save: 'Save',
        delete: 'Delete',
        edit: 'Edit',
        close: 'Close',
        confirm: 'Confirm',
      },
      settings: {
        title: 'Settings',
        apiKey: 'API Key',
        baseURL: 'Base URL',
        model: 'Model',
        temperature: 'Temperature',
        maxTokens: 'Max Tokens',
        theme: 'Theme',
        language: 'Language',
      },
      chat: {
        title: 'Chat',
        placeholder: 'Type your message...',
        send: 'Send',
        newChat: 'New Chat',
      },
    },
  },
};

i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: 'pt-BR',
    fallbackLng: 'pt-BR',
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
