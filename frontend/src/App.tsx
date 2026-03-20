import { useEffect, useRef, useState } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import './App.css';
import { GetConfig, RespondQuestionnaire, NeedsWelcomeWizard, RunWelcomeWizard } from "@wailsjs/go/main/App";
import { EventsOn, EventsOff } from "@wailsjs/runtime/runtime";
import { useSettingsStore } from './store/settingsStore';
import { useUIStore } from './store/uiStore';
import { useChatStore } from './store/chatStore';
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';
import { ConfirmHost } from './components/ui/ConfirmHost';
import { QuestionnaireDialog, QuestionnairePayload } from './components/ui/QuestionnaireDialog';
import { useQuestionnaireUIStore } from './store/questionnaireUIStore';
import { useTheme } from './hooks/useTheme';

function App() {
    useTheme();
    const navigate = useNavigate();
    const { setConfig, setLoading, setError } = useSettingsStore();
    const { addToast } = useUIStore();
    const { initializeTabs, isInitialized, handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleTabTitleUpdated, handleDatabaseReset, handleTabClosed, handleExternalIncoming } = useChatStore();
    const wasQuestionnaireOpenRef = useRef(false);
    const lastFocusedElementRef = useRef<HTMLElement | null>(null);

    // Estado do dialog de questionário (tool: collect_responses e aprovações)
    const [questionnaireOpen, setQuestionnaireOpen] = useState(false);
    const [questionnaireData, setQuestionnaireData] = useState<QuestionnairePayload | null>(null);

    // Questionários disparados pela UI (reusa o mesmo QuestionnaireDialog)
    const uiQuestionnaireData = useQuestionnaireUIStore((s) => s.active);
    const uiSubmit = useQuestionnaireUIStore((s) => s.submit);
    const uiCancel = useQuestionnaireUIStore((s) => s.cancel);

    useEffect(() => {
        // Aguardar Wails estar pronto antes de carregar configuração
        const loadConfig = async () => {
            // Verificar se Wails está disponível
            const wailsWindow = window as Window & { go?: unknown };
            if (typeof window === 'undefined' || !wailsWindow.go) {
                setTimeout(loadConfig, 100);
                return;
            }

            setLoading(true);
            try {
                // Verifica se precisa do wizard de boas-vindas
                const needsWizard = await NeedsWelcomeWizard();
                if (needsWizard) {
                    try {
                        const completed = await RunWelcomeWizard();
                        if (!completed) {
                            addToast('Configuração cancelada. Configure nas Configurações.', 'warning');
                        } else {
                            addToast('Configuração concluída com sucesso!', 'success', 5000);
                        }
                    } catch (error) {
                        console.error('[App] Erro ao executar wizard:', error);
                        addToast('Erro ao configurar. Verifique nas Configurações.', 'error');
                    }
                }

                const config = await GetConfig() as {
                    api_key?: string;
                    api_base_url?: string;
                    chat_params?: {
                        model?: string;
                        temperature?: number;
                        max_tokens?: number;
                        stream?: boolean;
                    };
                    default_model?: string;
                };
                setConfig({
                    apiKey: config.api_key || '',
                    baseURL: config.api_base_url || 'https://api.openai.com/v1',
                    defaultModel: config.chat_params?.model || config.default_model || 'gpt-4',
                    temperature: config.chat_params?.temperature || 0.7,
                    maxTokens: config.chat_params?.max_tokens || 2000,
                    streamEnabled: config.chat_params?.stream ?? true,
                    theme: 'assistente',
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

    // Restaura foco da janela no startup (resolve bug do Wails)
	// Ouve o evento de focus da window e delega para primeiro elemento focável
	useEffect(() => {
		const handleWindowFocus = () => {
			try {
				// Agenda o foco para o próximo ciclo de event loop
				// para garantir que o DOM está completamente renderizado
				requestAnimationFrame(() => {
					const activeElement = document.activeElement as HTMLElement;
					// Se o foco está no body ou html, delega para primeiro elemento focável
					if (!activeElement || activeElement === document.body || activeElement.tagName === 'HTML') {
						const focusableElements = document.querySelectorAll(
							'input, button, [tabindex]:not([tabindex="-1"]), textarea, select, a[href]'
						);
						if (focusableElements.length > 0) {
							(focusableElements[0] as HTMLElement).focus();
							console.warn('[App] Foco delegado para elemento focável');
						}
					}
				});
			} catch (err) {
				console.warn('[App] Erro ao delegar foco:', err);
			}
		};

		// Ouve quando a janela Wails recebe foco
		window.addEventListener('focus', handleWindowFocus);

		// Também tenta delegação imediata para elementos iniciais
		requestAnimationFrame(() => {
			const focusableElements = document.querySelectorAll(
				'input, button, [tabindex]:not([tabindex="-1"]), textarea, select, a[href]'
			);
			if (focusableElements.length > 0 && (document.activeElement === document.body || document.activeElement === null)) {
				(focusableElements[0] as HTMLElement).focus();
				console.warn('[App] Foco delegado no mount');
			}
		});

		return () => {
			window.removeEventListener('focus', handleWindowFocus);
		};
	}, []);

    // Inicializa tabs do backend
    useEffect(() => {
        if (!isInitialized) {
            initializeTabs();
        }
    }, [initializeTabs, isInitialized]);

    // Escuta eventos de conversa deletada/limpa para atualizar tabs
    useEffect(() => {
        EventsOn('conversation:deleted', (data: unknown) => {
            const eventData = data as { conversation_id?: number };
            if (eventData.conversation_id) {
                handleConversationDeleted(eventData.conversation_id);
            }
        });

        EventsOn('conversation:cleared', (data: unknown) => {
            const eventData = data as { conversation_id?: number };
            if (eventData.conversation_id) {
                handleConversationCleared(eventData.conversation_id);
            }
        });

        EventsOn('conversation:renamed', (data: unknown) => {
            const eventData = data as { conversation_id?: number; new_title?: string };
            if (eventData.conversation_id && eventData.new_title) {
                handleConversationRenamed(eventData.conversation_id, eventData.new_title);
            }
        });

        EventsOn('tab:title_updated', (data: unknown) => {
            const eventData = data as { tab_id?: string; new_title?: string };
            if (eventData.tab_id && eventData.new_title) {
                const backendTabId = parseInt(eventData.tab_id, 10);
                if (!Number.isNaN(backendTabId)) {
                    handleTabTitleUpdated(backendTabId, eventData.new_title);
                }
            }
        });

        EventsOn('database:reset', () => {
            handleDatabaseReset();
        });

        EventsOn('tab_closed', (data: unknown) => {
            const eventData = data as { id?: string };
            if (eventData.id) {
                const backendTabId = parseInt(eventData.id, 10);
                if (!Number.isNaN(backendTabId)) {
                    handleTabClosed(backendTabId);
                }
            }
        });

        EventsOn('navigate:update', () => {
            navigate('/update');
        });

        EventsOn('chat:summary_started', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(`Sumarizando conversa (${eventData.messageCount ?? 0} mensagens)...`, 'info', 10000);
        });

        EventsOn('chat:summary_completed', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(`Resumo da conversa atualizado (${eventData.messageCount ?? 0} mensagens resumidas)`, 'success', 5000);
        });

        EventsOn('chat:summary_error', (data: unknown) => {
            const eventData = data as { error?: string };
            addToast(`Erro ao sumarizar conversa: ${eventData.error || ''}`, 'error');
        });

        return () => {
            EventsOff('conversation:deleted');
            EventsOff('conversation:cleared');
            EventsOff('conversation:renamed');
            EventsOff('tab:title_updated');
            EventsOff('database:reset');
            EventsOff('tab_closed');
            EventsOff('navigate:update');
            EventsOff('chat:summary_started');
            EventsOff('chat:summary_completed');
            EventsOff('chat:summary_error');
        };
    }, [handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleTabTitleUpdated, handleDatabaseReset, handleTabClosed, navigate]);

    // Listener para mensagens de canais externos (Signal, Telegram).
    // Quando messaging:incoming chega, delega ao chatStore que monta placeholders
    // e registra listeners de streaming (chat:stream, chat:done, etc.) — mesmo fluxo
    // do sendMessage, com som, TTS, announcer e streaming em tempo real.
    useEffect(() => {
        const unsubIncoming = EventsOn('messaging:incoming', (data: unknown) => {
            const eventData = data as {
                channel?: string;
                from?: string;
                fromId?: string;
                text?: string;
                conversationId?: number;
                newConversation?: boolean;
                tabId?: number;
                tabCreated?: boolean;
                tabTitle?: string;
                tabIcon?: string;
            };
            handleExternalIncoming({
                channel: eventData.channel || '',
                from: eventData.from || '',
                fromId: eventData.fromId || '',
                text: eventData.text || '',
                conversationId: eventData.conversationId || 0,
                newConversation: eventData.newConversation || false,
                tabId: eventData.tabId || 0,
                tabCreated: eventData.tabCreated || false,
                tabTitle: eventData.tabTitle || '',
                tabIcon: eventData.tabIcon || '',
            });
        });

        return () => {
            unsubIncoming();
        };
    }, [handleExternalIncoming]);

    useEffect(() => {
        const unsub = EventsOn('tool:questionnaire', (data: QuestionnairePayload) => {
            lastFocusedElementRef.current = document.activeElement as HTMLElement;
            setQuestionnaireData(data);
            setQuestionnaireOpen(true);
        });
        return unsub;
    }, []);

    // Quando um questionário da UI abre, captura o foco atual
    useEffect(() => {
        if (uiQuestionnaireData && !questionnaireOpen) {
            lastFocusedElementRef.current = document.activeElement as HTMLElement;
        }
    }, [uiQuestionnaireData, questionnaireOpen]);

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

    const restoreFocus = () => {
        requestAnimationFrame(() => {
            const el = lastFocusedElementRef.current;
            if (el && document.contains(el)) {
                el.focus();
                return;
            }
            const textarea = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement | null;
            textarea?.focus();
        });
    };

    const effectiveQuestionnaireOpen = questionnaireOpen || !!uiQuestionnaireData;
    const effectiveQuestionnaireData = questionnaireData || uiQuestionnaireData;

    // Restaura foco quando qualquer questionário fecha
    useEffect(() => {
        if (!effectiveQuestionnaireOpen && wasQuestionnaireOpenRef.current) {
            restoreFocus();
        }
        wasQuestionnaireOpenRef.current = effectiveQuestionnaireOpen;
    }, [effectiveQuestionnaireOpen]);

    const handleQuestionnaireSubmit = async (answers: Record<string, unknown>) => {
        if (questionnaireOpen && questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, answers, false);
            } catch (err) {
                console.error('[App] Erro ao enviar questionário:', err);
                addToast('Erro ao enviar questionário', 'error');
            }
            setQuestionnaireOpen(false);
            setQuestionnaireData(null);
            restoreFocus();
            return;
        }

        // Questionário disparado pela UI
        uiSubmit(answers);
        restoreFocus();
    };

    const handleQuestionnaireCancel = async () => {
        if (questionnaireOpen && questionnaireData) {
            try {
                await RespondQuestionnaire(questionnaireData.id, {}, true);
            } catch (err) {
                console.error('[App] Erro ao cancelar questionário:', err);
            }
            setQuestionnaireOpen(false);
            setQuestionnaireData(null);
            restoreFocus();
            return;
        }

        // Questionário disparado pela UI
        uiCancel();
        restoreFocus();
    };

    return (
        <>
            <ScreenReaderAnnouncer />
            <Outlet />

            <ConfirmHost />

            {/* Dialog de questionário global */}
            <QuestionnaireDialog
                isOpen={effectiveQuestionnaireOpen}
                data={effectiveQuestionnaireData}
                onSubmit={handleQuestionnaireSubmit}
                onCancel={handleQuestionnaireCancel}
            />
        </>
    )
}

export default App
