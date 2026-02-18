import { useEffect, useRef, useState } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import './App.css';
import { GetConfig, RespondQuestionnaire } from "../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime";
import { useSettingsStore } from './store/settingsStore';
import { useUIStore } from './store/uiStore';
import { useChatStore } from './store/chatStore';
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';
import { QuestionnaireDialog, QuestionnairePayload } from './components/ui/QuestionnaireDialog';

function App() {
    const navigate = useNavigate();
    const { setConfig, setLoading, setError } = useSettingsStore();
    const { addToast } = useUIStore();
    const { initializeTabs, isInitialized, handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, handleTabClosed, handleExternalIncoming } = useChatStore();
    const wasQuestionnaireOpenRef = useRef(false);

    // Estado do dialog de questionário (tool: collect_responses e aprovações)
    const [questionnaireOpen, setQuestionnaireOpen] = useState(false);
    const [questionnaireData, setQuestionnaireData] = useState<QuestionnairePayload | null>(null);

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

        EventsOn('navigate:update', () => {
            console.log('[App] navigate:update event received');
            navigate('/update');
        });

        return () => {
            EventsOff('conversation:deleted');
            EventsOff('conversation:cleared');
            EventsOff('conversation:renamed');
            EventsOff('database:reset');
            EventsOff('tab_closed');
            EventsOff('navigate:update');
        };
    }, [handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, handleTabClosed, navigate]);

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

    useEffect(() => {
        const unsub = EventsOn('tool:questionnaire', (data: QuestionnairePayload) => {
            setQuestionnaireData(data);
            setQuestionnaireOpen(true);
        });
        return unsub;
    }, []);

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

    const focusChatInput = () => {
        requestAnimationFrame(() => {
            const textarea = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement | null;
            textarea?.focus();
        });
    };

    // Restaura foco quando o questionário fecha
    useEffect(() => {
        if (!questionnaireOpen && wasQuestionnaireOpenRef.current) {
            focusChatInput();
        }
        wasQuestionnaireOpenRef.current = questionnaireOpen;
    }, [questionnaireOpen]);

    const handleQuestionnaireSubmit = async (answers: Record<string, any>) => {
        if (questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, answers, false);
            } catch (err) {
                console.error('[App] Erro ao enviar questionário:', err);
                addToast('Erro ao enviar questionário', 'error');
            }
        }
        setQuestionnaireOpen(false);
        setQuestionnaireData(null);
        focusChatInput();
    };

    const handleQuestionnaireCancel = async () => {
        if (questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, {}, true);
            } catch (err) {
                console.error('[App] Erro ao cancelar questionário:', err);
            }
        }
        setQuestionnaireOpen(false);
        setQuestionnaireData(null);
        focusChatInput();
    };

    return (
        <>
            <ScreenReaderAnnouncer />
            <Outlet />

            {/* Dialog de questionário global */}
            <QuestionnaireDialog
                isOpen={questionnaireOpen}
                data={questionnaireData}
                onSubmit={handleQuestionnaireSubmit}
                onCancel={handleQuestionnaireCancel}
            />
        </>
    )
}

export default App
