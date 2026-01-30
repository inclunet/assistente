import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import './HelpPage.css';

interface HelpSection {
  id: string;
  title: string;
  icon: string;
  content: React.ReactNode;
}

export default function HelpPage() {
  const { t } = useTranslation();
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
      icon: '🗣️',
      content: (
        <div className="help-content">
          <p>
            O grande diferencial do Assistente é que você pode <strong>pedir ações diretamente no chat</strong>,
            seja digitando ou falando. A IA entende comandos em linguagem natural e executa as ações
            através dos agentes integrados.
          </p>

          <h4>Por que usar comandos no chat?</h4>
          <ul>
            <li>Não precisa decorar teclas de atalho ou navegar por menus</li>
            <li>Funciona tanto digitando quanto por voz</li>
            <li>Comandos em português ou inglês são entendidos</li>
            <li>A IA confirma as ações antes de executar quando necessário</li>
          </ul>

          <h4>Gerenciamento de Conversas</h4>
          <p>Peça para o Chat Manager:</p>
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

          <h4>Memórias</h4>
          <p>Peça para o Memory Agent:</p>
          <ul>
            <li><code>"Lembra que eu prefiro X"</code> / <code>"Salva que meu nome é Y"</code></li>
            <li><code>"Guarda que eu trabalho com Z"</code></li>
            <li><code>"Você lembra meu nome?"</code> / <code>"O que você sabe sobre mim?"</code></li>
            <li><code>"Esquece aquela informação sobre X"</code></li>
            <li><code>"Atualiza: agora eu prefiro Y"</code></li>
            <li><code>"Lista tudo que você sabe sobre mim"</code></li>
            <li><code>"Tem alguma aba aberta sobre X?"</code></li>
            <li><code>"A gente já conversou sobre Y?"</code></li>
          </ul>

          <h4>FAQs e Base de Conhecimento</h4>
          <p>Peça para o FAQ Agent:</p>
          <ul>
            <li><code>"Como faço para X?"</code> - busca automaticamente na base</li>
            <li><code>"Tem documentação sobre Y?"</code></li>
            <li><code>"Cria um FAQ: Pergunta... Resposta..."</code></li>
            <li><code>"Adiciona na base de conhecimento: ..."</code></li>
            <li><code>"Atualiza o FAQ sobre X"</code></li>
            <li><code>"Deleta o FAQ sobre Y"</code></li>
          </ul>

          <h4>Arquivos</h4>
          <p>Peça para o File Agent:</p>
          <ul>
            <li><code>"Lê o arquivo X"</code> / <code>"Abre o documento Y"</code></li>
            <li><code>"O que tem nesse PDF?"</code> (com arquivo anexado)</li>
            <li><code>"Lista os arquivos na pasta Z"</code></li>
            <li><code>"Busca por 'texto' nos arquivos"</code></li>
            <li><code>"Cria um arquivo chamado X com o conteúdo Y"</code></li>
            <li><code>"Salva isso no arquivo X"</code></li>
          </ul>

          <h4>Imagens</h4>
          <p>Peça para o Image Agent:</p>
          <ul>
            <li><code>"Gera uma imagem de X"</code></li>
            <li><code>"Cria uma ilustração mostrando Y"</code></li>
            <li><code>"Desenha um diagrama de Z"</code></li>
          </ul>

          <h4>Perfis de Voz e Interação</h4>
          <p>Peça para o Profile Agent:</p>
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
            <li>Você pode combinar comandos: <code>"Salva que meu nome é João e cria uma nova conversa"</code></li>
            <li>A IA pede confirmação antes de ações destrutivas como deletar</li>
            <li>Se algo não funcionar, tente reformular o pedido</li>
          </ul>
        </div>
      ),
    },
    {
      id: 'overview',
      title: 'Visão Geral',
      icon: '📖',
      content: (
        <div className="help-content">
          <p>
            O <strong>Assistente IA</strong> é uma aplicação desktop de inteligência artificial que
            permite conversas naturais com modelos de linguagem, suporte a voz, gerenciamento de
            conhecimento e integração com serviços externos.
          </p>

          <h4>Destaque: Comandos por Chat e Voz</h4>
          <p>
            Você pode <strong>pedir ações diretamente no chat</strong>, digitando ou falando.
            Em vez de navegar por menus, simplesmente diga o que quer fazer:
          </p>
          <ul>
            <li><em>"Salva que meu email é x@y.com"</em> - cria uma memória</li>
            <li><em>"Cria uma nova conversa"</em> - abre uma nova aba</li>
            <li><em>"Lê o arquivo relatório.pdf"</em> - acessa um arquivo</li>
            <li><em>"Usa a voz do OpenAI"</em> - muda o perfil de voz</li>
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
              <strong>Memória:</strong> A IA lembra informações sobre você entre conversas
            </li>
            <li>
              <strong>FAQ:</strong> Base de conhecimento para respostas rápidas sobre procedimentos
            </li>
            <li>
              <strong>Agentes:</strong> Ferramentas que permitem à IA executar ações como ler
              arquivos, gerar imagens, buscar informações
            </li>
            <li>
              <strong>Arquivos:</strong> Envie arquivos no chat ou peça para ler documentos do seu
              computador
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
      icon: '💬',
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

          <h4>Threads e Mensagens Internas</h4>
          <p>
            As mensagens são organizadas em uma estrutura hierárquica. O nível principal (raiz)
            contém suas mensagens e as respostas finais da IA. As threads contêm as interações
            internas: chamadas de ferramentas (tool calls), requisições a agentes e suas respostas.
          </p>
          <ul>
            <li>
              <strong>Mostrar/ocultar internas:</strong> Use o toggle "Mostrar mensagens internas"
              na barra de ferramentas
            </li>
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
          <p>
            As threads são úteis para entender como a IA chegou a uma resposta, ver quais agentes
            foram consultados e depurar problemas.
          </p>

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
              <strong>Tool Calls:</strong> A IA pode executar ferramentas e agentes automaticamente
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
      icon: '🎤',
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
            <li>Clique no botão de microfone 🎤 ou use o hotkey configurado</li>
            <li>Fale sua mensagem ou comando</li>
            <li>A gravação para automaticamente (VAD) ou quando você soltar a tecla (PTT)</li>
            <li>Sua fala é convertida em texto e enviada</li>
            <li>A resposta pode ser lida em voz alta automaticamente</li>
          </ol>

          <h4>Exemplos de Comandos por Voz</h4>
          <ul>
            <li><em>"Lembra que meu nome é João"</em></li>
            <li><em>"Cria uma nova conversa"</em></li>
            <li><em>"Gera uma imagem de um gato programando"</em></li>
            <li><em>"Busca nos FAQs como configurar o sistema"</em></li>
            <li><em>"Usa a voz mais rápida"</em></li>
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
      id: 'memory',
      title: 'Memória',
      icon: '🧠',
      content: (
        <div className="help-content">
          <h4>O que são Memórias?</h4>
          <p>
            Memórias são informações persistentes que a IA pode consultar durante as conversas.
            Diferente do contexto da conversa, memórias são armazenadas permanentemente e podem ser
            recuperadas através de busca semântica.
          </p>

          <h4>Tipos de Uso</h4>
          <ul>
            <li>
              <strong>Informações pessoais:</strong> Preferências, dados de contato, informações
              sobre você
            </li>
            <li>
              <strong>Conhecimento específico:</strong> Fatos, definições, procedimentos da sua
              área
            </li>
            <li>
              <strong>Contexto de projetos:</strong> Detalhes sobre projetos em andamento
            </li>
            <li>
              <strong>Instruções recorrentes:</strong> Como você prefere que certas tarefas sejam
              feitas
            </li>
          </ul>

          <h4>Gerenciamento</h4>
          <ul>
            <li>
              <strong>Criar memória:</strong> Ctrl+N na página de Memória
            </li>
            <li>
              <strong>Categorias:</strong> Organize memórias por categoria para melhor recuperação
            </li>
            <li>
              <strong>Embeddings:</strong> Memórias são indexadas automaticamente para busca
              semântica
            </li>
          </ul>

          <h4>Como a IA usa Memórias</h4>
          <p>
            O agente de memória é consultado automaticamente quando você faz perguntas sobre
            informações pessoais, preferências ou coisas que você mencionou anteriormente. Ele
            também busca em abas abertas e no histórico de conversas para encontrar contexto
            relevante.
          </p>
        </div>
      ),
    },
    {
      id: 'faq',
      title: 'FAQ (Perguntas Frequentes)',
      icon: '❓',
      content: (
        <div className="help-content">
          <h4>O que é o FAQ?</h4>
          <p>
            O FAQ é uma base de conhecimento estruturada com pares de pergunta-resposta. Diferente
            das memórias (texto livre), FAQs são otimizados para consultas específicas.
          </p>

          <h4>Quando usar FAQ vs Memória?</h4>
          <ul>
            <li>
              <strong>FAQ:</strong> Para perguntas específicas com respostas diretas e bem
              definidas
            </li>
            <li>
              <strong>Memória:</strong> Para informações contextuais e conhecimento geral
            </li>
          </ul>

          <h4>Gerenciamento</h4>
          <ul>
            <li>
              <strong>Criar FAQ:</strong> Ctrl+N na página de FAQ
            </li>
            <li>
              <strong>Formato:</strong> Cada FAQ tem uma pergunta (título) e uma resposta (conteúdo)
            </li>
            <li>
              <strong>Busca:</strong> Suporta busca textual e semântica
            </li>
          </ul>

          <h4>Integração com Chat</h4>
          <p>
            O agente de FAQ é consultado automaticamente durante as conversas. Quando uma pergunta
            similar é identificada, a resposta do FAQ é incluída na resposta da IA.
          </p>
        </div>
      ),
    },
    {
      id: 'agents',
      title: 'Agentes',
      icon: '🤖',
      content: (
        <div className="help-content">
          <h4>O que são Agentes?</h4>
          <p>
            Agentes são ferramentas que estendem as capacidades da IA. Eles permitem que a IA
            execute ações como buscar informações, manipular arquivos, chamar APIs externas e muito
            mais.
          </p>

          <h4>Agentes Internos (Nativos)</h4>
          <ul>
            <li>
              <strong>Memory Agent:</strong> Gerencia memórias persistentes sobre você (salva,
              busca, atualiza). Também busca em abas abertas e histórico de conversas para
              encontrar contexto relevante.
            </li>
            <li>
              <strong>FAQ Agent:</strong> Busca respostas na base de perguntas frequentes usando
              busca semântica.
            </li>
            <li>
              <strong>File Agent:</strong> Lê, escreve, lista e busca em arquivos. Só funciona em
              pastas autorizadas.
            </li>
            <li>
              <strong>Image Agent:</strong> Gera imagens a partir de descrições usando DALL-E.
            </li>
            <li>
              <strong>Chat Manager:</strong> Executa ações em abas e conversas: navegar, criar,
              renomear, excluir, limpar, resumir.
            </li>
            <li>
              <strong>Profile Agent:</strong> Gerencia perfis de voz e interação: lista, cria,
              atualiza, ativa.
            </li>
            <li>
              <strong>Builder Agent:</strong> Cria e modifica agentes HTTP e MCP dinamicamente.
            </li>
          </ul>

          <h4>Agentes HTTP</h4>
          <p>
            Permitem integração com APIs REST externas. Você pode criar endpoints personalizados com
            templates Go para processar requisições e respostas.
          </p>
          <ul>
            <li>
              <strong>Endpoints:</strong> Defina múltiplos endpoints por agente
            </li>
            <li>
              <strong>Templates:</strong> Use templates Go para construir URLs, headers e body
            </li>
            <li>
              <strong>OAuth:</strong> Suporte a autenticação OAuth2 para APIs protegidas
            </li>
          </ul>

          <h4>Agentes MCP</h4>
          <p>
            Implementam o Model Context Protocol para integração padronizada com ferramentas
            externas.
          </p>
          <ul>
            <li>
              <strong>Modo stdio:</strong> Comunicação via processo local
            </li>
            <li>
              <strong>Modo HTTP:</strong> Comunicação via servidor HTTP
            </li>
          </ul>

          <h4>Gerenciamento de Agentes</h4>
          <ul>
            <li>
              <strong>Habilitar/Desabilitar:</strong> Controle quais agentes estão ativos
            </li>
            <li>
              <strong>Testar:</strong> Use o chat de teste para validar agentes
            </li>
            <li>
              <strong>Diagnóstico:</strong> Veja logs e erros de execução
            </li>
            <li>
              <strong>Hot Reload:</strong> Recarregue agentes sem reiniciar o app
            </li>
          </ul>
        </div>
      ),
    },
    {
      id: 'files',
      title: 'Arquivos e Documentos',
      icon: '📁',
      content: (
        <div className="help-content">
          <h4>Enviando Arquivos no Chat (Upload)</h4>
          <p>
            Você pode enviar arquivos diretamente na conversa para a IA analisar. Há três formas de
            fazer isso:
          </p>
          <ul>
            <li>
              <strong>Botão Anexar (📎):</strong> Clique no botão ao lado do campo de mensagem e
              selecione os arquivos
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

          <h4>File Agent (Acesso a Arquivos do Sistema)</h4>
          <p>
            O File Agent permite que a IA acesse arquivos no seu computador sem precisar fazer
            upload. Basta pedir no chat:
          </p>
          <ul>
            <li><code>"Lê o arquivo C:/pasta/documento.txt"</code></li>
            <li><code>"Lista os arquivos na pasta X"</code></li>
            <li><code>"Busca por 'palavra' nos arquivos"</code></li>
            <li><code>"Salva isso no arquivo Y"</code></li>
          </ul>

          <h4>Formatos Suportados pelo File Agent</h4>
          <p>O File Agent suporta mais formatos que o upload direto:</p>
          <ul>
            <li>Texto, Markdown, JSON, XML, YAML, CSV</li>
            <li>Código-fonte (todas as linguagens)</li>
            <li>PDF, Word, Excel, PowerPoint</li>
            <li>OpenDocument (ODT, ODS, ODP)</li>
            <li>E-books (EPUB)</li>
            <li>Google Docs (via OAuth)</li>
          </ul>

          <h4>Pastas Autorizadas (Segurança)</h4>
          <p>
            O File Agent só pode acessar pastas que você autorizar explicitamente. Configure em:
            Menu → Agentes → File Agent → Pastas Autorizadas.
          </p>
        </div>
      ),
    },
    {
      id: 'profiles',
      title: 'Perfis de Voz e Interação',
      icon: '🎭',
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

          <h4>Gerenciando Perfis pelo Chat</h4>
          <p>Você pode gerenciar perfis diretamente no chat:</p>
          <ul>
            <li><code>"Usa a voz da OpenAI nessa conversa"</code></li>
            <li><code>"Cria um perfil de voz com voz feminina e velocidade 1.2"</code></li>
            <li><code>"Lista meus perfis de voz"</code></li>
            <li><code>"Muda a velocidade da voz para mais lento"</code></li>
            <li><code>"Ativa o perfil de interação Desktop"</code></li>
          </ul>

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
      id: 'oauth',
      title: 'Integrações OAuth',
      icon: '🔐',
      content: (
        <div className="help-content">
          <h4>O que é OAuth?</h4>
          <p>
            OAuth é um protocolo de autorização que permite que o Assistente acesse serviços
            externos em seu nome, sem que você precise compartilhar suas senhas.
          </p>

          <h4>Provedores Suportados</h4>
          <ul>
            <li>
              <strong>Google:</strong> Google Drive, Google Docs, Gmail, etc.
            </li>
            <li>
              <strong>Microsoft:</strong> OneDrive, Outlook, Microsoft 365
            </li>
            <li>
              <strong>GitHub:</strong> Repositórios, Issues, Pull Requests
            </li>
          </ul>

          <h4>Como Conectar</h4>
          <ol>
            <li>Vá para a página OAuth no menu</li>
            <li>Clique em "Conectar" no provedor desejado</li>
            <li>Autorize o acesso no navegador</li>
            <li>O token será salvo automaticamente</li>
          </ol>

          <h4>Gerenciamento de Conexões</h4>
          <ul>
            <li>
              <strong>Status:</strong> Veja se a conexão está ativa
            </li>
            <li>
              <strong>Renovar:</strong> Tokens são renovados automaticamente quando expiram
            </li>
            <li>
              <strong>Desconectar:</strong> Revogue o acesso a qualquer momento
            </li>
          </ul>

          <h4>Uso pelos Agentes</h4>
          <p>
            Agentes HTTP podem usar conexões OAuth para acessar APIs protegidas. Configure o
            provedor OAuth no endpoint do agente.
          </p>
        </div>
      ),
    },
    {
      id: 'settings',
      title: 'Configurações',
      icon: '⚙️',
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

          <h4>Embeddings</h4>
          <ul>
            <li>
              <strong>Modelo:</strong> Modelo para busca semântica (recomendado:
              text-embedding-3-small)
            </li>
            <li>
              <strong>Dimensões:</strong> Tamanho do vetor (0 = padrão do modelo)
            </li>
          </ul>

          <h4>Geração de Imagens</h4>
          <ul>
            <li>
              <strong>Modelo:</strong> DALL-E 3, DALL-E 2 ou GPT Image 1
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

          <h4>Padrões do Chat</h4>
          <ul>
            <li>
              <strong>Usar Agentes:</strong> Habilita ferramentas por padrão
            </li>
            <li>
              <strong>Mostrar Mensagens Internas:</strong> Exibe tool calls na conversa
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
      icon: '📜',
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
            O agente de memória pode buscar automaticamente em conversas do histórico quando você
            faz perguntas sobre discussões anteriores. Use frases como "lembra quando
            conversamos sobre..." para ativar essa busca.
          </p>
        </div>
      ),
    },
    {
      id: 'keyboard',
      title: 'Teclas de Atalho',
      icon: '⌨️',
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
      icon: '♿',
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
      icon: '📤',
      content: (
        <div className="help-content">
          <h4>Dados Exportáveis</h4>
          <ul>
            <li>
              <strong>Conversas:</strong> Histórico completo de mensagens
            </li>
            <li>
              <strong>Memórias:</strong> Base de conhecimento
            </li>
            <li>
              <strong>FAQs:</strong> Perguntas frequentes
            </li>
            <li>
              <strong>Agentes HTTP:</strong> Configurações de agentes criados
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
            <li>Vá para a página do recurso desejado (Histórico, Memória, etc.)</li>
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
      icon: '💾',
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
            <li>Memórias e FAQs</li>
            <li>Embeddings (vetores para busca semântica)</li>
            <li>Perfis de voz e interação</li>
            <li>Configurações de agentes</li>
            <li>Tokens OAuth (criptografados)</li>
            <li>Estatísticas de uso de tokens</li>
          </ul>

          <h4>Privacidade</h4>
          <ul>
            <li>Dados são armazenados apenas localmente</li>
            <li>Comunicação com APIs é feita diretamente (sem intermediários)</li>
            <li>Tokens OAuth são armazenados de forma segura</li>
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
      icon: '🔧',
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
            <li>
              <strong>Busca lenta:</strong> Embeddings são calculados em background
            </li>
          </ul>

          <h4>Problemas com Agentes</h4>
          <ul>
            <li>Use o "Diagnóstico" para ver logs de execução</li>
            <li>Teste endpoints individualmente com o "Testador"</li>
            <li>Verifique conexões OAuth se o agente precisa de autenticação</li>
            <li>Use "Hot Reload" após modificar configurações</li>
          </ul>

          <h4>Problemas com Arquivos</h4>
          <ul>
            <li>Verifique se a pasta está autorizada no File Agent</li>
            <li>Confirme que o formato de arquivo é suportado</li>
            <li>Arquivos muito grandes podem demorar para processar</li>
          </ul>

          <h4>Resetar Aplicação</h4>
          <p>Se nada funcionar:</p>
          <ol>
            <li>Tente "Resetar Configurações" em Configurações</li>
            <li>Se persistir, "Apagar Banco de Dados" (⚠️ perda de dados)</li>
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
                <h3>{section.title}</h3>
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
          Versão do Assistente IA • Última atualização da documentação: Janeiro 2026
        </p>
      </footer>
    </div>
  );
}
