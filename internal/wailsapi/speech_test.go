package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/apidto"
)

func TestSpeechNotWired(t *testing.T) {
	t.Parallel()
	api := NewSpeech()
	if err := api.InitSpeechManagerFromProfile(); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("InitSpeechManagerFromProfile: got %v", err)
	}
	if _, err := api.TranscribeWhisper("a", "f"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("TranscribeWhisper: got %v", err)
	}
	if _, err := api.SynthesizeOpenAI("t"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SynthesizeOpenAI: got %v", err)
	}
	if _, err := api.SynthesizeOpenAIWithVoice("t", "v"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SynthesizeOpenAIWithVoice: got %v", err)
	}
	if err := api.SynthesizeOpenAIStream("t", "v", "s"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SynthesizeOpenAIStream: got %v", err)
	}
	if _, err := api.GetOpenAITTSVoices(); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetOpenAITTSVoices: got %v", err)
	}
	if err := api.SetOpenAITTSVoice("v"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SetOpenAITTSVoice: got %v", err)
	}
	if err := api.SetOpenAITTSSpeed(1); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SetOpenAITTSSpeed: got %v", err)
	}
	if _, err := api.GetMessageAudio("id"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetMessageAudio: got %v", err)
	}
	if err := api.SaveMessageAudio("id", "a", "audio/mpeg"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SaveMessageAudio: got %v", err)
	}
	if _, err := api.GenerateAndSaveMessageAudio("id", "t"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GenerateAndSaveMessageAudio: got %v", err)
	}
	if _, err := api.SpeakMessage("id", "p", "m", "v", 1, ""); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SpeakMessage: got %v", err)
	}
	if _, err := api.GetSpeechProviders(); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetSpeechProviders: got %v", err)
	}
	if _, err := api.GetTTSModels("p"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetTTSModels: got %v", err)
	}
	if _, err := api.GetTTSVoices("p", "m"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetTTSVoices: got %v", err)
	}
	if _, err := api.GetSTTModels("p"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetSTTModels: got %v", err)
	}
	if err := api.SpeakPreview("p", "m", "v", 1, 1, "", "t", "s"); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("SpeakPreview: got %v", err)
	}
	if err := api.DispatchSpeech(apidto.ChatSpeakRequest{}); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("DispatchSpeech: got %v", err)
	}
}

func TestSpeechNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewSpeech()
	AttachSpeech(api, stubSession{}, nil, stubSpeechDispatcher{})
	if _, err := api.GetSpeechProviders(); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("GetSpeechProviders com ctrl nil: got %v", err)
	}
}

func TestSpeechNilDispatcherIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewSpeech()
	AttachSpeech(api, stubSession{}, controllers.NewSpeechController(controllers.SpeechControllerConfig{}), nil)
	if err := api.DispatchSpeech(apidto.ChatSpeakRequest{}); !errors.Is(err, ErrSpeechNotWired) {
		t.Fatalf("DispatchSpeech com dispatcher nil: got %v", err)
	}
}

type stubSpeechDispatcher struct {
	err error
}

func (s stubSpeechDispatcher) DispatchSpeech(apidto.ChatSpeakRequest) error {
	return s.err
}

// TestSpeechUsesWithUserNotRequireAuth cobre o fail-closed da borda.
func TestSpeechUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewSpeech()
	AttachSpeech(
		api,
		stubSession{err: semAuth},
		controllers.NewSpeechController(controllers.SpeechControllerConfig{}),
		stubSpeechDispatcher{},
	)

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"InitSpeechManagerFromProfile", func() error {
			return api.InitSpeechManagerFromProfile()
		}},
		{"TranscribeWhisper", func() error {
			_, err := api.TranscribeWhisper("a", "f")
			return err
		}},
		{"SynthesizeOpenAI", func() error {
			_, err := api.SynthesizeOpenAI("t")
			return err
		}},
		{"SynthesizeOpenAIWithVoice", func() error {
			_, err := api.SynthesizeOpenAIWithVoice("t", "v")
			return err
		}},
		{"SynthesizeOpenAIStream", func() error {
			return api.SynthesizeOpenAIStream("t", "v", "s")
		}},
		{"GetOpenAITTSVoices", func() error {
			_, err := api.GetOpenAITTSVoices()
			return err
		}},
		{"SetOpenAITTSVoice", func() error {
			return api.SetOpenAITTSVoice("v")
		}},
		{"SetOpenAITTSSpeed", func() error {
			return api.SetOpenAITTSSpeed(1)
		}},
		{"GetMessageAudio", func() error {
			_, err := api.GetMessageAudio("id")
			return err
		}},
		{"SaveMessageAudio", func() error {
			return api.SaveMessageAudio("id", "a", "audio/mpeg")
		}},
		{"GenerateAndSaveMessageAudio", func() error {
			_, err := api.GenerateAndSaveMessageAudio("id", "t")
			return err
		}},
		{"SpeakMessage", func() error {
			_, err := api.SpeakMessage("id", "p", "m", "v", 1, "")
			return err
		}},
		{"GetSpeechProviders", func() error {
			_, err := api.GetSpeechProviders()
			return err
		}},
		{"GetTTSModels", func() error {
			_, err := api.GetTTSModels("p")
			return err
		}},
		{"GetTTSVoices", func() error {
			_, err := api.GetTTSVoices("p", "m")
			return err
		}},
		{"GetSTTModels", func() error {
			_, err := api.GetSTTModels("p")
			return err
		}},
		{"SpeakPreview", func() error {
			return api.SpeakPreview("p", "m", "v", 1, 1, "", "t", "s")
		}},
		{"DispatchSpeech", func() error {
			return api.DispatchSpeech(apidto.ChatSpeakRequest{Text: "oi", Role: "assistant"})
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}

func TestSpeechUsesWithUserNotRequireAuthSource(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "speech.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("speech.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("speech.go deve chamar WithUser(session,")
	}
}
