import { useEffect, useState } from 'react';
import { Outlet } from 'react-router-dom';
import './App.css';
import { GetConfig, AuthorizeMessagingContact } from "../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime";
import { useSettingsStore } from './store/settingsStore';
import { useUIStore } from './store/uiStore';
import { useChatStore } from './store/chatStore';
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';
import { ConfirmDialog } from './components/ui/ConfirmDialog';

function App() {
    const { setConfig, setLoading, setError } = useSettingsStore();
    const { addToast } = useUIStore();
    const { initializeTabs, isInitialized, handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, handleTabClosed, handleExternalIncoming } = useChatStore();

    useEffect(() => {
        // Aguardar Wails estar pronto antes de carregar configuração
        const loadConfig = async () => {
            // Verificar se Wails está disponível
            if (typeof window === 'undefined' || !(window as any)['go']) {
                console.log('Aguardando Wails inicializar...');
                setTimeout(loadConfig, 100);
                return;
            }

            setLoading(true);
            try {
                const config: any = await GetConfig();
                setConfig({
                    apiKey: config.api_key || '',
                    baseURL: config.api_base_url || 'https://api.openai.com/v1',
                    defaultModel: config.chat_params?.model || config.default_model || 'gpt-4',
                    temperature: config.chat_params?.temperature || 0.7,
                    maxTokens: config.chat_params?.max_tokens || 2000,
                    streamEnabled: config.chat_params?.stream ?? true,
                    theme: 'system',
                    language: 'pt-BR',
                });
                addToast('Configuração carregada!', 'success', 3000);
            } catch (error) {
                console.error('Erro ao carregar configuração:', error);
                setError('Erro ao carregar configuração');
                addToast('Erro ao carregar configuração', 'error');
            } finally {
                setLoading(false);
            }
        };

        loadConfig();
    }, [setConfig, setLoading, setError, addToast]);

    // Inicializa tabs do backend
    useEffect(() => {
        console.log('===== [App] useEffect EXECUTANDO =====');
        console.log('[App] isInitialized:', isInitialized);
        console.log('[App] initializeTabs:', typeof initializeTabs);
        
        if (!isInitialized) {
            console.log('[App] ===== CHAMANDO initializeTabs =====');
            initializeTabs();
        } else {
            console.log('[App] ===== JÁ INICIALIZADO, PULANDO =====');
        }
    }, [initializeTabs, isInitialized]);

    // Escuta eventos de conversa deletada/limpa para atualizar tabs
    useEffect(() => {
        console.log('[App] 📡 Registrando listeners de eventos de conversa...');
        
        EventsOn('conversation:deleted', (data: any) => {
            console.log('[App] 🗑️ EVENTO conversation:deleted RECEBIDO:', data);
            if (data.conversation_id) {
                console.log('[App] 🗑️ Chamando handleConversationDeleted com:', data.conversation_id);
                handleConversationDeleted(data.conversation_id);
            } else {
                console.log('[App] 🗑️ ERRO: conversation_id não encontrado no evento!');
            }
        });

        EventsOn('conversation:cleared', (data: any) => {
            console.log('[App] 🧹 EVENTO conversation:cleared RECEBIDO:', data);
            if (data.conversation_id) {
                console.log('[App] 🧹 Chamando handleConversationCleared com:', data.conversation_id);
                handleConversationCleared(data.conversation_id);
            } else {
                console.log('[App] 🧹 ERRO: conversation_id não encontrado no evento!');
            }
        });

        EventsOn('conversation:renamed', (data: any) => {
            console.log('[App] Conversation renamed event received:', data);
            if (data.conversation_id && data.new_title) {
                handleConversationRenamed(data.conversation_id, data.new_title);
            }
        });

        EventsOn('database:reset', () => {
            console.log('[App] Database reset event received');
            handleDatabaseReset();
        });

        EventsOn('tab_closed', (data: any) => {
            console.log('[App] Tab closed event received:', data);
            if (data.id) {
                handleTabClosed(data.id);
            }
        });

        return () => {
            EventsOff('conversation:deleted');
            EventsOff('conversation:cleared');
            EventsOff('conversation:renamed');
            EventsOff('database:reset');
            EventsOff('tab_closed');
        };
    }, [handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, handleTabClosed]);

    // Listener para mensagens de canais externos (Signal, Telegram).
    // Quando messaging:incoming chega, delega ao chatStore que monta placeholders
    // e registra listeners de streaming (chat:stream, chat:done, etc.) — mesmo fluxo
    // do sendMessage, com som, TTS, announcer e streaming em tempo real.
    useEffect(() => {
        const unsubIncoming = EventsOn('messaging:incoming', (data: any) => {
            console.log('[App] messaging:incoming — delegando ao chatStore');
            handleExternalIncoming({
                channel: data.channel || '',
                from: data.from || '',
                fromId: data.fromId || '',
                text: data.text || '',
                conversationId: data.conversationId || 0,
                newConversation: data.newConversation || false,
                tabId: data.tabId || 0,
                tabCreated: data.tabCreated || false,
                tabTitle: data.tabTitle || '',
                tabIcon: data.tabIcon || '',
            });
        });

        return () => {
            unsubIncoming();
        };
    }, [handleExternalIncoming]);

    // Estado do dialog de autorização de contato (mensageria)
    const [contactAuthOpen, setContactAuthOpen] = useState(false);
    const [contactAuthData, setContactAuthData] = useState<{
        channel: string; displayName: string; contactId: string; username: string;
    } | null>(null);

    useEffect(() => {
        EventsOn('messaging:contact_blocked', (data: any) => {
            // Se já tem um dialog aberto, ignora (evita empilhar)
            setContactAuthData((prev) => {
                if (prev) return prev;
                return {
                    channel: data.channel || '',
                    displayName: data.displayName || '',
                    contactId: data.contactId || '',
                    username: data.username || '',
                };
            });
            setContactAuthOpen(true);
        });

        return () => {
            EventsOff('messaging:contact_blocked');
        };
    }, []);

    const handleAuthorizeContact = async () => {
        if (contactAuthData) {
            const identifier = contactAuthData.contactId || contactAuthData.username;
            const name = contactAuthData.displayName || identifier;
            try {
                await AuthorizeMessagingContact(contactAuthData.channel, identifier);
                addToast(`Contato ${name} autorizado no ${contactAuthData.channel}!`, 'success');
            } catch (err: any) {
                addToast(err.message || 'Erro ao autorizar contato', 'error');
            }
        }
        setContactAuthOpen(false);
        setContactAuthData(null);
    };

    const handleDenyContact = () => {
        setContactAuthOpen(false);
        setContactAuthData(null);
    };

    // Previne menu de contexto nativo quando tecla ContextMenu ou Shift+F10 for pressionada
    useEffect(() => {
        const preventNativeContextMenu = (e: KeyboardEvent) => {
            if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                e.preventDefault();
                e.stopPropagation();
            }
        };

        document.addEventListener('keydown', preventNativeContextMenu, true);
        return () => document.removeEventListener('keydown', preventNativeContextMenu, true);
    }, []);

    // Monta a mensagem descritiva para o dialog de autorização
    const contactAuthMessage = contactAuthData
        ? `O contato ${contactAuthData.displayName || 'desconhecido'} enviou uma mensagem via ${contactAuthData.channel}, mas não está na lista de contatos autorizados.\n\nIdentificador: ${contactAuthData.contactId || contactAuthData.username}\n\nDeseja autorizar este contato?`
        : '';

    return (
        <>
            <ScreenReaderAnnouncer />
            <Outlet />

            {/* Dialog de autorização de contato (mensageria) */}
            <ConfirmDialog
                isOpen={contactAuthOpen}
                title="Contato não autorizado"
                message={contactAuthMessage}
                confirmText="Autorizar"
                cancelText="Ignorar"
                variant="warning"
                onConfirm={handleAuthorizeContact}
                onCancel={handleDenyContact}
            />
        </>
    )
}

export default App
