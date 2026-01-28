import { useState, useRef, useEffect } from 'react';
import { TestAgent } from '../../../wailsjs/go/main/App';
import { Modal } from '../ui/Modal';
import { MessageList } from '../chat/MessageList';
import { ChatInput } from '../chat/ChatInput';
import { MessageNode as MessageNodeType } from '../../store/chatStore';
import { main } from '../../../wailsjs/go/models';
import './AgentTestChat.css';

interface AgentTestChatProps {
  agentName: string;
  agentType: string;
  displayName?: string;
  onClose: () => void;
}

interface SimpleMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: Date;
}

export function AgentTestChat({ agentName, agentType, displayName, onClose }: AgentTestChatProps) {
  const [messages, setMessages] = useState<SimpleMessage[]>([
    {
      role: 'system',
      content: `Testando agente: ${displayName || agentName}. Digite uma mensagem para interagir com o agente.`,
      timestamp: new Date()
    }
  ]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [announcement, setAnnouncement] = useState('');
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const messageListRef = useRef<HTMLDivElement>(null);

  // Foca no botão de fechar quando o modal abre (primeiro elemento navegável)
  useEffect(() => {
    const timer = setTimeout(() => {
      closeButtonRef.current?.focus();
    }, 100);
    return () => clearTimeout(timer);
  }, []);

  const announce = (message: string) => {
    setAnnouncement(message);
    setTimeout(() => setAnnouncement(''), 100);
  };

  // Handler para focar no último item da MessageList
  const focusLastMessage = () => {
    if (!messageListRef.current) return;
    
    // Encontra todos os message nodes
    const messageNodes = messageListRef.current.querySelectorAll('[data-message-node]');
    if (messageNodes.length > 0) {
      const lastNode = messageNodes[messageNodes.length - 1] as HTMLElement;
      lastNode.focus();
    }
  };

  // Handler para focar no input
  const focusInput = () => {
    inputRef.current?.focus();
  };

  // Handler customizado para ArrowUp no ChatInput
  const handleInputArrowUp = () => {
    // Verifica se o cursor está no início do texto
    const textarea = inputRef.current;
    if (textarea && textarea.selectionStart === 0) {
      focusLastMessage();
    }
  };

  // Converte mensagens simples para o formato MessageNode
  const convertToMessageNodes = (msgs: SimpleMessage[]): MessageNodeType[] => {
    return msgs.map((msg, idx) => {
      const enrichedMsg = new main.EnrichedMessage({
        id: `test-msg-${idx}`,
        conversationId: 0,
        parentId: null,
        role: msg.role,
        content: msg.content,
        createdAt: msg.timestamp,
        timestamp: msg.timestamp.getTime(),
        isStreaming: false,
        internal: msg.role === 'system',
        model: agentType === 'internal' ? agentName : undefined,
      });

      const node = new main.MessageNode({
        message: enrichedMsg,
        children: [],
        level: 0,
        childCount: 0,
      });

      // Adiciona propriedade isExpanded mantendo o objeto node intacto
      (node as MessageNodeType).isExpanded = false;
      
      return node as MessageNodeType;
    });
  };

  const handleSend = async (message: string) => {
    if (!message.trim() || loading) return;

    const userMessage: SimpleMessage = {
      role: 'user',
      content: message.trim(),
      timestamp: new Date()
    };

    setMessages(prev => [...prev, userMessage]);
    setLoading(true);
    setError('');
    announce('Enviando mensagem para o agente. Aguarde...');

    console.log('[AgentTestChat] Enviando para agente:', {
      agentName,
      agentType,
      message: message.trim()
    });

    try {
      // Chama o backend para testar o agente
      // TestAgent(agentName, task) - task é a mensagem do usuário
      const response = await TestAgent(agentName, message.trim());
      
      console.log('[AgentTestChat] Resposta recebida:', response);
      
      const assistantMessage: SimpleMessage = {
        role: 'assistant',
        content: response || 'Sem resposta do agente.',
        timestamp: new Date()
      };

      setMessages(prev => [...prev, assistantMessage]);
      announce(`Resposta recebida do agente: ${assistantMessage.content}`);
    } catch (err: any) {
      console.error('[AgentTestChat] Erro:', err);
      const errorMsg = 'Erro ao testar agente: ' + (err.message || err);
      setError(errorMsg);
      announce(errorMsg);
      
      const errorMessage: SimpleMessage = {
        role: 'assistant',
        content: `❌ Erro: ${err.message || err}`,
        timestamp: new Date()
      };

      setMessages(prev => [...prev, errorMessage]);
    } finally {
      setLoading(false);
    }
  };

  const threadedMessages = convertToMessageNodes(messages);

  return (
    <Modal
      id="agent-test-chat"
      onClose={onClose}
      title={`Testar: ${displayName || agentName}`}
      size="lg"
    >
      <div className="agent-test-chat" role="application" aria-label="Chat de teste do agente">
        {/* Live region para anúncios */}
        <div 
          role="status" 
          aria-live="polite" 
          aria-atomic="true"
          className="sr-only"
        >
          {announcement}
        </div>

        {/* Botão de fechar (primeiro na ordem de navegação) */}
        <button
          ref={closeButtonRef}
          onClick={onClose}
          className="test-chat-close sr-only-focusable"
          aria-label="Fechar chat de teste"
        >
          Fechar
        </button>

        {error && (
          <div 
            className="test-chat-error" 
            role="alert"
            aria-live="assertive"
          >
            {error}
          </div>
        )}

        {/* MessageList do chat (segundo na ordem de navegação) */}
        <div 
          ref={messageListRef}
          className="test-chat-messages-container"
        >
          <MessageList
            isLoading={loading}
            threadedMessages={threadedMessages}
            onReachEnd={focusInput}
          />
        </div>

        {/* ChatInput (terceiro na ordem de navegação) */}
        <div className="test-chat-input-container">
          <ChatInput
            ref={inputRef}
            onSend={handleSend}
            disabled={loading}
            placeholder="Digite sua mensagem para testar o agente..."
            onArrowUp={handleInputArrowUp}
          />
        </div>

        <div className="test-chat-note" role="note">
          <strong>Nota:</strong> Esta é uma janela de teste simplificada. Use as setas para navegar entre mensagens.
        </div>
      </div>
    </Modal>
  );
}
