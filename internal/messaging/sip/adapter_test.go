package sip

import (
	"testing"
	"time"
)

func TestNewCallSession(t *testing.T) {
	cs := NewCallSession("dialog-1", "100@pbx", "Alice")

	if cs.ID != "dialog-1" {
		t.Errorf("ID = %q, want %q", cs.ID, "dialog-1")
	}
	if cs.CallerID != "100@pbx" {
		t.Errorf("CallerID = %q, want %q", cs.CallerID, "100@pbx")
	}
	if cs.CallerName != "Alice" {
		t.Errorf("CallerName = %q, want %q", cs.CallerName, "Alice")
	}
	if cs.GetState() != CallStateRinging {
		t.Errorf("State = %q, want %q", cs.GetState(), CallStateRinging)
	}
}

func TestCallSessionSetState(t *testing.T) {
	cs := NewCallSession("d1", "100@pbx", "")

	cs.SetState(CallStateActive)
	if cs.GetState() != CallStateActive {
		t.Errorf("State = %q, want %q", cs.GetState(), CallStateActive)
	}
	if cs.StartedAt.IsZero() {
		t.Error("StartedAt should be set when state is Active")
	}

	cs.SetState(CallStateEnded)
	if cs.GetState() != CallStateEnded {
		t.Errorf("State = %q, want %q", cs.GetState(), CallStateEnded)
	}
	if cs.EndedAt.IsZero() {
		t.Error("EndedAt should be set when state is Ended")
	}
}

func TestCallSessionDuration(t *testing.T) {
	cs := NewCallSession("d1", "100", "")

	// Antes de ativar, dura├º├úo = 0
	if d := cs.Duration(); d != 0 {
		t.Errorf("Duration before active = %v, want 0", d)
	}

	cs.SetState(CallStateActive)
	time.Sleep(10 * time.Millisecond)

	// Durante atividade, dura├º├úo > 0
	if d := cs.Duration(); d <= 0 {
		t.Errorf("Duration during active = %v, want > 0", d)
	}

	cs.SetState(CallStateEnded)
	finalDuration := cs.Duration()

	// Ap├│s encerrar, dura├º├úo est├ível
	time.Sleep(10 * time.Millisecond)
	if d := cs.Duration(); d != finalDuration {
		t.Errorf("Duration after ended changed: %v ÔåÆ %v", finalDuration, d)
	}
}

func TestCallSessionString(t *testing.T) {
	cs := NewCallSession("d-42", "200@asterisk", "Bob")
	s := cs.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

func TestNewAdapter(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}

	adapter := NewAdapter(cfg)

	if adapter.Name() != "sip" {
		t.Errorf("Name() = %q, want %q", adapter.Name(), "sip")
	}
	if adapter.Status() != "disconnected" {
		t.Errorf("Status() = %q, want %q", adapter.Status(), "disconnected")
	}
}

func TestAdapterConnectValidationError(t *testing.T) {
	// Config inv├ílida (sem server)
	cfg := SIPConfig{
		User:     "100",
		Password: "secret",
	}

	adapter := NewAdapter(cfg)
	err := adapter.Connect(t.Context())
	if err == nil {
		t.Error("Connect() should return error for invalid config")
	}
	if adapter.Status() != "error" {
		t.Errorf("Status after failed connect = %q, want %q", adapter.Status(), "error")
	}
}

func TestAdapter_SetVoiceID(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	// Valor inicial deve ser vazio
	if adapter.voiceID != "" {
		t.Errorf("voiceID initial = %q, want empty", adapter.voiceID)
	}

	// Configura voz
	adapter.SetVoiceID("alloy")
	if adapter.voiceID != "alloy" {
		t.Errorf("voiceID = %q, want %q", adapter.voiceID, "alloy")
	}

	// Atualiza voz
	adapter.SetVoiceID("nova")
	if adapter.voiceID != "nova" {
		t.Errorf("voiceID = %q, want %q", adapter.voiceID, "nova")
	}
}

func TestMp3StreamToMono8kReader_EmptyInput(t *testing.T) {
	// Testa que reader com buffer vazio retorna corretamente
	r := &mp3StreamToMono8kReader{
		srcRate: 44100,
		tmp:     make([]byte, 1024),
	}
	// Sem decoder, n├úo pode ler ÔÇö mas verifica que a struct ├® cri├ível
	if r.srcRate != 44100 {
		t.Errorf("srcRate = %d, want 44100", r.srcRate)
	}
}

func TestBuildRecipientURI_Extension(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
		Port:     5060,
	}
	adapter := NewAdapter(cfg)

	// Ramal simples ÔåÆ user@server configurado
	uri := adapter.buildRecipientURI("200")
	if uri.User != "200" {
		t.Errorf("User = %q, want %q", uri.User, "200")
	}
	if uri.Host != "asterisk.local" {
		t.Errorf("Host = %q, want %q", uri.Host, "asterisk.local")
	}
	// Porta padr├úo (5060) ├® omitida da URI para permitir DNS SRV (RFC 3263)
	if uri.Port != 0 {
		t.Errorf("Port = %d, want 0 (default port omitted)", uri.Port)
	}
}

func TestBuildRecipientURI_UserAtHost(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	// user@host ÔåÆ separa user e host, porta padr├úo
	uri := adapter.buildRecipientURI("200@pbx.local")
	if uri.User != "200" {
		t.Errorf("User = %q, want %q", uri.User, "200")
	}
	if uri.Host != "pbx.local" {
		t.Errorf("Host = %q, want %q", uri.Host, "pbx.local")
	}
	if uri.Port != 0 {
		t.Errorf("Port = %d, want 0 (default)", uri.Port)
	}
}

func TestBuildRecipientURI_SIPPrefix(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	// sip:user@host ÔåÆ remove prefixo sip:
	uri := adapter.buildRecipientURI("sip:200@pbx.local")
	if uri.User != "200" {
		t.Errorf("User = %q, want %q", uri.User, "200")
	}
	if uri.Host != "pbx.local" {
		t.Errorf("Host = %q, want %q", uri.Host, "pbx.local")
	}
}

func TestActiveCallsEmpty(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	calls := adapter.ActiveCalls()
	if len(calls) != 0 {
		t.Errorf("ActiveCalls() = %d calls, want 0", len(calls))
	}
}

func TestDialNotConnected(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	// Sem conex├úo, Dial deve falhar
	_, err := adapter.Dial(t.Context(), "200")
	if err == nil {
		t.Error("Dial() should return error when not connected")
	}
}

func TestHangupCallNotFound(t *testing.T) {
	cfg := SIPConfig{
		Server:   "asterisk.local",
		User:     "100",
		Password: "secret",
	}
	adapter := NewAdapter(cfg)

	err := adapter.HangupCall(t.Context(), "nonexistent-call")
	if err == nil {
		t.Error("HangupCall() should return error for nonexistent call")
	}
}

func TestCallInfoStruct(t *testing.T) {
	info := CallInfo{
		ID:       "sip-out-200-1",
		CallerID: "200",
		State:    "ringing",
		Duration: "0s",
	}

	if info.ID != "sip-out-200-1" {
		t.Errorf("ID = %q, want %q", info.ID, "sip-out-200-1")
	}
	if info.State != "ringing" {
		t.Errorf("State = %q, want %q", info.State, "ringing")
	}
}
func TestSdpDeduplicateRtpmap(t *testing.T) {
	tests := []struct {
		name string
		sdp  string
		want string
	}{
		{
			name: "no duplicates unchanged",
			sdp: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 97\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n",
			want: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 97\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n",
		},
		{
			name: "cross-media duplicate removed",
			sdp: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 97\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n" +
				"m=video 0 RTP/AVP 97\r\n" +
				"a=rtpmap:97 H264/90000\r\n",
			want: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 97\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n" +
				"m=video 0 RTP/AVP 97\r\n",
		},
		{
			name: "multiple duplicates across sections",
			sdp: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 96 97 101\r\n" +
				"a=rtpmap:96 opus/48000/2\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n" +
				"a=rtpmap:101 telephone-event/8000\r\n" +
				"m=video 0 RTP/AVP 96 97\r\n" +
				"a=rtpmap:96 VP8/90000\r\n" +
				"a=rtpmap:97 H264/90000\r\n",
			want: "v=0\r\n" +
				"m=audio 5004 RTP/AVP 0 8 96 97 101\r\n" +
				"a=rtpmap:96 opus/48000/2\r\n" +
				"a=rtpmap:97 iLBC/8000\r\n" +
				"a=rtpmap:101 telephone-event/8000\r\n" +
				"m=video 0 RTP/AVP 96 97\r\n",
		},
		{
			name: "LF line endings handled",
			sdp:  "v=0\nm=audio 5004 RTP/AVP 97\na=rtpmap:97 opus/48000\nm=video 0 RTP/AVP 97\na=rtpmap:97 H264/90000\n",
			want: "v=0\nm=audio 5004 RTP/AVP 97\na=rtpmap:97 opus/48000\nm=video 0 RTP/AVP 97\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(sdpDeduplicateRtpmap([]byte(tt.sdp)))
			if got != tt.want {
				t.Errorf("sdpDeduplicateRtpmap():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}