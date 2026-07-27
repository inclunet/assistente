package slack

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"assistente/internal/messaging"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

const (
	logComponent = "messaging.slack.adapter"

	// maxInboundFileBytes limita bytes brutos por arquivo no download inbound.
	// O gateway serializa anexos em media JSON com base64 (~4/3) e o chat valida
	// len(UserMedia) contra chat.MaxMediaSize (20 MiB). 14 MiB brutos ≈ 18,7 MiB
	// em base64, deixando folga para o envelope JSON (name/type/size) de um anexo.
	maxInboundFileBytes = 14 * 1024 * 1024
	maxInboundFiles     = 10
)

// fileAPI abstrai o download autenticado da Slack API (permite fake em testes).
type fileAPI interface {
	GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error
}

// SlackAdapter implementa messaging.Messenger para Slack via Socket Mode.
type SlackAdapter struct {
	botToken string
	appToken string

	api     *slack.Client
	fileAPI fileAPI
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
	s.fileAPI = api
	s.socket = socketClient
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.status = messaging.StatusConnected
	s.mu.Unlock()

	go s.eventLoop()
	go func() {
		if err := socketClient.RunContext(s.ctx); err != nil {
			logging.Errorf(ctx, logComponent, "[Slack] RunContext error: %v", err)
		}
	}()

	logging.Println(ctx, logComponent, "[Slack] Conectado via Socket Mode")
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
	logging.Println(context.Background(), logComponent, "[Slack] Desconectado")
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
			return fmt.Errorf("erro ao enviar anexo Slack (file=%q channel=%s size=%d thread=%s): %w",
				filename, msg.ChatID, len(att.Data), msg.ReplyToMessageID, err)
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
	if !shouldHandleMessage(ev) {
		return
	}

	handler := s.getHandler()
	if handler == nil {
		return
	}

	s.mu.RLock()
	api := s.fileAPI
	ctx := s.ctx
	s.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}

	// Copia campos do evento: download + handler rodam em goroutine para
	// não bloquear o event loop do Socket Mode.
	userID := ev.User
	channelID := ev.Channel
	text := ev.Text
	msgTS := ev.TimeStamp
	files := append([]slackevents.File(nil), ev.Files...)

	go func() {
		displayName := s.getUserDisplayName(userID)
		attachments := attachmentsFromSlackFiles(ctx, api, files)
		if text == "" && len(attachments) == 0 {
			return
		}

		handler(ctx, messaging.IncomingMessage{
			ID:          msgTS,
			Channel:     "slack",
			Text:        text,
			Attachments: attachments,
			Timestamp:   parseSlackTimestamp(msgTS),
			From: messaging.Contact{
				ID:          userID,
				Username:    userID,
				DisplayName: displayName,
			},
			ReplyChatID: channelID,
		})
	}()
}

// shouldHandleMessage filtra eventos que o adapter deve processar.
// Aceita mensagens normais (sem subtype) e file_share (upload de mídia).
func shouldHandleMessage(ev *slackevents.MessageEvent) bool {
	if ev == nil {
		return false
	}
	if ev.BotID != "" {
		return false
	}
	if ev.User == "" || ev.Channel == "" {
		return false
	}
	switch ev.SubType {
	case "", "file_share":
		return true
	default:
		return false
	}
}

// attachmentsFromSlackFiles baixa bytes autenticados e monta Attachments.
// Erros individuais são logados; não interrompem o processamento dos demais.
func attachmentsFromSlackFiles(ctx context.Context, api fileAPI, files []slackevents.File) []messaging.Attachment {
	if len(files) == 0 {
		return nil
	}
	if api == nil {
		logging.Errorf(ctx, logComponent, "[Slack] fileAPI ausente; %d arquivo(s) ignorado(s)", len(files))
		return nil
	}

	limit := len(files)
	if limit > maxInboundFiles {
		logging.Errorf(ctx, logComponent, "[Slack] mensagem com %d arquivos; processando só os %d primeiros", len(files), maxInboundFiles)
		limit = maxInboundFiles
	}

	var out []messaging.Attachment
	for i := 0; i < limit; i++ {
		f := files[i]
		att, err := attachmentFromSlackFile(ctx, api, f)
		if err != nil {
			logging.Errorf(ctx, logComponent, "[Slack] Anexo ignorado (id=%s name=%q mime=%q): %v",
				f.ID, f.Name, f.Mimetype, err)
			continue
		}
		if att != nil {
			out = append(out, *att)
		}
	}
	return out
}

func attachmentFromSlackFile(ctx context.Context, api fileAPI, f slackevents.File) (*messaging.Attachment, error) {
	if f.ID == "" && f.URLPrivateDownload == "" && f.URLPrivate == "" {
		return nil, fmt.Errorf("metadados de arquivo vazios")
	}
	if f.IsExternal {
		return nil, fmt.Errorf("arquivo externo não suportado")
	}
	if f.Size > maxInboundFileBytes {
		return nil, fmt.Errorf("arquivo grande demais (%d bytes; máx %d)", f.Size, maxInboundFileBytes)
	}

	downloadURL := firstNonEmpty(f.URLPrivateDownload, f.URLPrivate)
	if downloadURL == "" {
		return nil, fmt.Errorf("url de download ausente (requer scope files:read?)")
	}

	mime := strings.ToLower(strings.TrimSpace(f.Mimetype))
	if mime == "" || mime == "application/octet-stream" {
		if inferred := mimeFromFilename(f.Name); inferred != "" {
			mime = inferred
		}
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	if !isSupportedInboundMIME(mime) {
		return nil, fmt.Errorf("tipo MIME não suportado: %s", mime)
	}

	data, err := downloadSlackFile(ctx, api, downloadURL, maxInboundFileBytes)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("download retornou vazio")
	}

	filename := f.Name
	if filename == "" {
		filename = firstNonEmpty(f.Title, "attachment_"+f.ID)
	}
	if filename == "" {
		filename = "attachment"
	}

	size := int64(f.Size)
	if size <= 0 {
		size = int64(len(data))
	}

	return &messaging.Attachment{
		Filename: filename,
		MIMEType: mime,
		Data:     data,
		Size:     size,
	}, nil
}

func downloadSlackFile(ctx context.Context, api fileAPI, downloadURL string, maxBytes int64) ([]byte, error) {
	w := &maxBytesWriter{max: maxBytes}
	if err := api.GetFileContext(ctx, downloadURL, w); err != nil {
		return nil, fmt.Errorf("erro ao baixar arquivo: %w", err)
	}
	return w.Bytes(), nil
}

// maxBytesWriter rejeita writes que ultrapassem o limite.
type maxBytesWriter struct {
	buf     bytes.Buffer
	max     int64
	written int64
}

func (w *maxBytesWriter) Write(p []byte) (int, error) {
	if w.max > 0 && w.written+int64(len(p)) > w.max {
		return 0, fmt.Errorf("arquivo excede limite de %d bytes", w.max)
	}
	n, err := w.buf.Write(p)
	w.written += int64(n)
	return n, err
}

func (w *maxBytesWriter) Bytes() []byte {
	return w.buf.Bytes()
}

// supportedDocumentMIMEs é allowlist explícita (IsDocument em types.go é catch-all).
var supportedDocumentMIMEs = map[string]struct{}{
	"application/pdf": {},
	"application/msword": {},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
	"application/vnd.ms-excel": {},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": {},
	"application/vnd.ms-powerpoint": {},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {},
	"application/rtf": {},
	"application/json": {},
	"application/xml":  {},
	"text/plain":       {},
	"text/csv":         {},
	"text/markdown":    {},
}

// isSupportedInboundMIME aceita imagem/áudio/vídeo por prefixo e documentos por allowlist.
func isSupportedInboundMIME(mime string) bool {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "" {
		return false
	}
	a := messaging.Attachment{MIMEType: mime}
	if a.IsImage() || a.IsAudio() || a.IsVideo() {
		return true
	}
	_, ok := supportedDocumentMIMEs[mime]
	return ok
}

// extensionMIME maps extensão → MIME (package-level evita alocar a cada anexo).
var extensionMIME = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".gif": "image/gif", ".webp": "image/webp",
	".mp3": "audio/mpeg", ".ogg": "audio/ogg", ".oga": "audio/ogg",
	".wav": "audio/wav", ".aac": "audio/aac", ".m4a": "audio/mp4",
	".mp4": "video/mp4", ".webm": "video/webm",
	".pdf":  "application/pdf",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".md":   "text/markdown",
	".json": "application/json",
	".rtf":  "application/rtf",
	".xml":  "application/xml",
}

func mimeFromFilename(filename string) string {
	return extensionMIME[strings.ToLower(path.Ext(filename))]
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
