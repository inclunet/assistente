import { useEffect, useRef, useState } from 'react';
import { Outlet, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { GetConfig, RespondQuestionnaire, NeedsWelcomeWizard, RunWelcomeWizard } from "@wailsjs/go/app/App";
import { EventsOn } from "@wailsjs/runtime/runtime";
import { useSettingsStore } from './store/settingsStore';
import { useUIStore } from './store/uiStore';
import { useChatStore } from './store/chatStore';
import { parseDeepLink, executeDeepLink } from './lib/deepLinks';
import { ScreenReaderAnnouncer } from './components/ui/ScreenReaderAnnouncer';
import { ConfirmHost } from './components/ui/ConfirmHost';
import { QuestionnaireDialog, QuestionnairePayload } from './components/ui/QuestionnaireDialog';
import { useQuestionnaireUIStore } from './store/questionnaireUIStore';
import { useTheme } from './hooks/useTheme';
import { ConfigProvider } from 'antd';
import type { Locale } from 'antd/es/locale';
import { getAntdTheme } from './theme/antdTheme';
import { waitForWailsBridge } from './lib/waitForWailsBridge';

function useAntdLocale(lang: string): Locale | undefined {
    const [locale, setLocale] = useState<Locale | undefined>(undefined);
    useEffect(() => {
        let cancelled = false;
        const loaders: Record<string, () => Promise<{ default: Locale }>> = {
            'pt-BR': () => import('antd/locale/pt_BR'),
            'en':    () => import('antd/locale/en_US'),
            'es':    () => import('antd/locale/es_ES'),
        };
        const load = loaders[lang] ?? loaders['en'];
        void load().then((m) => { if (!cancelled) setLocale(m.default); });
        return () => { cancelled = true; };
    }, [lang]);
    return locale;
}

function App() {
    const { theme } = useTheme();
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const antLocale = useAntdLocale(i18n.language);
    const { setConfig, setLoading, setError } = useSettingsStore();
    const { addToast } = useUIStore();
    const { handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, handleExternalIncoming } = useChatStore();
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
        const controller = new AbortController();

        const loadConfig = async () => {
            await waitForWailsBridge({ signal: controller.signal });
            if (controller.signal.aborted) return;

            setLoading(true);
            try {
                // Verifica se precisa do wizard de boas-vindas
                const needsWizard = await NeedsWelcomeWizard();
                if (needsWizard) {
                    try {
                        const completed = await RunWelcomeWizard();
                        if (!completed) {
                            addToast(t('app.wizard.cancelled'), 'warning');
                        } else {
                            addToast(t('app.wizard.success'), 'success', 5000);
                        }
                    } catch (error) {
                        console.error('[App] Erro ao executar wizard:', error);
                        addToast(t('app.wizard.error'), 'error');
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
                const current = useSettingsStore.getState();
                setConfig({
                    apiKey: config.api_key || '',
                    baseURL: config.api_base_url || 'https://api.openai.com/v1',
                    defaultModel: config.chat_params?.model || config.default_model || 'gpt-4',
                    temperature: config.chat_params?.temperature || 0.7,
                    maxTokens: config.chat_params?.max_tokens || 2000,
                    streamEnabled: config.chat_params?.stream ?? true,
                    theme: current.config?.theme || 'assistente',
                    language: current.config?.language || 'pt-BR',
                });
                addToast(t('app.config.loaded'), 'success', 3000);
            } catch (error) {
                console.error('Erro ao carregar configuração:', error);
                setError(t('app.config.loadError'));
                addToast(t('app.config.loadError'), 'error');
            } finally {
                if (!controller.signal.aborted) {
                    setLoading(false);
                }
            }
        };

        void loadConfig().catch((error) => {
            if (error instanceof DOMException && error.name === 'AbortError') {
                return;
            }
            console.error('Erro ao carregar configuração:', error);
        });
        return () => controller.abort();
    }, [setConfig, setLoading, setError, addToast]);

    // Escuta eventos de conversa deletada/limpa
    useEffect(() => {
        const unsubs: Array<() => void> = [];

        unsubs.push(EventsOn('conversation:deleted', (data: unknown) => {
            const eventData = data as { conversation_id?: number };
            if (eventData.conversation_id) {
                handleConversationDeleted(eventData.conversation_id);
            }
        }));

        unsubs.push(EventsOn('conversation:cleared', (data: unknown) => {
            const eventData = data as { conversation_id?: number };
            if (eventData.conversation_id) {
                handleConversationCleared(eventData.conversation_id);
            }
        }));

        unsubs.push(EventsOn('conversation:renamed', (data: unknown) => {
            const eventData = data as { conversationId?: number; newTitle?: string };
            if (eventData.conversationId && eventData.newTitle) {
                handleConversationRenamed(eventData.conversationId, eventData.newTitle);
            }
        }));

        unsubs.push(EventsOn('database:reset', () => {
            handleDatabaseReset();
        }));

        unsubs.push(EventsOn('navigate:update', () => {
            navigate('/update');
        }));

        unsubs.push(EventsOn('deeplink:execute', (uri: unknown) => {
            if (typeof uri !== 'string') return;
            const action = parseDeepLink(uri);
            if (action) {
                void executeDeepLink(action, { navigate });
            }
        }));

        unsubs.push(EventsOn('chat:summary_started', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(t('app.summary.started', { count: eventData.messageCount ?? 0 }), 'info', 10000);
        }));

        unsubs.push(EventsOn('chat:summary_completed', (data: unknown) => {
            const eventData = data as { messageCount?: number };
            addToast(t('app.summary.completed', { count: eventData.messageCount ?? 0 }), 'success', 5000);
        }));

        unsubs.push(EventsOn('chat:summary_error', (data: unknown) => {
            const eventData = data as { error?: string };
            addToast(t('app.summary.error', { error: eventData.error || '' }), 'error');
        }));

        return () => {
            unsubs.forEach(fn => fn());
        };
    }, [handleConversationDeleted, handleConversationCleared, handleConversationRenamed, handleDatabaseReset, navigate]);

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
            };
            handleExternalIncoming({
                channel: eventData.channel || '',
                from: eventData.from || '',
                fromId: eventData.fromId || '',
                text: eventData.text || '',
                conversationId: eventData.conversationId || 0,
                newConversation: eventData.newConversation || false,
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
                setQuestionnaireOpen(false);
                setQuestionnaireData(null);
                restoreFocus();
            } catch (err) {
                console.error('[App] Erro ao enviar questionário:', err);
                addToast(t('app.questionnaire.submitError'), 'error');
            }
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
                setQuestionnaireOpen(false);
                setQuestionnaireData(null);
                restoreFocus();
            } catch (err) {
                console.error('[App] Erro ao cancelar questionário:', err);
                addToast(t('app.questionnaire.submitError'), 'error');
            }
            return;
        }

        // Questionário disparado pela UI
        uiCancel();
        restoreFocus();
    };

    return (
        <ConfigProvider theme={getAntdTheme(theme)} locale={antLocale}>
            <ScreenReaderAnnouncer />
            <Outlet />

            <ConfirmHost />

            <QuestionnaireDialog
                isOpen={effectiveQuestionnaireOpen}
                data={effectiveQuestionnaireData}
                onSubmit={handleQuestionnaireSubmit}
                onCancel={handleQuestionnaireCancel}
            />
        </ConfigProvider>
    )
}

export default App
