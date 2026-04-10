package messaging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"

	"assistente/internal/channels"
	"assistente/internal/contacts"
	"assistente/internal/database"
	"assistente/internal/llm"
)

// SendMessageFunc é a assinatura da função App.SendMessage (ou wrapper).
// Recebe conversationID (0=criar nova), conteúdo, mídia, params e source.
// Retorna o conversationID usado.
type SendMessageFunc func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error)

// emitFunc é a callback para emitir eventos Wails.
type emitFunc func(event string, data any)

// SynthesizeTTSFunc é a assinatura da função que sintetiza texto em áudio (MP3).
// Recebe o texto, o canal e se a mensagem original era áudio.
// Resolve o perfil do canal e o ChannelResponseMode para decidir se gera TTS.
// Retorna (nil, nil) se não deve gerar áudio (o gateway enviará texto).
type SynthesizeTTSFunc func(text string, channel string, incomingIsAudio bool) ([]byte, error)

// SaveAudioFunc é a assinatura da função que salva áudio no DB.
type SaveAudioFunc func(messageID uint, audioBase64 string, mimeType string) error

// GetCachedAudioFunc busca áudio já gerado (cache TTS proativo) no DB.
// Retorna (audioBase64, mimeType, err). Se não encontrar, retorna ("", "", nil).
type GetCachedAudioFunc func(messageID uint) (audioBase64 string, mimeType string, err error)

// ApproveContactFunc é a assinatura da função que solicita aprovação para autorizar um contato.
// Retorna true se aprovado, false caso contrário.
type ApproveContactFunc func(ctx context.Context, channel, displayName, contactID, username string) (bool, error)

// Gateway é o router central de mensageria. Conecta os adapters dos mensageiros
// ao processamento do assistente (via App.SendMessage).
//
// Fluxo:
//  1. Adapter recebe mensagem → chama handler do Gateway
//  2. Gateway verifica allowlist
//  3. Gateway registra callback no Notifier (para capturar resposta)
//  4. Gateway chama SendMessage (mesma função do Wails)
//  5. Quando resposta fica pronta, Notifier dispara callback
//  6. Gateway reenvia resposta ao mensageiro de origem
type Gateway struct {
	mu             sync.RWMutex
	messengers     map[string]Messenger
	notifier       *ResponseNotifier
	sendMessage    SendMessageFunc
	emitEvent      emitFunc
	approveContact ApproveContactFunc
	synthesizeTTS  SynthesizeTTSFunc  // Opcional: sintetiza áudio para respostas em modo áudio
	saveAudio      SaveAudioFunc      // Opcional: salva áudio no DB
	getCachedAudio GetCachedAudioFunc // Opcional: busca áudio TTS proativo do cache DB
}

// NewGateway cria um novo Gateway de mensageria.
func NewGateway(
	notifier *ResponseNotifier,
	sendMessage SendMessageFunc,
	emitEvent emitFunc,
	approveContact ApproveContactFunc,
	synthesizeTTS SynthesizeTTSFunc,
	saveAudio SaveAudioFunc,
	getCachedAudio GetCachedAudioFunc,
) *Gateway {
	return &Gateway{
		messengers:     make(map[string]Messenger),
		notifier:       notifier,
		sendMessage:    sendMessage,
		emitEvent:      emitEvent,
		approveContact: approveContact,
		synthesizeTTS:  synthesizeTTS,
		saveAudio:      saveAudio,
		getCachedAudio: getCachedAudio,
	}
}

// Register registra um adapter de mensageiro e configura seu handler.
func (g *Gateway) Register(name string, m Messenger) {
	g.mu.Lock()
	g.messengers[name] = m
	g.mu.Unlock()

	m.SetHandler(g.handleIncoming)
	log.Printf("[Gateway] Messenger '%s' registrado", name)
}

// Unregister desconecta e remove um messenger pelo nome.
func (g *Gateway) Unregister(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if m, ok := g.messengers[name]; ok {
		log.Printf("[Gateway] Desconectando '%s'...", name)
		if err := m.Disconnect(); err != nil {
			log.Printf("[Gateway] Erro ao desconectar '%s': %v", name, err)
		}
		delete(g.messengers, name)
	}
}

// Shutdown desconecta todos os messengers.
func (g *Gateway) Shutdown() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for name, m := range g.messengers {
		log.Printf("[Gateway] Desconectando '%s'...", name)
		if err := m.Disconnect(); err != nil {
			log.Printf("[Gateway] Erro ao desconectar '%s': %v", name, err)
		}
	}
	g.messengers = make(map[string]Messenger)
}

// GetStatus retorna o status de todos os messengers registrados.
func (g *Gateway) GetStatus() map[string]ConnectionStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	status := make(map[string]ConnectionStatus, len(g.messengers))
	for name, m := range g.messengers {
		status[name] = m.Status()
	}
	return status
}

// GetMessenger retorna um messenger pelo nome (para uso pela tool send_message).
func (g *Gateway) GetMessenger(name string) (Messenger, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	m, ok := g.messengers[name]
	return m, ok
}

// handleIncoming é chamado quando uma mensagem chega de qualquer messenger.
func (g *Gateway) handleIncoming(ctx context.Context, msg IncomingMessage) {
	traceID := uuid.NewString()

	// 1. Verifica contato autorizado (contacts.json centralizado + max_contacts do canal)
	maxContacts := 1
	if chCfg, _ := channels.Load(msg.Channel); chCfg != nil {
		maxContacts = chCfg.GetMaxContacts()
	}

	hasContacts, isAllowed := contacts.IsAuthorized(msg.Channel, maxContacts, msg.From.ID, msg.From.Username)

	if hasContacts && !isAllowed {
		// Limite de contatos atingido e este não está na lista — rejeita silenciosamente
		log.Printf("[Gateway] trace=%s conv=? channel=%s contact=%s name=%s msg=%s rejeitada (limite de contatos)",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), msg.From.DisplayName, msg.ID)
		return
	}

	if !hasContacts {
		// Canal sem contatos ou com vaga — gera código de pareamento
		log.Printf("[Gateway] trace=%s conv=? channel=%s contact=%s username=%s name=%s msg=%s aguardando pareamento",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), maskIdentifier(msg.From.Username), msg.From.DisplayName, msg.ID)

		// Gera código de 6 dígitos
		pairingCode := contacts.GeneratePairingCode(msg.Channel, msg.From.ID)
		log.Printf("[Gateway] trace=%s channel=%s contact=%s código de pareamento gerado: %s",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), pairingCode)

		// Envia mensagem com código para o contato
		if messenger, ok := g.GetMessenger(msg.Channel); ok {
			pairingMsg := fmt.Sprintf(
				"Bem-vindo! Para autorizar seu acesso, responda ao assistente com o seguinte código de pareamento:\n\n🔐 Código: %s",
				pairingCode,
			)
			outMsg := OutgoingMessage{
				ChatID: msg.From.ID,
				Text:   pairingMsg,
			}
			if err := messenger.Send(ctx, outMsg); err != nil {
				log.Printf("[Gateway] trace=%s channel=%s erro ao enviar código: %v", traceID, msg.Channel, err)
			}
		}

		// Solicita confirmação pelo questionário (incluindo código)
		if g.approveContact != nil {
			approved, err := g.approveContact(ctx, msg.Channel, msg.From.DisplayName, msg.From.ID, msg.From.Username)
			if err != nil {
				log.Printf("[Gateway] trace=%s channel=%s erro ao solicitar pareamento: %v", traceID, msg.Channel, err)
				contacts.CancelPairingCode(msg.Channel, msg.From.ID)
				return
			}
			if !approved {
				log.Printf("[Gateway] trace=%s channel=%s pareamento recusado", traceID, msg.Channel)
				contacts.CancelPairingCode(msg.Channel, msg.From.ID)
				return
			}
			if err := contacts.Authorize(msg.Channel, msg.From.ID, msg.From.DisplayName, msg.From.Username, maxContacts); err != nil {
				log.Printf("[Gateway] trace=%s channel=%s erro ao autorizar contato: %v", traceID, msg.Channel, err)
				contacts.CancelPairingCode(msg.Channel, msg.From.ID)
				return
			}
			log.Printf("[Gateway] trace=%s channel=%s contato autorizado: %s", traceID, msg.Channel, maskIdentifier(msg.From.ID))
		} else {
			return
		}
	}

	// 2. Busca (ou cria) a conversa dedicada para este canal+contato.
	//    Primeiro verifica o config do canal (persistido entre reinícios),
	//    depois busca no DB por channel+contactID.
	conv, created, err := database.FindOrCreateChannelConversation(
		msg.Channel, msg.From.ID, msg.From.DisplayName,
	)
	if err != nil {
		log.Printf("[Gateway] trace=%s conv=? channel=%s contact=%s erro ao buscar/criar conversa: %v",
			traceID, msg.Channel, maskIdentifier(msg.From.ID), err)
		return
	}
	conversationID := conv.ID

	if created {
		log.Printf("[Gateway] trace=%s conv=%d channel=%s contact=%s nova conversa criada",
			traceID, conversationID, msg.Channel, maskIdentifier(msg.From.ID))
		// Persiste o mapeamento contactID → conversationID no config do canal
		if err := channels.SaveConversationID(msg.Channel, msg.From.ID, conversationID); err != nil {
			log.Printf("[Gateway] trace=%s conv=%d channel=%s erro ao persistir conversa no config: %v",
				traceID, conversationID, msg.Channel, err)
		}
	}
	log.Printf("[Gateway] trace=%s conv=%d channel=%s contact=%s msg=%s recebida",
		traceID, conversationID, msg.Channel, maskIdentifier(msg.From.ID), msg.ID)

	// 3. Converte attachments em media JSON (mesmo formato que o frontend)
	mediaJSON := ""
	if len(msg.Attachments) > 0 {
		mediaJSON = attachmentsToMediaJSON(msg.Attachments)
		log.Printf("[Gateway] trace=%s conv=%d attachments=%d convertidos para media JSON", traceID, conversationID, len(msg.Attachments))
	}

	// 4. Emite evento para o frontend
	hasAttachments := len(msg.Attachments) > 0
	if g.emitEvent != nil {
		g.emitEvent("messaging:incoming", map[string]any{
			"channel":         msg.Channel,
			"from":            msg.From.DisplayName,
			"fromId":          msg.From.ID,
			"text":            msg.Text,
			"messageId":       msg.ID,
			"conversationId":  conversationID,
			"newConversation": created,
			"hasAttachments":  hasAttachments,
			"audioOnly":       msg.IsAudioOnly(),
		})
	}

	// 6. Registra callback no Notifier para capturar a resposta e reenviar ao mensageiro.
	//    O ChannelResponseMode do perfil decide se a resposta será áudio ou texto.
	incomingIsAudio := msg.IsAudioOnly()
	g.notifier.Register(conversationID, ResponseCallback{
		Channel:   msg.Channel,
		ChatID:    msg.From.ID,
		AudioOnly: incomingIsAudio, // hint para o notifier (mantém compatibilidade)
		TraceID:   traceID,
		Callback: func(response string, assistantMsgID uint) {
			g.mu.RLock()
			messenger, ok := g.messengers[msg.Channel]
			g.mu.RUnlock()

			if !ok {
				log.Printf("[Gateway] trace=%s conv=%d channel=%s messenger não encontrado para resposta",
					traceID, conversationID, msg.Channel)
				return
			}

			outMsg := OutgoingMessage{
				ChatID:             msg.From.ID,
				Text:               response,
				ReplyToMessageID:   msg.ID,
				AssistantMessageID: assistantMsgID,
			}

			// Consulta cache TTS proativo (Phase 1 gera áudio ao salvar resposta).
			// Se já existe no DB, usa direto sem chamar synthesizeTTS.
			if assistantMsgID > 0 && g.getCachedAudio != nil {
				cachedBase64, cachedMime, cacheErr := g.getCachedAudio(assistantMsgID)
				if cacheErr == nil && cachedBase64 != "" {
					audioData, decErr := base64.StdEncoding.DecodeString(cachedBase64)
					if decErr == nil && len(audioData) > 0 {
						ext := "mp3"
						if cachedMime == "audio/wav" {
							ext = "wav"
						}
						outMsg.Attachments = []Attachment{{
							Filename: "resposta." + ext,
							MIMEType: cachedMime,
							Data:     audioData,
						}}
						outMsg.Text = ""
						log.Printf("[Gateway] trace=%s conv=%d msgID=%d cache hit TTS proativo bytes=%d mime=%s",
							traceID, conversationID, assistantMsgID, len(audioData), cachedMime)

						err := messenger.Send(ctx, outMsg)
						if err != nil {
							log.Printf("[Gateway] trace=%s conv=%d channel=%s erro ao enviar resposta (cache): %v",
								traceID, conversationID, msg.Channel, err)
						}
						return
					}
				}
			}

			// Consulta synthesizeTTS que resolve o perfil e o ChannelResponseMode
			// para decidir se gera áudio. Retorna (nil, nil) se deve enviar texto.
			if g.synthesizeTTS != nil {
				audioData, err := g.synthesizeTTS(response, msg.Channel, incomingIsAudio)
				if err == nil && len(audioData) > 0 {
					outMsg.Attachments = []Attachment{{
						Filename: "resposta.mp3",
						MIMEType: "audio/mpeg",
						Data:     audioData,
					}}
					outMsg.Text = ""
					log.Printf("[Gateway] trace=%s conv=%d channel=%s TTS gerado bytes=%d",
						traceID, conversationID, msg.Channel, len(audioData))

					// Salva o áudio TTS na mensagem do assistente no DB
					if assistantMsgID > 0 && g.saveAudio != nil {
						if err := g.saveAudio(assistantMsgID, base64.StdEncoding.EncodeToString(audioData), "audio/mpeg"); err != nil {
							log.Printf("[Gateway] trace=%s conv=%d msgID=%d erro ao salvar áudio TTS no DB: %v",
								traceID, conversationID, assistantMsgID, err)
						} else {
							log.Printf("[Gateway] trace=%s conv=%d msgID=%d áudio TTS salvo", traceID, conversationID, assistantMsgID)
						}
					}
				} else if err != nil {
					// Fallback explícito para texto
					if outMsg.Text == "" {
						outMsg.Text = response
					}
					log.Printf("[Gateway] trace=%s conv=%d channel=%s erro ao gerar TTS: %v (fallback texto)",
						traceID, conversationID, msg.Channel, err)
				} else {
					// TTS ignorado (perfil decidiu não gerar áudio) — garantir texto
					if outMsg.Text == "" {
						outMsg.Text = response
					}
					log.Printf("[Gateway] trace=%s conv=%d channel=%s TTS ignorado (fallback texto)",
						traceID, conversationID, msg.Channel)
				}
			}

			err := messenger.Send(ctx, outMsg)
			if err != nil {
				log.Printf("[Gateway] trace=%s conv=%d channel=%s erro ao enviar resposta: %v",
					traceID, conversationID, msg.Channel, err)
			} else {
				log.Printf("[Gateway] trace=%s conv=%d channel=%s resposta enviada", traceID, conversationID, msg.Channel)
			}
		},
	})

	// 7. Chama o mesmo SendMessage que o Wails usa (com o conversationID dedicado)
	//    Usa o perfil do canal (se configurado) em vez do perfil ativo global.
	params := llm.ChatParams{}
	if chCfg, _ := channels.Load(msg.Channel); chCfg != nil && chCfg.Profile != "" {
		params.ProfileSlug = chCfg.Profile
		log.Printf("[Gateway] trace=%s conv=%d channel=%s usando perfil=%s", traceID, conversationID, msg.Channel, chCfg.Profile)
	}
	_, err = g.sendMessage(conversationID, msg.Text, mediaJSON, params, msg.Channel)
	if err != nil {
		log.Printf("[Gateway] trace=%s conv=%d channel=%s erro ao processar mensagem: %v", traceID, conversationID, msg.Channel, err)
		g.mu.RLock()
		messenger, ok := g.messengers[msg.Channel]
		g.mu.RUnlock()
		if ok {
			messenger.Send(ctx, OutgoingMessage{
				ChatID: msg.From.ID,
				Text:   fmt.Sprintf("Erro ao processar mensagem: %v", err),
			})
		}
	}
}

// attachmentsToMediaJSON converte []Attachment para o formato media JSON
// usado pelo pipeline LLM (mesmo formato que o frontend envia).
// Formato: [{"name":"file.jpg","type":"image/jpeg","data":"base64...","size":1234}]
func attachmentsToMediaJSON(attachments []Attachment) string {
	var parts []map[string]interface{}
	for _, att := range attachments {
		parts = append(parts, map[string]interface{}{
			"name": att.Filename,
			"type": att.MIMEType,
			"data": base64.StdEncoding.EncodeToString(att.Data),
			"size": att.Size,
		})
	}
	data, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	return string(data)
}

func maskIdentifier(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	visible := value[len(value)-4:]
	return strings.Repeat("*", len(value)-4) + visible
}

