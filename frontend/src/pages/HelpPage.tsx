import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ApiOutlined,
  AudioOutlined,
  BulbOutlined,
  ConsoleSqlOutlined,
  ExportOutlined,
  FolderOutlined,
  HistoryOutlined,
  InteractionOutlined,
  KeyOutlined,
  MessageOutlined,
  MobileOutlined,
  PaperClipOutlined,
  ReadOutlined,
  SafetyOutlined,
  SaveOutlined,
  SettingOutlined,
  SoundOutlined,
  ToolOutlined,
  UserSwitchOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import './HelpPage.css';

interface HelpSection {
  id: string;
  title: string;
  icon: ReactNode;
  content: ReactNode;
}

export default function HelpPage() {
  const { t } = useTranslation();
  useContentPageLandmarks({ pageClass: 'help-page' });
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['commands']));

  const toggleSection = (id: string) => {
    setExpandedSections((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const expandAll = () => {
    setExpandedSections(new Set(sections.map((s) => s.id)));
  };

  const collapseAll = () => {
    setExpandedSections(new Set());
  };

  const sections: HelpSection[] = [
    {
      id: 'commands',
      title: 'Comandos por Chat e Voz',
      icon: <SoundOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <p>
            O grande diferencial do Assistente é que você pode <strong>pedir ações diretamente no chat</strong>,
            seja digitando ou falando. A IA entende comandos em linguagem natural e executa as ações
            diretamente.
          </p>

          <h4>Por que usar comandos no chat?</h4>
          <ul>
            <li>Não precisa decorar teclas de atalho ou navegar por menus</li>
            <li>Funciona tanto digitando quanto por voz</li>
            <li>Comandos em português ou inglês são entendidos</li>
            <li>A IA confirma as ações antes de executar quando necessário</li>
          </ul>

          <h4>Gerenciamento de Conversas</h4>
          <ul>
            <li><code>"Cria uma nova conversa"</code> / <code>"Nova aba"</code></li>
            <li><code>"Fecha essa aba"</code> / <code>"Fecha a aba atual"</code></li>
            <li><code>"Renomeia essa conversa para X"</code></li>
            <li><code>"Apaga essa conversa"</code> / <code>"Deleta essa conversa"</code></li>
            <li><code>"Limpa as mensagens dessa conversa"</code></li>
            <li><code>"Resume essa conversa"</code> / <code>"O que conversamos aqui?"</code></li>
            <li><code>"Vai para a aba X"</code> / <code>"Muda para outra aba"</code></li>
            <li><code>"Abre aquela conversa sobre Y"</code></li>
          </ul>

          <h4>Perfis de Voz e Interação</h4>
          <ul>
            <li><code>"Usa a voz do OpenAI nessa conversa"</code></li>
            <li><code>"Muda a velocidade da voz para mais rápido"</code></li>
            <li><code>"Cria um perfil de voz com voz feminina"</code></li>
            <li><code>"Lista os perfis de voz disponíveis"</code></li>
            <li><code>"Ativa o perfil de interação X"</code></li>
            <li><code>"Configura um hotkey Ctrl+Shift+V para gravar"</code></li>
          </ul>

          <h4>Dicas para Comandos por Voz</h4>
          <ul>
            <li>Fale de forma clara e natural</li>
            <li>Você pode combinar comandos: <code>"Cria uma nova conversa e usa a voz do OpenAI"</code></li>
            <li>A IA pede confirmação antes de ações destrutivas como deletar</li>
            <li>Se algo não funcionar, tente reformular o pedido</li>
          </ul>
        </div>
      ),
    },
    {
      id: 'overview',
      title: 'Visão Geral',
      icon: <ReadOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <p>
            O <strong>Assistente IA</strong> é uma aplicação desktop de inteligência artificial que
            permite conversas naturais com modelos de linguagem, suporte a voz e múltiplas abas
            para organizar suas conversas.
          </p>

          <h4>Destaque: Comandos por Chat e Voz</h4>
          <p>
            Você pode <strong>pedir ações diretamente no chat</strong>, digitando ou falando.
            Em vez de navegar por menus, simplesmente diga o que quer fazer:
          </p>
          <ul>
            <li><em>"Cria uma nova conversa"</em> - abre uma nova aba</li>
            <li><em>"Usa a voz do OpenAI"</em> - muda o perfil de voz</li>
            <li><em>"Renomeia essa conversa para X"</em> - renomeia a aba atual</li>
          </ul>
          <p>Veja a seção "Comandos por Chat e Voz" para mais exemplos.</p>

          <h4>Principais Recursos</h4>
          <ul>
            <li>
              <strong>Chat Inteligente:</strong> Conversas com IA usando modelos como GPT-4, com
              múltiplas abas simultâneas
            </li>
            <li>
              <strong>Voz:</strong> Fale com a IA e ouça as respostas. Suporte a múltiplos
              provedores de reconhecimento e síntese
            </li>
            <li>
              <strong>Terminal Integrado:</strong> Execute comandos do sistema diretamente no app
            </li>
            <li>
              <strong>MCP (Model Context Protocol):</strong> Conecte servidores externos para
              estender capacidades da IA
            </li>
            <li>
              <strong>Skills:</strong> Instruções especializadas para modificar comportamento da IA
            </li>
            <li>
              <strong>Channels:</strong> Integração com Telegram e Signal
            </li>
            <li>
              <strong>Allowlists:</strong> Controle de segurança sobre ferramentas da IA
            </li>
            <li>
              <strong>Acessibilidade:</strong> Navegação completa por teclado e suporte a leitores
              de tela
            </li>
          </ul>
        </div>
      ),
    },
    {
      id: 'chat',
      title: 'Chat e Conversas',
      icon: <MessageOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Sistema de Abas</h4>
          <p>
            O chat suporta múltiplas conversas simultâneas através de um sistema de abas. Cada aba
            pode ter sua própria conversa independente ou estar vinculada a uma conversa do
            histórico.
          </p>
          <ul>
            <li>
              <strong>Nova aba:</strong> Ctrl+T ou Ctrl+N
            </li>
            <li>
              <strong>Fechar aba:</strong> Ctrl+W ou Delete (quando focado na aba)
            </li>
            <li>
              <strong>Navegar entre abas:</strong> Ctrl+Tab (próxima) / Ctrl+Shift+Tab (anterior)
            </li>
            <li>
              <strong>Ir para aba específica:</strong> Ctrl+1 a Ctrl+9
            </li>
            <li>
              <strong>Renomear aba:</strong> F2 (quando focado na aba)
            </li>
          </ul>

          <h4>Threads e Mensagens</h4>
          <p>
            As mensagens são organizadas em uma estrutura hierárquica. O nível principal (raiz)
            contém suas mensagens e as respostas finais da IA. As threads contêm mensagens relacionadas
            e contexto adicional.
          </p>
          <ul>
            <li>
              <strong>Expandir thread:</strong> Seta Direita ou clique no indicador de interações
            </li>
            <li>
              <strong>Colapsar thread:</strong> Seta Esquerda ou Escape
            </li>
            <li>
              <strong>Navegar entre mensagens:</strong> Setas Cima/Baixo
            </li>
          </ul>

          <h4>Envio de Mensagens</h4>
          <ul>
            <li>
              <strong>Enviar:</strong> Enter
            </li>
            <li>
              <strong>Nova linha:</strong> Shift+Enter
            </li>
            <li>
              <strong>Anexar imagens:</strong> Ctrl+V (colar) ou arrastar arquivos
            </li>
          </ul>

          <h4>Funcionalidades do Chat</h4>
          <ul>
            <li>
              <strong>Streaming:</strong> As respostas da IA são exibidas em tempo real
            </li>
            <li>
              <strong>Menu de Contexto:</strong> Clique direito ou Shift+F10 para opções adicionais
            </li>
            <li>
              <strong>Detalhes da Mensagem:</strong> Enter na mensagem para ver detalhes completos
            </li>
          </ul>

          <h4>Barra de Ferramentas do Chat</h4>
          <ul>
            <li>
              <strong>Nova conversa:</strong> Ctrl+N
            </li>
            <li>
              <strong>Seletor de modelo:</strong> Ctrl+M
            </li>
            <li>
              <strong>Seletor de histórico:</strong> Ctrl+H
            </li>
            <li>
              <strong>Perfil de voz:</strong> Ctrl+P (quando voz habilitada)
            </li>
            <li>
              <strong>Perfil de interação:</strong> Ctrl+I (quando voz habilitada)
            </li>
          </ul>
        </div>
      ),
    },
    {
      id: 'voice',
      title: 'Voz e Áudio',
      icon: <AudioOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Interação por Voz</h4>
          <p>
            Você pode conversar com a IA usando voz, tanto para fazer perguntas quanto para dar
            comandos. Tudo que você pode digitar no chat, pode falar. A IA também pode responder
            por voz.
          </p>

          <h4>Como Usar</h4>
          <ol>
            <li>
              Clique no botão de microfone <AudioOutlined aria-hidden="true" /> ou use o hotkey
              configurado
            </li>
            <li>Fale sua mensagem ou comando</li>
            <li>A gravação para automaticamente (VAD) ou quando você soltar a tecla (PTT)</li>
            <li>Sua fala é convertida em texto e enviada</li>
            <li>A resposta pode ser lida em voz alta automaticamente</li>
          </ol>

          <h4>Exemplos de Comandos por Voz</h4>
          <ul>
            <li><em>"Cria uma nova conversa"</em></li>
            <li><em>"Usa a voz mais rápida"</em></li>
            <li><em>"Renomeia essa conversa para trabalho"</em></li>
            <li><em>"Ativa o perfil de interação Desktop"</em></li>
          </ul>

          <h4>Reconhecimento de Fala (STT)</h4>
          <ul>
            <li>
              <strong>Web Speech API:</strong> Reconhecimento em tempo real, gratuito, funciona
              enquanto você fala
            </li>
            <li>
              <strong>OpenAI Whisper:</strong> Alta precisão, processa após gravação, pago
            </li>
          </ul>

          <h4>Modos de Gravação</h4>
          <ul>
            <li>
              <strong>VAD:</strong> Detecta automaticamente quando você começa e para de falar.
              Mais natural para conversação.
            </li>
            <li>
              <strong>PTT:</strong> Segure o botão/tecla enquanto fala. Mais controle, evita
              capturas acidentais.
            </li>
            <li>
              <strong>Toggle:</strong> Pressione para iniciar, pressione novamente para parar.
            </li>
          </ul>

          <h4>Síntese de Voz (TTS)</h4>
          <ul>
            <li>
              <strong>OpenAI TTS:</strong> Vozes naturais de alta qualidade (alloy, echo, fable,
              onyx, nova, shimmer)
            </li>
            <li>
              <strong>Web Speech:</strong> Vozes do sistema, gratuito
            </li>
            <li>
              <strong>SAPI5 (Windows):</strong> Vozes nativas do Windows, funciona offline
            </li>
          </ul>

          <h4>Controles</h4>
          <ul>
            <li>
              <strong>Space na mensagem:</strong> Reproduzir/pausar leitura
            </li>
            <li>
              <strong>Botão de voz:</strong> Iniciar gravação
            </li>
            <li>
              <strong>Escape:</strong> Cancelar gravação
            </li>
            <li>
              <strong>Hotkey global:</strong> Gravar mesmo com app em segundo plano
            </li>
          </ul>

          <h4>Configuração</h4>
          <p>
            Veja a seção "Perfis de Voz e Interação" para configurar provedores, vozes, velocidade e
            modos de ativação.
          </p>
        </div>
      ),
    },
    {
      id: 'files',
      title: 'Arquivos e Documentos',
      icon: <FolderOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Enviando Arquivos no Chat (Upload)</h4>
          <p>
            Você pode enviar arquivos diretamente na conversa para a IA analisar. Há três formas de
            fazer isso:
          </p>
          <ul>
            <li>
              <strong>
                Botão Anexar (<PaperClipOutlined aria-hidden="true" />):
              </strong>{' '}
              Clique no botão ao lado do campo de mensagem e selecione os arquivos
            </li>
            <li>
              <strong>Colar (Ctrl+V):</strong> Copie uma imagem e cole diretamente no campo de
              mensagem
            </li>
            <li>
              <strong>Arrastar e soltar:</strong> Arraste arquivos do explorador para a área do
              chat. Uma sobreposição "Solte os arquivos aqui" aparecerá
            </li>
          </ul>

          <h4>Formatos Aceitos no Upload</h4>
          <ul>
            <li>
              <strong>Imagens:</strong> PNG, JPG, GIF, WebP - a IA pode "ver" e analisar o
              conteúdo visual
            </li>
            <li>
              <strong>Áudios:</strong> MP3, WAV, etc.
            </li>
            <li>
              <strong>Vídeos:</strong> MP4, WebM, etc.
            </li>
            <li>
              <strong>PDF:</strong> Documentos PDF
            </li>
            <li>
              <strong>Documentos:</strong> Word (DOC, DOCX), Excel (XLSX), texto (TXT), Markdown
              (MD), CSV
            </li>
          </ul>

          <h4>Como Funciona</h4>
          <ol>
            <li>Anexe o arquivo usando um dos métodos acima</li>
            <li>O arquivo aparece em preview acima do campo de mensagem</li>
            <li>Você pode remover arquivos clicando no X</li>
            <li>Digite uma pergunta ou instrução sobre o arquivo (opcional)</li>
            <li>Envie com Enter ou clique no botão</li>
          </ol>
          <p>
            <strong>Limite:</strong> Até 5 arquivos por mensagem.
          </p>

          <h4>Exemplos de Uso</h4>
          <ul>
            <li>Anexar uma imagem e perguntar <em>"O que você vê nessa imagem?"</em></li>
            <li>Anexar um PDF e perguntar <em>"Resume esse documento"</em></li>
            <li>Anexar uma planilha e pedir <em>"Analisa esses dados"</em></li>
            <li>Colar um screenshot e perguntar <em>"O que está errado aqui?"</em></li>
          </ul>
        </div>
      ),
    },
    {
      id: 'profiles',
      title: 'Perfis de Voz e Interação',
      icon: <UserSwitchOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que são Perfis?</h4>
          <p>
            Perfis permitem configurar como a IA fala com você (Perfil de Voz) e como você fala com
            a IA (Perfil de Interação). Você pode ter vários perfis e trocar entre eles conforme a
            situação.
          </p>

          <h4>Perfis de Voz (TTS - Como a IA fala)</h4>
          <p>Controlam a síntese de voz das respostas da IA:</p>
          <ul>
            <li>
              <strong>OpenAI:</strong> Vozes de alta qualidade (alloy, echo, fable, onyx, nova,
              shimmer). Pago, mas natural.
            </li>
            <li>
              <strong>Web Speech:</strong> Vozes do navegador/sistema. Gratuito, qualidade variável.
            </li>
            <li>
              <strong>SAPI5:</strong> Vozes do Windows. Gratuito, funciona offline.
            </li>
            <li>
              <strong>Desabilitado:</strong> Sem voz, apenas texto.
            </li>
          </ul>
          <p>Configurações ajustáveis:</p>
          <ul>
            <li>
              <strong>Velocidade (Rate):</strong> 0.5 (lento) a 2.0 (rápido), 1.0 é normal
            </li>
            <li>
              <strong>Tom (Pitch):</strong> 0.5 (grave) a 2.0 (agudo)
            </li>
            <li>
              <strong>Volume:</strong> 0 (mudo) a 1.0 (máximo)
            </li>
            <li>
              <strong>Auto-leitura:</strong> Ler automaticamente as respostas da IA
            </li>
          </ul>

          <h4>Perfis de Interação (STT - Como você fala)</h4>
          <p>Controlam o reconhecimento de voz e como ativar a gravação:</p>
          <ul>
            <li>
              <strong>Web Speech:</strong> Reconhecimento em tempo real pelo navegador. Gratuito.
            </li>
            <li>
              <strong>Whisper (OpenAI):</strong> Alta precisão, processa após gravação. Pago.
            </li>
          </ul>
          <p>Modos de ativação (triggers):</p>
          <ul>
            <li>
              <strong>VAD (Voice Activity Detection):</strong> Detecta automaticamente quando você
              começa e para de falar. Mais natural.
            </li>
            <li>
              <strong>PTT (Push-to-Talk):</strong> Segure uma tecla enquanto fala. Mais controle.
            </li>
            <li>
              <strong>Toggle:</strong> Pressione para iniciar, pressione novamente para parar.
            </li>
            <li>
              <strong>Wake Word:</strong> Diga uma palavra-chave para ativar (ex: "Assistente").
            </li>
          </ul>

          <h4>Hotkeys Globais</h4>
          <p>
            Perfis de interação podem ter hotkeys que funcionam mesmo quando o app está em segundo
            plano:
          </p>
          <ul>
            <li>
              <strong>Exemplo:</strong> Ctrl+Shift+Space traz a janela e ativa gravação VAD
            </li>
            <li>
              <strong>Exemplo:</strong> Ctrl+W liga/desliga detecção de wake word
            </li>
          </ul>
          <p>
            Configure hotkeys na página "Perfis de Interação" ou peça no chat:
            <code>"Configura um hotkey Ctrl+Alt+V para gravar"</code>
          </p>

          <h4>Perfis por Conversa</h4>
          <p>
            Cada conversa pode usar perfis diferentes. Use os seletores na barra de ferramentas do
            chat (Ctrl+P para voz, Ctrl+I para interação) ou peça no chat para aplicar um perfil.
          </p>

          <h4>Perfis Padrão</h4>
          <p>
            Em Configurações, defina quais perfis serão usados automaticamente em novas conversas.
          </p>
        </div>
      ),
    },
    {
      id: 'settings',
      title: 'Configurações',
      icon: <SettingOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>API</h4>
          <ul>
            <li>
              <strong>API Key:</strong> Sua chave de API da OpenAI (ou compatível)
            </li>
            <li>
              <strong>Base URL:</strong> URL da API (padrão: api.openai.com/v1)
            </li>
            <li>
              <strong>Testar Conexão:</strong> Valida a chave e lista modelos disponíveis
            </li>
          </ul>

          <h4>Modelo de Chat</h4>
          <ul>
            <li>
              <strong>Modelo:</strong> Modelo padrão para conversas (ex: gpt-4o-mini)
            </li>
            <li>
              <strong>Temperature:</strong> Criatividade das respostas (0-2)
            </li>
            <li>
              <strong>Max Tokens:</strong> Limite de tokens por resposta
            </li>
            <li>
              <strong>Top P:</strong> Diversidade de amostragem (0-1)
            </li>
          </ul>

          <h4>Perfis Padrão</h4>
          <ul>
            <li>
              <strong>Perfil de Voz:</strong> Perfil de TTS usado em novas conversas
            </li>
            <li>
              <strong>Perfil de Interação:</strong> Configurações de entrada e hotkeys para novas
              conversas
            </li>
          </ul>

          <h4>Zona de Perigo</h4>
          <ul>
            <li>
              <strong>Resetar Configurações:</strong> Restaura valores padrão
            </li>
            <li>
              <strong>Apagar Banco de Dados:</strong> Remove todos os dados permanentemente
            </li>
          </ul>

          <h4>Arquivo de Configuração</h4>
          <p>
            As configurações são salvas em <code>~/.assistente/config.json</code>
          </p>
        </div>
      ),
    },
    {
      id: 'history',
      title: 'Histórico',
      icon: <HistoryOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Gerenciamento de Conversas</h4>
          <p>
            O histórico mantém todas as suas conversas salvas. Cada conversa tem um título e pode
            ser acessada a qualquer momento.
          </p>

          <h4>Funcionalidades</h4>
          <ul>
            <li>
              <strong>Busca:</strong> Filtre conversas por título
            </li>
            <li>
              <strong>Edição:</strong> Renomeie conversas clicando duas vezes no título
            </li>
            <li>
              <strong>Exportar:</strong> Exporte conversas selecionadas para backup em JSON
            </li>
            <li>
              <strong>Importar:</strong> Importe conversas de arquivos de backup
            </li>
            <li>
              <strong>Excluir:</strong> Remova conversas individualmente ou em lote
            </li>
          </ul>

          <h4>Abrindo Conversas</h4>
          <p>
            Clique duas vezes em uma conversa para abri-la no chat. A conversa será carregada na
            aba atual, permitindo que você continue de onde parou.
          </p>

          <h4>Busca em Conversas Passadas</h4>
          <p>
            Você pode buscar conversas anteriores usando a busca na página de histórico. Digite
            palavras-chave relacionadas ao que você procura para encontrar conversas relevantes.
          </p>
        </div>
      ),
    },
    {
      id: 'terminal',
      title: 'Terminal Integrado',
      icon: <ConsoleSqlOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que é o Terminal?</h4>
          <p>
            O Assistente inclui um terminal integrado que permite executar comandos do sistema
            diretamente no aplicativo. É útil para tarefas de desenvolvimento, administração
            de sistema ou quando a IA precisa executar comandos para você.
          </p>

          <h4>Sistema de Sessões</h4>
          <p>
            O terminal suporta múltiplas sessões simultâneas, cada uma com seu próprio histórico
            e contexto de trabalho:
          </p>
          <ul>
            <li><strong>Nova sessão:</strong> Ctrl+T</li>
            <li><strong>Fechar sessão:</strong> Ctrl+W</li>
            <li><strong>Navegar entre sessões:</strong> Ctrl+Tab / Ctrl+Shift+Tab</li>
            <li><strong>Ir para sessão específica:</strong> Ctrl+1 a Ctrl+9</li>
          </ul>

          <h4>Executando Comandos</h4>
          <ol>
            <li>Digite o comando no campo de entrada</li>
            <li>Pressione Enter para executar</li>
            <li>A saída aparece no histórico acima</li>
            <li>Use Ctrl+C para interromper processos em execução</li>
          </ol>

          <h4>Histórico de Comandos</h4>
          <ul>
            <li>Navegue com Seta Cima/Baixo no campo de entrada</li>
            <li>Cada sessão mantém seu próprio histórico</li>
            <li>Comandos são salvos automaticamente</li>
          </ul>

          <h4>Integração com IA</h4>
          <p>
            A IA pode sugerir e executar comandos de terminal quando necessário. Você pode pedir
            no chat coisas como:
          </p>
          <ul>
            <li><em>"Lista os arquivos dessa pasta"</em></li>
            <li><em>"Executa npm install"</em></li>
            <li><em>"Mostra o conteúdo do arquivo X"</em></li>
          </ul>
        </div>
      ),
    },
    {
      id: 'mcp',
      title: 'MCP (Model Context Protocol)',
      icon: <ApiOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que é MCP?</h4>
          <p>
            O <strong>Model Context Protocol</strong> é um padrão aberto que permite conectar
            servidores externos que fornecem ferramentas e contexto adicional para a IA. Com MCP,
            você pode estender as capacidades do assistente conectando-o a diversas fontes de dados
            e serviços.
          </p>

          <h4>Servidores MCP</h4>
          <p>
            Servidores MCP são programas que expõem ferramentas que a IA pode usar. Exemplos:
          </p>
          <ul>
            <li><strong>Filesystem:</strong> Acesso a arquivos e pastas do sistema</li>
            <li><strong>Database:</strong> Consultas a bancos de dados</li>
            <li><strong>Web:</strong> Busca na web, fetch de URLs</li>
            <li><strong>APIs:</strong> Integração com serviços externos (GitHub, Slack, etc.)</li>
          </ul>

          <h4>Configurando Servidores</h4>
          <ol>
            <li>Vá para a página "MCP" no menu</li>
            <li>Clique em "Novo Servidor"</li>
            <li>Preencha os dados:
              <ul>
                <li><strong>Nome:</strong> Identificação do servidor</li>
                <li><strong>Transporte:</strong> stdio (processo local) ou SSE (HTTP)</li>
                <li><strong>Comando:</strong> Caminho do executável (para stdio)</li>
                <li><strong>Args:</strong> Argumentos do comando</li>
                <li><strong>URL:</strong> Endereço do servidor (para SSE)</li>
              </ul>
            </li>
            <li>Marque "Habilitado" e "Auto-conectar"</li>
            <li>Salve a configuração</li>
          </ol>

          <h4>Gerenciando Conexões</h4>
          <ul>
            <li><strong>Conectar:</strong> Inicia a conexão com o servidor</li>
            <li><strong>Desconectar:</strong> Encerra a conexão</li>
            <li><strong>Reconectar:</strong> Reinicia a conexão (útil após mudanças)</li>
            <li><strong>Status:</strong> Mostra se está conectado, desconectado ou com erro</li>
          </ul>

          <h4>Ferramentas Disponíveis</h4>
          <p>
            Quando um servidor MCP está conectado, suas ferramentas ficam disponíveis para a IA
            usar automaticamente. A coluna "Tools" mostra quantas ferramentas cada servidor fornece.
          </p>

          <h4>Exemplos de Uso</h4>
          <p>Com o servidor de filesystem conectado, você pode pedir:</p>
          <ul>
            <li><em>"Lista os arquivos da pasta Documents"</em></li>
            <li><em>"Lê o conteúdo do arquivo config.json"</em></li>
            <li><em>"Cria um arquivo README.md com..."</em></li>
          </ul>
        </div>
      ),
    },
    {
      id: 'skills',
      title: 'Skills (Habilidades)',
      icon: <BulbOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que são Skills?</h4>
          <p>
            Skills são <strong>instruções especializadas</strong> que você pode dar à IA para
            modificar seu comportamento ou dar contexto adicional. Elas são como "modos" ou
            "personalidades" que a IA pode assumir para tarefas específicas.
          </p>

          <h4>Como Funcionam</h4>
          <p>
            Cada skill contém:
          </p>
          <ul>
            <li><strong>Instruções:</strong> Texto que é adicionado ao contexto da IA</li>
            <li><strong>Ferramentas permitidas:</strong> Lista de tools que a skill pode usar</li>
            <li><strong>Invocação automática:</strong> Se a IA pode ativar a skill automaticamente</li>
          </ul>

          <h4>Tipos de Skills</h4>
          <ul>
            <li>
              <strong>Manuais:</strong> Você ativa explicitamente dizendo <code>"Use a skill X"</code>
            </li>
            <li>
              <strong>Automáticas:</strong> A IA decide quando usar baseado no contexto
            </li>
          </ul>

          <h4>Criando Skills</h4>
          <ol>
            <li>Vá para a página "Skills" no menu</li>
            <li>Clique em "Nova Skill"</li>
            <li>Preencha:
              <ul>
                <li><strong>Nome:</strong> Identificação da skill</li>
                <li><strong>Descrição:</strong> O que a skill faz</li>
                <li><strong>Conteúdo:</strong> As instruções que serão dadas à IA</li>
                <li><strong>Ferramentas:</strong> Lista de tools permitidas (opcional)</li>
                <li><strong>Auto:</strong> Se a IA pode ativar automaticamente</li>
              </ul>
            </li>
            <li>Salve a skill</li>
          </ol>

          <h4>Exemplos de Skills</h4>
          <ul>
            <li>
              <strong>Revisor de Código:</strong> Instrui a IA a revisar código em busca de bugs
              e sugerir melhorias
            </li>
            <li>
              <strong>Assistente de Escrita:</strong> Ajuda a escrever textos formais, e-mails,
              documentos
            </li>
            <li>
              <strong>Tradutor Técnico:</strong> Traduz mantendo terminologia técnica precisa
            </li>
            <li>
              <strong>Debugger:</strong> Analisa erros e sugere soluções
            </li>
          </ul>

          <h4>Usando Skills</h4>
          <p>Para usar uma skill, basta pedir no chat:</p>
          <ul>
            <li><code>"Usa a skill Revisor de Código"</code></li>
            <li><code>"Ativa o modo Assistente de Escrita"</code></li>
            <li><code>"Como um tradutor técnico, traduza isso..."</code></li>
          </ul>
        </div>
      ),
    },
    {
      id: 'allowlists',
      title: 'Allowlists (Listas de Permissões)',
      icon: <SafetyOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que são Allowlists?</h4>
          <p>
            Allowlists são <strong>regras de segurança</strong> que controlam quais ferramentas
            a IA pode usar e em quais condições. Elas protegem você de ações potencialmente
            perigosas ou indesejadas.
          </p>

          <h4>Como Funcionam</h4>
          <p>
            Cada allowlist define três categorias de ações:
          </p>
          <ul>
            <li>
              <strong>Auto-aprovar:</strong> Ferramentas que a IA pode usar livremente sem pedir
              permissão
            </li>
            <li>
              <strong>Confirmar:</strong> Ferramentas que requerem sua aprovação antes de executar
            </li>
            <li>
              <strong>Sempre negar:</strong> Ferramentas que nunca devem ser usadas
            </li>
          </ul>

          <h4>Ação Padrão</h4>
          <p>
            Para ferramentas não listadas explicitamente, a allowlist define uma ação padrão:
          </p>
          <ul>
            <li><strong>auto_approve:</strong> Permite usar livremente</li>
            <li><strong>confirm:</strong> Pede confirmação (recomendado)</li>
            <li><strong>deny:</strong> Nega automaticamente</li>
          </ul>

          <h4>Criando Allowlists</h4>
          <ol>
            <li>Vá para a página "Allowlists" no menu</li>
            <li>Clique em "Nova Allowlist"</li>
            <li>Preencha o nome e descrição</li>
            <li>Configure as regras:
              <ul>
                <li>Digite nomes de ferramentas separados por vírgula</li>
                <li>Ou use padrões como <code>read_*</code> para todas as ferramentas de leitura</li>
              </ul>
            </li>
            <li>Defina a ação padrão</li>
            <li>Salve a allowlist</li>
          </ol>

          <h4>Exemplos de Uso</h4>
          <ul>
            <li>
              <strong>Modo Seguro:</strong> Auto-aprovar apenas leitura, confirmar escrita, negar
              execução
            </li>
            <li>
              <strong>Desenvolvimento:</strong> Auto-aprovar operações de arquivo, confirmar git e
              npm
            </li>
            <li>
              <strong>Apenas Consulta:</strong> Auto-aprovar apenas leitura e busca, negar tudo
              que modifica
            </li>
          </ul>

          <h4>Vinculando a Perfis</h4>
          <p>
            Allowlists podem ser vinculadas a Perfis de Interação, aplicando as regras
            automaticamente quando o perfil está ativo.
          </p>
        </div>
      ),
    },
    {
      id: 'channels',
      title: 'Channels (Canais de Mensagens)',
      icon: <MobileOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>O que são Channels?</h4>
          <p>
            Channels permitem que o Assistente se conecte a <strong>aplicativos de mensagens</strong>
            como Telegram e Signal, permitindo que você converse com a IA diretamente nesses apps.
          </p>

          <h4>Canais Suportados</h4>
          <ul>
            <li>
              <strong>Telegram:</strong> Via Bot API - crie um bot com @BotFather
            </li>
            <li>
              <strong>Signal:</strong> Via signal-cli-rest-api - requer servidor local
            </li>
          </ul>

          <h4>Configurando Telegram</h4>
          <ol>
            <li>Crie um bot no Telegram com @BotFather</li>
            <li>Copie o token do bot</li>
            <li>Vá para "Channels" no menu</li>
            <li>Na aba "Canais", edite "Telegram"</li>
            <li>Cole o Bot Token</li>
            <li>Escolha um Perfil de Voz (opcional)</li>
            <li>Configure limites de histórico e contatos</li>
            <li>Marque "Habilitado" e salve</li>
            <li>Inicie uma conversa com o bot no Telegram</li>
          </ol>

          <h4>Configurando Signal</h4>
          <ol>
            <li>Instale e configure signal-cli-rest-api</li>
            <li>Registre um número no Signal (via API ou app)</li>
            <li>Vá para "Channels" no menu</li>
            <li>Na aba "Canais", edite "Signal"</li>
            <li>Configure a URL da API</li>
            <li>Selecione a conta registrada</li>
            <li>Escolha um Perfil de Voz (opcional)</li>
            <li>Marque "Habilitado" e salve</li>
          </ol>

          <h4>Gerenciamento de Contatos</h4>
          <p>
            Na aba "Contatos", você pode ver e gerenciar quem pode conversar com o assistente:
          </p>
          <ul>
            <li>Primeiro contato é aprovado automaticamente (configurável)</li>
            <li>Você pode remover contatos autorizados</li>
            <li>Limite de contatos por canal previne uso excessivo</li>
          </ul>

          <h4>Como Funciona</h4>
          <ol>
            <li>Alguém envia mensagem para o bot/número</li>
            <li>A mensagem chega ao Assistente</li>
            <li>Uma nova aba é criada automaticamente no chat</li>
            <li>A IA processa e responde</li>
            <li>A resposta é enviada de volta pelo canal</li>
          </ol>

          <h4>Recursos</h4>
          <ul>
            <li><strong>Conversas isoladas:</strong> Cada contato tem sua própria aba/conversa</li>
            <li><strong>Histórico limitado:</strong> Controle de contexto para economia de tokens</li>
            <li><strong>Perfis de voz:</strong> Respostas podem ser enviadas como áudio</li>
            <li><strong>Multi-conta:</strong> Suporte a múltiplas contas Signal</li>
          </ul>
        </div>
      ),
    },
    {
      id: 'keyboard',
      title: 'Teclas de Atalho',
      icon: <KeyOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Navegação Global</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Alt</kbd>+<kbd>M</kbd>
                </td>
                <td>Abrir/fechar menu de navegação</td>
              </tr>
              <tr>
                <td>
                  <kbd>F1</kbd>
                </td>
                <td>Abrir página de ajuda</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>F</kbd>
                </td>
                <td>Focar no campo de busca (em páginas com busca)</td>
              </tr>
            </tbody>
          </table>

          <h4>Gerenciamento de Abas</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>T</kbd> ou <kbd>Ctrl</kbd>+<kbd>N</kbd>
                </td>
                <td>Nova aba</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>W</kbd>
                </td>
                <td>Fechar aba atual</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Tab</kbd>
                </td>
                <td>Próxima aba</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Tab</kbd>
                </td>
                <td>Aba anterior</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>1</kbd> a <kbd>9</kbd>
                </td>
                <td>Ir para aba específica</td>
              </tr>
              <tr>
                <td>
                  <kbd>F2</kbd>
                </td>
                <td>Renomear aba (quando focado)</td>
              </tr>
              <tr>
                <td>
                  <kbd>Delete</kbd>
                </td>
                <td>Fechar aba (quando focado)</td>
              </tr>
            </tbody>
          </table>

          <h4>Barra de Ferramentas do Chat</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>N</kbd>
                </td>
                <td>Nova conversa</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>M</kbd>
                </td>
                <td>Seletor de modelo</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>H</kbd>
                </td>
                <td>Seletor de histórico</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>P</kbd>
                </td>
                <td>Seletor de perfil de voz</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>I</kbd>
                </td>
                <td>Seletor de perfil de interação</td>
              </tr>
            </tbody>
          </table>

          <h4>Terminal</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>T</kbd>
                </td>
                <td>Nova sessão de terminal</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>W</kbd>
                </td>
                <td>Fechar sessão atual</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Tab</kbd>
                </td>
                <td>Próxima sessão</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Tab</kbd>
                </td>
                <td>Sessão anterior</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>1</kbd> a <kbd>9</kbd>
                </td>
                <td>Ir para sessão específica</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>C</kbd>
                </td>
                <td>Interromper comando em execução</td>
              </tr>
              <tr>
                <td>
                  <kbd>↑</kbd> / <kbd>↓</kbd>
                </td>
                <td>Navegar histórico de comandos</td>
              </tr>
              <tr>
                <td>
                  <kbd>Escape</kbd>
                </td>
                <td>Voltar para o campo de entrada</td>
              </tr>
            </tbody>
          </table>

          <h4>Navegação de Mensagens</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>↑</kbd> / <kbd>↓</kbd>
                </td>
                <td>Navegar entre mensagens</td>
              </tr>
              <tr>
                <td>
                  <kbd>→</kbd>
                </td>
                <td>Expandir thread / focar primeiro filho</td>
              </tr>
              <tr>
                <td>
                  <kbd>←</kbd>
                </td>
                <td>Colapsar thread / voltar ao pai</td>
              </tr>
              <tr>
                <td>
                  <kbd>Home</kbd>
                </td>
                <td>Primeira mensagem do nível</td>
              </tr>
              <tr>
                <td>
                  <kbd>End</kbd>
                </td>
                <td>Última mensagem do nível</td>
              </tr>
              <tr>
                <td>
                  <kbd>Page Up</kbd> / <kbd>Page Down</kbd>
                </td>
                <td>Pular 10 mensagens</td>
              </tr>
              <tr>
                <td>
                  <kbd>Space</kbd>
                </td>
                <td>Reproduzir TTS da mensagem</td>
              </tr>
              <tr>
                <td>
                  <kbd>Enter</kbd>
                </td>
                <td>Ver detalhes da mensagem</td>
              </tr>
              <tr>
                <td>
                  <kbd>Escape</kbd>
                </td>
                <td>Voltar ao input</td>
              </tr>
              <tr>
                <td>
                  <kbd>Shift</kbd>+<kbd>F10</kbd>
                </td>
                <td>Menu de contexto</td>
              </tr>
            </tbody>
          </table>

          <h4>Input de Chat</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Enter</kbd>
                </td>
                <td>Enviar mensagem</td>
              </tr>
              <tr>
                <td>
                  <kbd>Shift</kbd>+<kbd>Enter</kbd>
                </td>
                <td>Nova linha</td>
              </tr>
              <tr>
                <td>
                  <kbd>↑</kbd> (no início)
                </td>
                <td>Navegar para mensagens</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>V</kbd>
                </td>
                <td>Colar imagens/arquivos</td>
              </tr>
            </tbody>
          </table>

          <h4>Menu e Pickers</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>↑</kbd> / <kbd>↓</kbd>
                </td>
                <td>Navegar entre itens</td>
              </tr>
              <tr>
                <td>
                  <kbd>Home</kbd> / <kbd>End</kbd>
                </td>
                <td>Primeiro/último item</td>
              </tr>
              <tr>
                <td>
                  <kbd>Page Up</kbd> / <kbd>Page Down</kbd>
                </td>
                <td>Pular 10 itens</td>
              </tr>
              <tr>
                <td>
                  <kbd>Enter</kbd>
                </td>
                <td>Selecionar item</td>
              </tr>
              <tr>
                <td>
                  <kbd>Escape</kbd>
                </td>
                <td>Fechar menu/picker</td>
              </tr>
              <tr>
                <td>
                  <kbd>Tab</kbd>
                </td>
                <td>Fechar e mover foco</td>
              </tr>
            </tbody>
          </table>

          <h4>DataGrid / Histórico / Gerenciamento de Dados</h4>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Atalho</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>↑</kbd> / <kbd>↓</kbd>
                </td>
                <td>Navegar entre linhas</td>
              </tr>
              <tr>
                <td>
                  <kbd>←</kbd> / <kbd>→</kbd>
                </td>
                <td>Navegar entre colunas</td>
              </tr>
              <tr>
                <td>
                  <kbd>Home</kbd> / <kbd>End</kbd>
                </td>
                <td>Primeira/última coluna</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Home</kbd> / <kbd>Ctrl</kbd>+<kbd>End</kbd>
                </td>
                <td>Primeira/última célula da grid</td>
              </tr>
              <tr>
                <td>
                  <kbd>Page Up</kbd> / <kbd>Page Down</kbd>
                </td>
                <td>Pular 10 linhas</td>
              </tr>
              <tr>
                <td>
                  <kbd>Enter</kbd>
                </td>
                <td>Ativar item (abrir/executar ação)</td>
              </tr>
              <tr>
                <td>
                  <kbd>F2</kbd>
                </td>
                <td>Editar célula (em colunas editáveis)</td>
              </tr>
              <tr>
                <td>
                  <kbd>Delete</kbd>
                </td>
                <td>Remover item selecionado</td>
              </tr>
              <tr>
                <td>
                  <kbd>Space</kbd>
                </td>
                <td>Marcar/desmarcar item</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Space</kbd>
                </td>
                <td>Toggle seleção do item atual</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>A</kbd>
                </td>
                <td>Selecionar todos os itens</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>C</kbd>
                </td>
                <td>Copiar conteúdo da célula focada</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>C</kbd>
                </td>
                <td>Copiar todas as linhas selecionadas (TSV)</td>
              </tr>
              <tr>
                <td>
                  <kbd>Escape</kbd>
                </td>
                <td>Limpar seleção</td>
              </tr>
            </tbody>
          </table>

          <h4>Hotkeys Globais (Configuráveis)</h4>
          <p>
            Hotkeys globais funcionam mesmo quando o app não está em foco. Configure-os nos Perfis
            de Interação.
          </p>
          <table className="help-shortcuts">
            <thead>
              <tr>
                <th>Exemplo</th>
                <th>Ação</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Space</kbd>
                </td>
                <td>Desktop Rápido (traz janela e ativa VAD)</td>
              </tr>
              <tr>
                <td>
                  <kbd>Ctrl</kbd>+<kbd>W</kbd>
                </td>
                <td>Toggle Wake Word (liga/desliga)</td>
              </tr>
            </tbody>
          </table>
        </div>
      ),
    },
    {
      id: 'accessibility',
      title: 'Acessibilidade',
      icon: <InteractionOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Navegação por Teclado</h4>
          <p>
            Toda a aplicação pode ser navegada usando apenas o teclado. Os principais atalhos estão
            listados na seção "Teclas de Atalho".
          </p>
          <ul>
            <li>
              <strong>Tab/Shift+Tab:</strong> Navegar entre elementos focáveis
            </li>
            <li>
              <strong>Enter/Space:</strong> Ativar botões e controles
            </li>
            <li>
              <strong>Setas:</strong> Navegar em listas, menus e mensagens
            </li>
            <li>
              <strong>Escape:</strong> Fechar modais, menus e cancelar ações
            </li>
          </ul>

          <h4>Leitores de Tela</h4>
          <p>O aplicativo implementa práticas de acessibilidade ARIA:</p>
          <ul>
            <li>
              <strong>Landmarks:</strong> Regiões semânticas para navegação rápida
            </li>
            <li>
              <strong>Live Regions:</strong> Anúncios automáticos de mudanças importantes
            </li>
            <li>
              <strong>Labels:</strong> Todos os controles têm labels descritivos
            </li>
            <li>
              <strong>Roles:</strong> Papéis ARIA para componentes interativos
            </li>
          </ul>

          <h4>Anunciador de Tela</h4>
          <p>O componente ScreenReaderAnnouncer fornece duas regiões aria-live:</p>
          <ul>
            <li>
              <strong>Polite:</strong> Para informações não urgentes (não interrompe)
            </li>
            <li>
              <strong>Assertive:</strong> Para alertas importantes (interrompe leitura)
            </li>
          </ul>

          <h4>Contraste e Cores</h4>
          <ul>
            <li>Interface projetada com contraste adequado para leitura</li>
            <li>Não depende apenas de cores para transmitir informação</li>
          </ul>

          <h4>Interação por Voz</h4>
          <p>
            Para usuários que preferem ou precisam usar voz, o sistema oferece reconhecimento de
            fala (STT) e síntese de voz (TTS) completos:
          </p>
          <ul>
            <li>Dite suas mensagens em vez de digitar</li>
            <li>Ouça as respostas da IA em voz alta</li>
            <li>Configure hotkeys globais para ativar voz rapidamente</li>
          </ul>

          <h4>Feedback Sonoro</h4>
          <p>Sons indicam eventos importantes:</p>
          <ul>
            <li>Início e fim de gravação de voz</li>
            <li>Recebimento de respostas</li>
            <li>Erros e alertas</li>
          </ul>
        </div>
      ),
    },
    {
      id: 'export-import',
      title: 'Exportação e Importação',
      icon: <ExportOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Dados Exportáveis</h4>
          <ul>
            <li>
              <strong>Conversas:</strong> Histórico completo de mensagens
            </li>
            <li>
              <strong>Perfis de Voz:</strong> Configurações de síntese de voz
            </li>
            <li>
              <strong>Perfis de Interação:</strong> Configurações de entrada e hotkeys
            </li>
          </ul>

          <h4>Formato de Exportação</h4>
          <p>
            Os dados são exportados em formato JSON estruturado, incluindo metadados como versão do
            formato e data de exportação.
          </p>

          <h4>Como Exportar</h4>
          <ol>
            <li>Vá para a página do recurso desejado (Histórico, Perfis de Voz, etc.)</li>
            <li>Selecione os itens a exportar (ou todos)</li>
            <li>Clique em "Exportar"</li>
            <li>Escolha o local para salvar o arquivo</li>
          </ol>

          <h4>Como Importar</h4>
          <ol>
            <li>Vá para a página do recurso desejado</li>
            <li>Clique em "Importar"</li>
            <li>Selecione o arquivo JSON de backup</li>
            <li>Confirme a importação</li>
          </ol>

          <h4>Tratamento de Conflitos</h4>
          <p>Na importação, IDs são regenerados para evitar conflitos com dados existentes.</p>
        </div>
      ),
    },
    {
      id: 'data-storage',
      title: 'Armazenamento de Dados',
      icon: <SaveOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Localização dos Dados</h4>
          <p>Todos os dados são armazenados localmente no seu computador:</p>
          <ul>
            <li>
              <strong>Configurações:</strong> <code>~/.assistente/config.json</code>
            </li>
            <li>
              <strong>Banco de Dados:</strong> <code>~/.assistente/assistente.db</code> (SQLite)
            </li>
            <li>
              <strong>Logs:</strong> <code>~/.assistente/logs/</code>
            </li>
          </ul>

          <h4>O que é Armazenado</h4>
          <ul>
            <li>Conversas e mensagens</li>
            <li>Perfis de voz e interação</li>
            <li>Estatísticas de uso de tokens</li>
          </ul>

          <h4>Privacidade</h4>
          <ul>
            <li>Dados são armazenados apenas localmente</li>
            <li>Comunicação com APIs é feita diretamente (sem intermediários)</li>
            <li>Você pode excluir todos os dados a qualquer momento</li>
          </ul>

          <h4>Backup</h4>
          <p>
            Para fazer backup completo, copie o diretório <code>~/.assistente/</code> ou use a
            funcionalidade de exportação para itens específicos.
          </p>
        </div>
      ),
    },
    {
      id: 'troubleshooting',
      title: 'Solução de Problemas',
      icon: <ToolOutlined aria-hidden="true" />,
      content: (
        <div className="help-content">
          <h4>Problemas de Conexão com API</h4>
          <ul>
            <li>Verifique se a API Key está correta</li>
            <li>Confirme que a Base URL está correta</li>
            <li>Use "Testar Conexão" nas Configurações</li>
            <li>Verifique se há créditos disponíveis na sua conta OpenAI</li>
          </ul>

          <h4>Problemas de Voz</h4>
          <ul>
            <li>
              <strong>Microfone não detectado:</strong> Verifique permissões do navegador/sistema
            </li>
            <li>
              <strong>STT não funciona:</strong> Tente outro provedor (Web Speech vs Whisper)
            </li>
            <li>
              <strong>TTS sem som:</strong> Verifique volume do sistema e do app
            </li>
            <li>
              <strong>SAPI5 não lista vozes:</strong> Apenas disponível no Windows
            </li>
          </ul>

          <h4>Problemas de Performance</h4>
          <ul>
            <li>
              <strong>App lento:</strong> Limite o número de abas abertas
            </li>
            <li>
              <strong>Conversas grandes:</strong> Threads são carregadas sob demanda (lazy loading)
            </li>
          </ul>

          <h4>Resetar Aplicação</h4>
          <p>Se nada funcionar:</p>
          <ol>
            <li>Tente "Resetar Configurações" em Configurações</li>
            <li>
              Se persistir, &quot;Apagar Banco de Dados&quot; (
              <WarningOutlined aria-hidden="true" /> perda de dados)
            </li>
            <li>
              Último recurso: delete a pasta <code>~/.assistente/</code>
            </li>
          </ol>
        </div>
      ),
    },
  ];

  return (
    <div className="help-page">
      <div className="help-header">
        <h1>{t('help.title', 'Ajuda')}</h1>
        <p>{t('help.subtitle', 'Guia completo do Assistente IA')}</p>
        <div className="help-header-actions">
          <button className="help-expand-btn" onClick={expandAll}>
            Expandir Tudo
          </button>
          <button className="help-expand-btn" onClick={collapseAll}>
            Colapsar Tudo
          </button>
        </div>
      </div>

      <main className="help-main">
        {sections.map((section) => {
          const isExpanded = expandedSections.has(section.id);
          return (
            <section
              key={section.id}
              id={`help-${section.id}`}
              className={`help-section ${isExpanded ? 'expanded' : ''}`}
            >
              <button
                className="help-section-header"
                onClick={() => toggleSection(section.id)}
                aria-expanded={isExpanded}
                aria-controls={`help-content-${section.id}`}
              >
                <span className="help-section-icon" aria-hidden="true">
                  {section.icon}
                </span>
                <h3>{t(`help.sections.${section.id}`)}</h3>
                <span className="help-section-chevron" aria-hidden="true">
                  {isExpanded ? '▼' : '▶'}
                </span>
              </button>
              {isExpanded && (
                <div id={`help-content-${section.id}`} className="help-section-body">
                  {section.content}
                </div>
              )}
            </section>
          );
        })}
      </main>

      <footer className="help-footer">
        <p>
          {t('help.footer')}
        </p>
      </footer>
    </div>
  );
}
