import { useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import './App.css';
import { GetConfig } from "../wailsjs/go/main/App";
import { useSettingsStore } from './store/settingsStore';
import { useUIStore } from './store/uiStore';
import { ToastContainer } from './components/ui/Toast';

function App() {
    const { i18n } = useTranslation();
    const { setConfig, setLoading, setError } = useSettingsStore();
    const { addToast } = useUIStore();

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

    return (
        <>
            <Outlet />
            <ToastContainer />
        </>
    )
}

export default App
