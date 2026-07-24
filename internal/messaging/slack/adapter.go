package slack

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"assistente/internal/messaging"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// SlackAdapter implementa messaging.Messenger para Slack via Socket Mode.
type SlackAdapter struct {
	botToken string
	appToken string

	api     *slack.Client
	socket  *socketmode.Client
	handler messaging.IncomingMessageHandler
	status  messaging.ConnectionStatus
	ctx     context.Context
	cancel  context.CancelFunc

	mu        sync.RWMutex
	userCache map[string]string
}

// NewAdapter cria um novo adapter para Slack (Socket Mode).
// botToken: xoxb-..., appToken: xapp-...
func NewAdapter(botToken, appToken string) *SlackAdapter {
	return &SlackAdapter{
		botToken:  botToken,
		appToken:  appToken,
		status:    messaging.StatusDisconnected,
		userCache: make(map[string]string),
	}
}

// Name retorna o identificador da plataforma.
func (s *SlackAdapter) Name() string {
	return "slack"
}

// Connect inicia a conexão com o Slack via Socket Mode.
func (s *SlackAdapter) Connect(ctx context.Context) error {
	s.mu.Lock()
	s.status = messaging.StatusConnecting
	s.mu.Unlock()

	if s.botToken == "" || s.appToken == "" {
		s.mu.Lock()
		s.status = messaging.StatusError
		s.mu.Unlock()
		return fmt.Errorf("tokens do Slack ausentes")
	}

	api := slack.New(
		s.botToken,
		slack.OptionAppLevelToken(s.appToken),
	)
	socketClient := socketmode.New(api)

	s.mu.Lock()
	s.api = api
	s.socket = socketClient
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.status = messaging.StatusConnected
	s.mu.Unlock()

	go s.eventLoop()
	go func() {
		if err := socketClient.RunContext(s.ctx); err != nil {
			logging.Errorf(ctx, "messaging.slack.adapter", "[Slack] RunContext error: %v", err)
		}
	}()

	logging.Println(ctx, "messaging.slack.adapter", "[Slack] Conectado via Socket Mode")
	return nil
}

// Disconnect encerra a conexão.
func (s *SlackAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.status = messaging.StatusDisconnected
	logging.Println(context.Background(), "messaging.slack.adapter", "[Slack] Desconectado")
	return nil
}

// Send envia uma mensagem de texto e anexos para um canal/chat do Slack.
func (s *SlackAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	s.mu.RLock()
	api := s.api
	s.mu.RUnlock()

	if api == nil {
		return fmt.Errorf("slack não está conectado")
	}

	for _, att := range msg.Attachments {
		if len(att.Data) == 0 {
			continue
		}
		filename := att.Filename
		if filename == "" {
			filename = "attachment"
		}
		// files.upload foi descontinuado; usar o fluxo V2
		// (getUploadURLExternal + completeUploadExternal).
		_, err := api.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
			Reader:          bytes.NewReader(att.Data),
			Filename:        filename,
			FileSize:        len(att.Data),
			Channel:         msg.ChatID,
			ThreadTimestamp: msg.ReplyToMessageID,
		})
		if err != nil {
			return fmt.Errorf("erro ao enviar anexo Slack: %w", err)
		}
	}

	text := msg.Text
	if text == "" {
		return nil
	}

	params := slack.PostMessageParameters{}
	if msg.ReplyToMessageID != "" {
		params.ThreadTimestamp = msg.ReplyToMessageID
	}

	_, _, err := api.PostMessageContext(ctx, msg.ChatID,
		slack.MsgOptionText(text, false),
		slack.MsgOptionPostMessageParameters(params),
	)
	return err
}

// SetHandler define o callback chamado quando uma mensagem chega.
func (s *SlackAdapter) SetHandler(handler messaging.IncomingMessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// Status retorna o estado atual da conexão.
func (s *SlackAdapter) Status() messaging.ConnectionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *SlackAdapter) eventLoop() {
	for evt := range s.socket.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			s.setStatus(messaging.StatusConnecting)
		case socketmode.EventTypeConnected:
			s.setStatus(messaging.StatusConnected)
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			s.socket.Ack(*evt.Request)

			if eventsAPIEvent.Type != slackevents.CallbackEvent {
				continue
			}

			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				s.handleMessage(ev)
			}
		}
	}
}

func (s *SlackAdapter) handleMessage(ev *slackevents.MessageEvent) {
	if ev == nil {
		return
	}
	if ev.SubType != "" || ev.BotID != "" {
		return
	}
	if ev.User == "" || ev.Channel == "" {
		return
	}

	handler := s.getHandler()
	if handler == nil {
		return
	}

	displayName := s.getUserDisplayName(ev.User)
	timestamp := parseSlackTimestamp(ev.TimeStamp)

	msg := messaging.IncomingMessage{
		ID:        ev.TimeStamp,
		Channel:   "slack",
		Text:      ev.Text,
		Timestamp: timestamp,
		From: messaging.Contact{
			ID:          ev.Channel,
			Username:    ev.User,
			DisplayName: displayName,
		},
	}

	handler(s.ctx, msg)
}

func (s *SlackAdapter) getHandler() messaging.IncomingMessageHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handler
}

func (s *SlackAdapter) setStatus(status messaging.ConnectionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

func (s *SlackAdapter) getUserDisplayName(userID string) string {
	s.mu.RLock()
	cached, ok := s.userCache[userID]
	api := s.api
	s.mu.RUnlock()
	if ok {
		return cached
	}
	if api == nil {
		return userID
	}

	user, err := api.GetUserInfo(userID)
	if err != nil {
		return userID
	}

	name := user.Profile.DisplayName
	if name == "" {
		name = user.Profile.RealName
	}
	if name == "" {
		name = user.Name
	}
	if name == "" {
		name = userID
	}

	s.mu.Lock()
	s.userCache[userID] = name
	s.mu.Unlock()

	return name
}

func parseSlackTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Now().UTC()
	}
	val, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return time.Now().UTC()
	}
	sec, dec := math.Modf(val)
	return time.Unix(int64(sec), int64(dec*1e9)).UTC()
}
