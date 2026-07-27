package slack

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/messaging"

	"github.com/slack-go/slack/slackevents"
)

type fakeFileAPI struct {
	mu      sync.Mutex
	files   map[string][]byte
	err     error
	calls   []string
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

	atts := attachmentsFromSlackFiles(context.Background(), api, files)
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

	atts := attachmentsFromSlackFiles(context.Background(), api, files)
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

	atts := attachmentsFromSlackFiles(context.Background(), api, files)
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

	atts := attachmentsFromSlackFiles(context.Background(), failFirst, files)
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
	}})
	if len(atts) != 0 {
		t.Fatalf("esperava nil/vazio com api nil, got %d", len(atts))
	}
}
