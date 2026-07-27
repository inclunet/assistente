package slack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/messaging"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type fakeFileAPI struct {
	mu      sync.Mutex
	files   map[string][]byte
	info    map[string]*slack.File // id -> files.info
	err     error
	infoErr error
	calls   []string
	infoIDs []string
	writeN  int // se > 0, escreve em chunks de writeN
}

func (f *fakeFileAPI) GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error {
	f.mu.Lock()
	f.calls = append(f.calls, downloadURL)
	err := f.err
	data := f.files[downloadURL]
	f.mu.Unlock()

	if err != nil {
		return err
	}
	if data == nil {
		return errors.New("arquivo não encontrado no fake")
	}
	if f.writeN > 0 {
		for len(data) > 0 {
			n := f.writeN
			if n > len(data) {
				n = len(data)
			}
			if _, werr := writer.Write(data[:n]); werr != nil {
				return werr
			}
			data = data[n:]
		}
		return nil
	}
	_, werr := writer.Write(data)
	return werr
}

func (f *fakeFileAPI) GetFileInfoContext(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
	f.mu.Lock()
	f.infoIDs = append(f.infoIDs, fileID)
	err := f.infoErr
	info := f.info[fileID]
	f.mu.Unlock()
	if err != nil {
		return nil, nil, nil, err
	}
	if info == nil {
		return nil, nil, nil, errors.New("files.info: não encontrado")
	}
	return info, nil, nil, nil
}

func TestShouldHandleMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ev   *slackevents.MessageEvent
		want bool
	}{
		{name: "nil", ev: nil, want: false},
		{name: "texto normal", ev: &slackevents.MessageEvent{User: "U1", Channel: "C1", Text: "oi"}, want: true},
		{name: "file_share", ev: &slackevents.MessageEvent{User: "U1", Channel: "C1", SubType: "file_share"}, want: true},
		{name: "bot", ev: &slackevents.MessageEvent{User: "U1", Channel: "C1", BotID: "B1"}, want: false},
		{name: "sem user", ev: &slackevents.MessageEvent{Channel: "C1"}, want: false},
		{name: "sem channel", ev: &slackevents.MessageEvent{User: "U1"}, want: false},
		{name: "message_changed", ev: &slackevents.MessageEvent{User: "U1", Channel: "C1", SubType: "message_changed"}, want: false},
		{name: "bot_message subtype", ev: &slackevents.MessageEvent{User: "U1", Channel: "C1", SubType: "bot_message"}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldHandleMessage(tt.ev); got != tt.want {
				t.Fatalf("shouldHandleMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAttachmentsFromSlackFiles_CapsAtMaxInboundFiles(t *testing.T) {
	t.Parallel()

	files := make([]slackevents.File, maxInboundFiles+3)
	apiFiles := map[string][]byte{}
	for i := range files {
		url := "https://files.slack.com/f" + strconv.Itoa(i) + ".png"
		apiFiles[url] = []byte("PNG")
		files[i] = slackevents.File{
			ID:                 "F" + strconv.Itoa(i),
			Name:               "f" + strconv.Itoa(i) + ".png",
			Mimetype:           "image/png",
			Size:               3,
			URLPrivateDownload: url,
		}
	}
	api := &fakeFileAPI{files: apiFiles}

	atts := attachmentsFromSlackFiles(context.Background(), api, files, nil)
	if len(atts) != maxInboundFiles {
		t.Fatalf("esperava %d anexos, got %d", maxInboundFiles, len(atts))
	}
	if len(api.calls) != maxInboundFiles {
		t.Fatalf("esperava %d downloads, got %d", maxInboundFiles, len(api.calls))
	}
}

func TestAttachmentsFromSlackFiles_DownloadsAuthenticatedBytes(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/img.png": []byte("PNGDATA"),
			"https://files.slack.com/doc.pdf": []byte("%PDF-1.4"),
		},
	}

	files := []slackevents.File{
		{
			ID:                 "F1",
			Name:               "foto.png",
			Mimetype:           "image/png",
			Size:               7,
			URLPrivateDownload: "https://files.slack.com/img.png",
		},
		{
			ID:                 "F2",
			Name:               "relatorio.pdf",
			Mimetype:           "application/pdf",
			Size:               8,
			URLPrivateDownload: "https://files.slack.com/doc.pdf",
		},
	}

	atts := attachmentsFromSlackFiles(context.Background(), api, files, nil)
	if len(atts) != 2 {
		t.Fatalf("esperava 2 anexos, got %d", len(atts))
	}
	if atts[0].Filename != "foto.png" || atts[0].MIMEType != "image/png" || string(atts[0].Data) != "PNGDATA" {
		t.Fatalf("anexo imagem inesperado: %+v data=%q", atts[0], atts[0].Data)
	}
	if !atts[0].IsImage() {
		t.Fatal("esperava IsImage")
	}
	if atts[1].Filename != "relatorio.pdf" || !atts[1].IsDocument() {
		t.Fatalf("anexo documento inesperado: %+v", atts[1])
	}
	if len(api.calls) != 2 {
		t.Fatalf("esperava 2 downloads, got %d (%v)", len(api.calls), api.calls)
	}
}

func TestAttachmentsFromSlackFiles_SkipsOversizedAndExternal(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/ok.jpg": []byte("JPEG"),
		},
	}

	files := []slackevents.File{
		{
			ID:                 "Fbig",
			Name:               "huge.bin",
			Mimetype:           "application/octet-stream",
			Size:               maxInboundFileBytes + 1,
			URLPrivateDownload: "https://files.slack.com/huge.bin",
		},
		{
			ID:         "Fext",
			Name:       "drive.doc",
			Mimetype:   "application/msword",
			IsExternal: true,
			URLPrivate: "https://files.slack.com/ext",
		},
		{
			ID:                 "Fok",
			Name:               "ok.jpg",
			Mimetype:           "image/jpeg",
			Size:               4,
			URLPrivateDownload: "https://files.slack.com/ok.jpg",
		},
	}

	atts := attachmentsFromSlackFiles(context.Background(), api, files, nil)
	if len(atts) != 1 {
		t.Fatalf("esperava 1 anexo (só o válido), got %d", len(atts))
	}
	if atts[0].Filename != "ok.jpg" {
		t.Fatalf("anexo inesperado: %s", atts[0].Filename)
	}
	if len(api.calls) != 1 || api.calls[0] != "https://files.slack.com/ok.jpg" {
		t.Fatalf("só deveria baixar o arquivo válido, calls=%v", api.calls)
	}
}

func TestAttachmentsFromSlackFiles_DownloadErrorDoesNotAbortOthers(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/a.png": []byte("A"),
			"https://files.slack.com/b.png": []byte("B"),
		},
	}
	// força erro só na primeira URL via wrapper
	failFirst := &selectiveFailAPI{inner: api, failURL: "https://files.slack.com/a.png"}

	files := []slackevents.File{
		{ID: "Fa", Name: "a.png", Mimetype: "image/png", URLPrivateDownload: "https://files.slack.com/a.png"},
		{ID: "Fb", Name: "b.png", Mimetype: "image/png", URLPrivateDownload: "https://files.slack.com/b.png"},
	}

	atts := attachmentsFromSlackFiles(context.Background(), failFirst, files, nil)
	if len(atts) != 1 || string(atts[0].Data) != "B" {
		t.Fatalf("esperava só o segundo anexo, got %+v", atts)
	}
}

type selectiveFailAPI struct {
	inner   *fakeFileAPI
	failURL string
}

func (s *selectiveFailAPI) GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error {
	if downloadURL == s.failURL {
		return errors.New("falha simulada")
	}
	return s.inner.GetFileContext(ctx, downloadURL, writer)
}

func (s *selectiveFailAPI) GetFileInfoContext(ctx context.Context, fileID string, count, page int) (*slack.File, []slack.Comment, *slack.Paging, error) {
	return s.inner.GetFileInfoContext(ctx, fileID, count, page)
}

func TestAttachmentsFromSlackFiles_RespectsTotalBudget(t *testing.T) {
	t.Parallel()

	half := maxInboundTotalBytes/2 + 1
	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/a.bin": bytes.Repeat([]byte("a"), half),
			"https://files.slack.com/b.bin": bytes.Repeat([]byte("b"), half),
		},
	}
	files := []slackevents.File{
		{ID: "Fa", Name: "a.pdf", Mimetype: "application/pdf", Size: half, URLPrivateDownload: "https://files.slack.com/a.bin"},
		{ID: "Fb", Name: "b.pdf", Mimetype: "application/pdf", Size: half, URLPrivateDownload: "https://files.slack.com/b.bin"},
	}
	atts := attachmentsFromSlackFiles(context.Background(), api, files, nil)
	if len(atts) != 1 {
		t.Fatalf("esperava 1 anexo dentro do teto agregado, got %d", len(atts))
	}
	if len(api.calls) != 1 {
		t.Fatalf("segundo download deveria ser evitado pelo Size, calls=%v", api.calls)
	}
}

func TestAttachmentFromSlackFile_ResolvesStubViaFilesInfo(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/resolved.png": []byte("PNG"),
		},
		info: map[string]*slack.File{
			"Fstub": {
				ID:                 "Fstub",
				Name:               "foto.png",
				Mimetype:           "image/png",
				Size:               3,
				URLPrivateDownload: "https://files.slack.com/resolved.png",
			},
		},
	}
	att, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{
		ID: "Fstub", // stub sem URL (Slack Connect)
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if att.Filename != "foto.png" || !att.IsImage() || string(att.Data) != "PNG" {
		t.Fatalf("attachment inesperado: %+v", att)
	}
	if len(api.infoIDs) != 1 || api.infoIDs[0] != "Fstub" {
		t.Fatalf("esperava files.info, got %v", api.infoIDs)
	}
}

func TestDownloadSlackFile_EnforcesMaxBytes(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files:  map[string][]byte{"https://x/big": bytes.Repeat([]byte("x"), 100)},
		writeN: 10,
	}
	_, err := downloadSlackFile(context.Background(), api, "https://x/big", 50)
	if err == nil || !strings.Contains(err.Error(), "excede limite") {
		t.Fatalf("esperava erro de limite, got %v", err)
	}
}

func TestMimeFromFilenameAndSupported(t *testing.T) {
	t.Parallel()

	if mimeFromFilename("voz.ogg") != "audio/ogg" {
		t.Fatal("ogg")
	}
	if mimeFromFilename("foto.JPEG") != "image/jpeg" {
		t.Fatal("jpeg case-insensitive")
	}
	if !isSupportedInboundMIME("audio/mpeg") || !isSupportedInboundMIME("image/png") ||
		!isSupportedInboundMIME("video/mp4") || !isSupportedInboundMIME("application/pdf") {
		t.Fatal("tipos suportados rejeitados")
	}
	if isSupportedInboundMIME("application/octet-stream") || isSupportedInboundMIME("application/x-msdownload") ||
		isSupportedInboundMIME("") {
		t.Fatal("tipos não suportados deveriam ser rejeitados")
	}
	if mimeFromFilename("nota.rtf") != "application/rtf" || mimeFromFilename("data.xml") != "application/xml" {
		t.Fatal("extensoes do allowlist devem mapear via mimeFromFilename")
	}
	if isSupportedInboundMIME("text/html") {
		t.Fatal("text/html nao deve ser aceito (conteudo ativo)")
	}
}

func TestAttachmentFromSlackFile_NormalizesMIMECase(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{"https://files.slack.com/pic.PNG": []byte("PNG")},
	}
	att, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{
		ID:                 "Fcase",
		Name:               "pic.PNG",
		Mimetype:           "IMAGE/PNG",
		URLPrivateDownload: "https://files.slack.com/pic.PNG",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if att.MIMEType != "image/png" {
		t.Fatalf("MIMEType=%q, want image/png (normalizado)", att.MIMEType)
	}
	if !att.IsImage() {
		t.Fatal("IsImage deve funcionar apos normalizacao")
	}
}

func TestAttachmentFromSlackFile_RejectsUnsupportedMIME(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{"https://files.slack.com/x.exe": []byte("MZ")},
	}
	_, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{
		ID:                 "Fexe",
		Name:               "setup.exe",
		Mimetype:           "application/x-msdownload",
		URLPrivateDownload: "https://files.slack.com/x.exe",
	})
	if err == nil || !strings.Contains(err.Error(), "tipo MIME não suportado") {
		t.Fatalf("esperava rejeição de MIME, got %v", err)
	}
	if len(api.calls) != 0 {
		t.Fatalf("não deveria baixar MIME rejeitado, calls=%v", api.calls)
	}
}

func TestAttachmentFromSlackFile_InfersMIMEAndFallbackURL(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/private": []byte("AUDIO"),
		},
	}
	att, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{
		ID:         "Faudio",
		Name:       "nota.m4a",
		Mimetype:   "application/octet-stream",
		URLPrivate: "https://files.slack.com/private",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if att.MIMEType != "audio/mp4" {
		t.Fatalf("mime inferido = %q, want audio/mp4", att.MIMEType)
	}
	if !att.IsAudio() || string(att.Data) != "AUDIO" {
		t.Fatalf("attachment inesperado: %+v", att)
	}
}

func TestHandleMessage_DropsWhenInboundSaturated(t *testing.T) {
	t.Parallel()

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = &fakeFileAPI{files: map[string][]byte{
		"https://files.slack.com/a.png": []byte("PNG"),
	}}
	s.ctx = context.Background()
	s.mu.Unlock()

	// Esgota o semáforo.
	for i := 0; i < maxInboundInFlight; i++ {
		s.inboundSem <- struct{}{}
	}

	called := make(chan struct{}, 1)
	s.SetHandler(func(ctx context.Context, msg messaging.IncomingMessage) {
		called <- struct{}{}
	})

	s.handleMessage(&slackevents.MessageEvent{
		User:    "U1",
		Channel: "C1",
		Text:    "capa",
		SubType: "file_share",
		Files: []slackevents.File{{
			ID:                 "F1",
			Name:               "a.png",
			Mimetype:           "image/png",
			URLPrivateDownload: "https://files.slack.com/a.png",
		}},
	})

	select {
	case <-called:
		t.Fatal("handler nao deveria ser chamado com inbound saturado")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestHandleMessage_TextBypassesInboundSemaphore(t *testing.T) {
	t.Parallel()

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = &fakeFileAPI{files: map[string][]byte{}}
	s.ctx = context.Background()
	s.mu.Unlock()

	for i := 0; i < maxInboundInFlight; i++ {
		s.inboundSem <- struct{}{}
	}

	done := make(chan messaging.IncomingMessage, 1)
	s.SetHandler(func(ctx context.Context, msg messaging.IncomingMessage) {
		done <- msg
	})

	s.handleMessage(&slackevents.MessageEvent{
		User:    "U1",
		Channel: "C1",
		Text:    "so texto",
	})

	select {
	case msg := <-done:
		if msg.Text != "so texto" {
			t.Fatalf("texto=%q", msg.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("texto puro nao deveria ser descartado pelo semaforo")
	}
}

func TestHandleMessage_FileSharePopulatesAttachments(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		files: map[string][]byte{
			"https://files.slack.com/voice.ogg": []byte("OGG"),
		},
	}

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = api
	s.ctx = context.Background()
	s.userCache["U123"] = "Alice"
	s.mu.Unlock()

	done := make(chan messaging.IncomingMessage, 1)
	s.SetHandler(func(ctx context.Context, msg messaging.IncomingMessage) {
		done <- msg
	})

	s.handleMessage(&slackevents.MessageEvent{
		User:     "U123",
		Channel:  "C999",
		Text:     "capa",
		SubType:  "file_share",
		TimeStamp: "1710000000.000100",
		Files: []slackevents.File{{
			ID:                 "Fvoice",
			Name:               "voice.ogg",
			Mimetype:           "audio/ogg",
			Size:               3,
			URLPrivateDownload: "https://files.slack.com/voice.ogg",
		}},
	})

	select {
	case msg := <-done:
		if msg.Text != "capa" || msg.Channel != "slack" || msg.ReplyChatID != "C999" {
			t.Fatalf("mensagem inesperada: %+v", msg)
		}
		if msg.From.DisplayName != "Alice" {
			t.Fatalf("displayName=%q", msg.From.DisplayName)
		}
		if len(msg.Attachments) != 1 || !msg.Attachments[0].IsAudio() {
			t.Fatalf("attachments=%+v", msg.Attachments)
		}
		if string(msg.Attachments[0].Data) != "OGG" {
			t.Fatalf("data=%q", msg.Attachments[0].Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout aguardando handler")
	}
}

func TestHandleMessage_SkipsEmptyWithoutAttachments(t *testing.T) {
	t.Parallel()

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = &fakeFileAPI{files: map[string][]byte{}}
	s.ctx = context.Background()
	s.mu.Unlock()

	called := make(chan struct{}, 1)
	s.SetHandler(func(ctx context.Context, msg messaging.IncomingMessage) {
		called <- struct{}{}
	})

	s.handleMessage(&slackevents.MessageEvent{
		User:    "U1",
		Channel: "C1",
		Text:    "",
		SubType: "file_share",
		Files: []slackevents.File{{
			ID:   "Fext",
			Name: "x",
			// sem URL → download falha → sem attachments
		}},
	})

	select {
	case <-called:
		t.Fatal("handler não deveria ser chamado sem texto nem anexos")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAttachmentsFromSlackFiles_NilAPI(t *testing.T) {
	t.Parallel()
	atts := attachmentsFromSlackFiles(context.Background(), nil, []slackevents.File{{
		ID: "F1", Name: "a.png", Mimetype: "image/png", URLPrivateDownload: "https://x",
	}}, nil)
	if len(atts) != 0 {
		t.Fatalf("esperava nil/vazio com api nil, got %d", len(atts))
	}
}

func TestIsMissingScopeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "generico", err: errors.New("timeout"), want: false},
		{name: "slack missing_scope", err: slack.SlackErrorResponse{Err: "missing_scope"}, want: true},
		{name: "slack not_allowed_token_type", err: slack.SlackErrorResponse{Err: "not_allowed_token_type"}, want: true},
		{name: "slack file_not_found", err: slack.SlackErrorResponse{Err: "file_not_found"}, want: false},
		{name: "http 401", err: slack.StatusCodeError{Code: http.StatusUnauthorized, Status: "401"}, want: true},
		{name: "http 403", err: slack.StatusCodeError{Code: http.StatusForbidden, Status: "403"}, want: true},
		{name: "http 500", err: slack.StatusCodeError{Code: http.StatusInternalServerError, Status: "500"}, want: false},
		{name: "wrapped missing_scope", err: fmt.Errorf("files.info: %w", slack.SlackErrorResponse{Err: "missing_scope"}), want: true},
		{name: "mensagem files:read", err: errors.New("url de download ausente (requer scope files:read?)"), want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isMissingScopeError(tt.err); got != tt.want {
				t.Fatalf("isMissingScopeError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAttachmentsFromSlackFiles_MissingScopeInvokesWarnAndSkips(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		infoErr: slack.SlackErrorResponse{Err: "missing_scope"},
	}
	files := []slackevents.File{
		{ID: "Fa", Name: "a.png", Mimetype: "image/png"},
		{ID: "Fb", Name: "b.png", Mimetype: "image/png"},
	}

	var warnCount int
	var mu sync.Mutex
	warn := func(ctx context.Context, err error) {
		mu.Lock()
		warnCount++
		mu.Unlock()
		if !isMissingScopeError(err) {
			t.Errorf("warn recebeu erro inesperado: %v", err)
		}
	}

	atts := attachmentsFromSlackFiles(context.Background(), api, files, warn)
	if len(atts) != 0 {
		t.Fatalf("esperava 0 anexos sem scope, got %d", len(atts))
	}
	mu.Lock()
	got := warnCount
	mu.Unlock()
	// Duas falhas → dois callbacks (dedupe via atomic.Bool fica no adapter;
	// aqui o callback de teste conta por chamada).
	if got != 2 {
		t.Fatalf("esperava 2 avisos de missing_scope, got %d", got)
	}
}

func TestProbeFilesReadScope_WarnsOnMissingScope(t *testing.T) {
	t.Parallel()

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = &fakeFileAPI{infoErr: slack.SlackErrorResponse{Err: "missing_scope"}}
	s.ctx = context.Background()
	s.mu.Unlock()

	s.probeFilesReadScope(context.Background())
	// Segunda chamada não deve panicar; atomic.Bool já marca como avisado.
	s.probeFilesReadScope(context.Background())
}

func TestProbeFilesReadScope_IgnoresFileNotFound(t *testing.T) {
	t.Parallel()

	s := NewAdapter("xoxb-test", "xapp-test")
	s.mu.Lock()
	s.fileAPI = &fakeFileAPI{infoErr: slack.SlackErrorResponse{Err: "file_not_found"}}
	s.ctx = context.Background()
	s.mu.Unlock()

	s.probeFilesReadScope(context.Background())
}

func TestAttachmentFromSlackFile_MissingScopeOnInfo(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{infoErr: slack.SlackErrorResponse{Err: "missing_scope"}}
	_, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{ID: "Fstub"})
	if err == nil || !isMissingScopeError(err) {
		t.Fatalf("esperava missing_scope, got %v", err)
	}
	if !strings.Contains(err.Error(), "files:read") {
		t.Fatalf("mensagem deveria citar files:read, got %v", err)
	}
}

func TestAttachmentFromSlackFile_MissingScopeOnDownload(t *testing.T) {
	t.Parallel()

	api := &fakeFileAPI{
		err: slack.StatusCodeError{Code: http.StatusForbidden, Status: "403 Forbidden"},
		files: map[string][]byte{
			"https://files.slack.com/x.png": []byte("PNG"),
		},
	}
	_, err := attachmentFromSlackFile(context.Background(), api, slackevents.File{
		ID:                 "F1",
		Name:               "x.png",
		Mimetype:           "image/png",
		URLPrivateDownload: "https://files.slack.com/x.png",
	})
	if err == nil || !isMissingScopeError(err) {
		t.Fatalf("esperava missing_scope via 403, got %v", err)
	}
}

func TestPostMessageOptions_SetsClientMsgID(t *testing.T) {
	t.Parallel()

	opts := postMessageOptions(slack.APIURL, "olá", "123.456", "trace-abc-uuid")
	_, values, err := slack.UnsafeApplyMsgOptions("xoxb-test", "C1", slack.APIURL,
		append([]slack.MsgOption{slack.MsgOptionPost()}, opts...)...)
	if err != nil {
		t.Fatalf("UnsafeApplyMsgOptions: %v", err)
	}
	if got := values.Get("client_msg_id"); got != "trace-abc-uuid" {
		t.Fatalf("client_msg_id=%q, want trace-abc-uuid", got)
	}
	if got := values.Get("text"); got != "olá" {
		t.Fatalf("text=%q, want olá", got)
	}
	if got := values.Get("thread_ts"); got != "123.456" {
		t.Fatalf("thread_ts=%q, want 123.456", got)
	}
}

func TestPostMessageOptions_OmitsClientMsgIDWhenEmpty(t *testing.T) {
	t.Parallel()

	opts := postMessageOptions(slack.APIURL, "sem chave", "", "  ")
	_, values, err := slack.UnsafeApplyMsgOptions("xoxb-test", "C1", slack.APIURL,
		append([]slack.MsgOption{slack.MsgOptionPost()}, opts...)...)
	if err != nil {
		t.Fatalf("UnsafeApplyMsgOptions: %v", err)
	}
	if got := values.Get("client_msg_id"); got != "" {
		t.Fatalf("client_msg_id deveria estar vazio, got %q", got)
	}
}

func TestSlackAdapter_Send_IncludesClientMsgID(t *testing.T) {
	t.Parallel()

	var gotClientMsgID string
	var gotChannel string
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotClientMsgID = r.Form.Get("client_msg_id")
		gotChannel = r.Form.Get("channel")
		gotText = r.Form.Get("text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C99","ts":"1.2"}`))
	}))
	t.Cleanup(srv.Close)

	base := srv.URL + "/"
	api := slack.New("xoxb-test", slack.OptionAPIURL(base))
	adapter := &SlackAdapter{
		api:        api,
		apiBaseURL: base,
		status:     messaging.StatusConnected,
	}

	err := adapter.Send(context.Background(), messaging.OutgoingMessage{
		ChatID:         "C99",
		Text:           "dedup please",
		IdempotencyKey: "550e8400-e29b-41d4-a716-446655440000",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotClientMsgID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("client_msg_id=%q, want UUID do IdempotencyKey", gotClientMsgID)
	}
	if gotChannel != "C99" || gotText != "dedup please" {
		t.Fatalf("channel=%q text=%q", gotChannel, gotText)
	}
}
