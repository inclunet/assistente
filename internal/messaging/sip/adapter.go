package sip

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"assistente/internal/messaging"
	"assistente/internal/speech"

	"github.com/emiago/diago"
	diagomedia "github.com/emiago/diago/media"
	"github.com/emiago/sipgo"
	siplib "github.com/emiago/sipgo/sip"
	mp3 "github.com/hajimehoshi/go-mp3"
)

// sipScannerUserAgents cont├®m fragmentos de User-Agent de scanners SIP conhecidos.
// Esses bots enviam SDP malformado que causa panic na lib diago.
var sipScannerUserAgents = []string{
	"sipper",
	"friendly-scanner",
	"sipvicious",
	"sundayddr",
	"iwar",
	"sip-scan",
	"sipsak",
	"nmap",
}

// sdpSanitizeMiddleware ├® um middleware sipgo que corrige SDPs malformados antes
// que o diago os processe. O parser SDP do diago (media/sdp.Unmarshal) armazena
// TODAS as linhas "a=" de todas as se├º├Áes de m├¡dia (audio, video, etc) em um
// mapa plano. CodecsFromSDPRead itera sobre esses atributos sem break ap├│s match,
// ent├úo se duas se├º├Áes de m├¡dia usam o mesmo payload type din├ómico (ex: 97),
// o rtpmap aparece duplicado e causa array bounds overflow (panic).
// Workaround: deduplicar a=rtpmap por payload type no SDP do INVITE.
func sdpSanitizeMiddleware(next sipgo.RequestHandler) sipgo.RequestHandler {
	return func(req *siplib.Request, tx siplib.ServerTransaction) {
		if req.IsInvite() {
			if body := req.Body(); len(body) > 0 {
				sanitized := sdpDeduplicateRtpmap(body)
				if len(sanitized) != len(body) {
					req.SetBody(sanitized)
				}
			}
		}
		next(req, tx)
	}
}

// sdpDeduplicateRtpmap remove entradas a=rtpmap duplicadas para o mesmo payload
// type number. Mant├®m apenas a primeira ocorr├¬ncia de cada PT.
func sdpDeduplicateRtpmap(body []byte) []byte {
	// Determina o separador de linha (CRLF ou LF)
	sep := "\r\n"
	if !bytes.Contains(body, []byte("\r\n")) {
		sep = "\n"
	}
	lines := strings.Split(string(body), sep)
	seen := make(map[string]bool)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "a=rtpmap:") {
			rest := line[len("a=rtpmap:"):]
			if sp := strings.IndexByte(rest, ' '); sp > 0 {
				pt := rest[:sp]
				if seen[pt] {
					continue // drop duplicate
				}
				seen[pt] = true
			}
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, sep))
}

// SIPAdapter implementa messaging.Messenger para comunica├º├úo via SIP.
// Permite receber e iniciar chamadas telef├┤nicas com pipeline de ├íudio bidirecional.
type SIPAdapter struct {
	config        SIPConfig
	ua            *sipgo.UserAgent
	dg            *diago.Diago
	handler       messaging.IncomingMessageHandler
	status        messaging.ConnectionStatus
	calls         map[string]*CallSession // dialogID ÔåÆ CallSession
	speechManager *speech.SpeechManager
	voiceID       string // voz para TTS streaming (vazio = padr├úo do provider)
	pipelineCfg   AudioPipelineConfig
	proxyHost     string // IP:port fixo resolvido via SRV (evita round-robin)

	// CancelStreamingForContact cancela o streaming LLM em andamento para um contato SIP.
	// Configurado pelo App. Recebe channel ("sip") e contactID (callerID).
	CancelStreamingForContact func(channel, contactID string)

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewAdapter cria um novo SIPAdapter com a configura├º├úo fornecida.
func NewAdapter(cfg SIPConfig) *SIPAdapter {
	return &SIPAdapter{
		config:      cfg,
		status:      messaging.StatusDisconnected,
		calls:       make(map[string]*CallSession),
		pipelineCfg: DefaultAudioPipelineConfig(),
	}
}

// SetSpeechManager configura o SpeechManager usado para STT/TTS no pipeline de ├íudio.
func (s *SIPAdapter) SetSpeechManager(sm *speech.SpeechManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speechManager = sm
}

// SetVoiceID configura a voz usada para streaming TTS.
// Se vazio, usa a voz padr├úo do provider.
func (s *SIPAdapter) SetVoiceID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.voiceID = id
}

// Name retorna o identificador da plataforma.
func (s *SIPAdapter) Name() string {
	return "sip"
}

// SetHandler define o callback chamado quando uma mensagem (chamada) chega.
func (s *SIPAdapter) SetHandler(handler messaging.IncomingMessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// Status retorna o estado atual da conex├úo SIP.
func (s *SIPAdapter) Status() messaging.ConnectionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// Connect registra no servidor SIP e come├ºa a receber chamadas.
func (s *SIPAdapter) Connect(ctx context.Context) error {
	if err := s.config.Validate(); err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("sip: configura├º├úo inv├ílida: %w", err)
	}

	s.setStatus(messaging.StatusConnecting)

	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	// Configura loggers para as libs diago/sipgo.
	// diago/media loga RTCP Unmarshal errors e sequence duplicates a cada poucos
	// segundos durante chamadas normais WebRTCÔåÆSIP. Esses s├úo inofensivos e poluem
	// o log. N├¡vel acima de ERROR suprime esse ru├¡do sem perder warnings do sipgo.
	mediaLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	diagomedia.SetDefaultLogger(mediaLogger)
	// sipgo mant├®m logger padr├úo para manter diagn├│sticos de transporte/DNS.

	// Cria o User Agent SIP
	// O nome do UA ├® usado pelo sipgo como User na From URI do REGISTER.
	// O hostname do UA ├® usado como Host na From URI.
	// Sem estes valores, o sipgo gera headers com campos vazios ÔåÆ 400 Invalid SIP MSG.
	// DNS SRV habilitado para resolver provedores SIP que usam SRV records
	// (ex: sip2sip.info ÔåÆ _sip._udp.sip2sip.info ÔåÆ proxy.sipthor.net).
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(s.config.User),
		sipgo.WithUserAgentHostname(s.config.Server),
		sipgo.WithUserAgentTransportLayerOptions(
			siplib.WithTransportLayerDNSLookupSRV(true),
		),
	)
	if err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("sip: erro ao criar UA: %w", err)
	}

	s.mu.Lock()
	s.ua = ua
	s.mu.Unlock()

	// Cria o Diago (framework VOIP de alto n├¡vel)
	// BindPort 0 ÔåÆ porta ef├¬mera: evita SIP ALG de roteadores que interceptam porta 5060.
	// Softphones fazem o mesmo ÔÇö usam portas aleat├│rias.
	transport := diago.Transport{
		Transport: s.config.GetTransport(),
		BindHost:  s.config.GetLocalIP(),
		BindPort:  0,
	}
	dg := diago.NewDiago(ua,
		diago.WithTransport(transport),
		diago.WithServerRequestMiddleware(sdpSanitizeMiddleware),
		diago.WithLogger(mediaLogger),
	)

	s.mu.Lock()
	s.dg = dg
	s.mu.Unlock()

	// Inicia o listener de chamadas SIP em background.
	// ServeBackground espera o listener UDP estar pronto antes de retornar.
	// Isso garante que o REGISTER subsequente use o mesmo socket do listener,
	// permitindo receber respostas do servidor.
	if err := dg.ServeBackground(s.ctx, func(inDialog *diago.DialogServerSession) {
		s.handleIncomingCall(inDialog)
	}); err != nil {
		s.setStatus(messaging.StatusError)
		return fmt.Errorf("sip: erro ao iniciar listener: %w", err)
	}

	s.setStatus(messaging.StatusConnected)
	log.Printf("[SIP] Conectado como %s@%s:%d (%s)",
		s.config.User, s.config.Server, s.config.GetPort(), s.config.GetTransport())
	log.Printf("[SIP] Aguardando chamadas...")

	// Registra no servidor SIP (como ramal) em goroutine separada ÔÇö
	// diago.Register ├® blocking e mant├®m o registro renovado.
	go s.registerLoop()

	return nil
}

// Disconnect encerra a conex├úo SIP: desliga chamadas ativas, cancela registro.
func (s *SIPAdapter) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancela o contexto (para o registerLoop e listener)
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.ctx = nil

	// Fecha o UA
	if s.ua != nil {
		s.ua.Close()
		s.ua = nil
	}

	s.dg = nil
	s.status = messaging.StatusDisconnected
	log.Printf("[SIP] Desconectado")

	return nil
}

// Send envia uma mensagem (├íudio TTS) para uma chamada ativa.
// msg.ChatID deve ser o CallerID (identificador da chamada).
// Se h├í attachments de ├íudio, reproduz na chamada via playback.
func (s *SIPAdapter) Send(ctx context.Context, msg messaging.OutgoingMessage) error {
	s.mu.RLock()
	call := s.findCallByCallerID(msg.ChatID)
	s.mu.RUnlock()

	if call == nil {
		return fmt.Errorf("sip: chamada n├úo encontrada para %s", msg.ChatID)
	}

	if call.GetState() != CallStateActive {
		return fmt.Errorf("sip: chamada %s n├úo est├í ativa (estado: %s)", msg.ChatID, call.GetState())
	}

	// Reproduz attachments de ├íudio na chamada
	audioPlayed := false
	for _, att := range msg.Attachments {
		if !att.IsAudio() || len(att.Data) == 0 {
			continue
		}

		// Converte para PCM 8kHz para o codec G.711
		pcm8k, err := convertToPCM8k(att.Data, att.MIMEType)
		if err != nil {
			log.Printf("[SIP] Erro ao converter ├íudio para playback: %v", err)
			continue
		}

		if call.Pipeline != nil {
			completed, err := call.Pipeline.PlayAudio(pcm8k)
			if err != nil {
				log.Printf("[SIP] Erro no playback para %s: %v", msg.ChatID, err)
			}
			if !completed {
				log.Printf("[SIP] Playback interrompido (barge-in) para %s", msg.ChatID)
			}
		} else if call.Dialog != nil {
			// Fallback: playback direto sem pipeline
			playback, err := call.Dialog.PlaybackCreate()
			if err != nil {
				log.Printf("[SIP] Erro ao criar playback: %v", err)
				continue
			}
			_, err = playback.Play(bytes.NewReader(pcm8k), "audio/pcm")
			if err != nil {
				log.Printf("[SIP] Erro no playback direto para %s: %v", msg.ChatID, err)
			}
		}
		audioPlayed = true
	}

	// Streaming TTS: texto sem ├íudio ÔåÆ sintetiza via streaming com menor lat├¬ncia.
	// Primeiro tenta streaming (chunks progressivos), fallback para batch se necess├írio.
	if !audioPlayed && msg.Text != "" && call.Pipeline != nil {
		s.mu.RLock()
		voiceID := s.voiceID
		sm := s.speechManager
		s.mu.RUnlock()

		if sm != nil {
			completed, err := call.Pipeline.SpeakText(ctx, msg.Text, voiceID)
			if err != nil {
				log.Printf("[SIP] Streaming TTS falhou para %s, tentando batch: %v", msg.ChatID, err)
				// Fallback: batch TTS
				if batchErr := s.sendBatchTTS(ctx, call, msg.Text, voiceID, sm); batchErr != nil {
					log.Printf("[SIP] Batch TTS tamb├®m falhou para %s: %v", msg.ChatID, batchErr)
					return batchErr
				}
			} else if !completed {
				log.Printf("[SIP] TTS streaming interrompido (barge-in) para %s", msg.ChatID)
			}
		}
	}

	return nil
}

// sendBatchTTS sintetiza ├íudio batch (fallback quando streaming n├úo funciona)
// e reproduz na chamada ativa.
func (s *SIPAdapter) sendBatchTTS(ctx context.Context, call *CallSession, text, voiceID string, sm *speech.SpeechManager) error {
	var result *speech.SynthesisResult
	var err error
	if voiceID != "" {
		result, err = sm.SynthesizeWithVoice(text, voiceID)
	} else {
		result, err = sm.Synthesize(text)
	}
	if err != nil {
		return fmt.Errorf("sip batch TTS: %w", err)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(result.AudioBase64)
	if err != nil {
		return fmt.Errorf("sip batch TTS decode: %w", err)
	}

	pcm8k, err := decodeMp3ToPCM8k(audioBytes)
	if err != nil {
		return fmt.Errorf("sip batch TTS resample: %w", err)
	}

	_, err = call.Pipeline.PlayAudio(pcm8k)
	return err
}

// convertToPCM8k converte dados de ├íudio para PCM 16-bit signed LE 8kHz mono.
// Suporta MP3 (formato padr├úo do TTS), PCM raw (24kHz do OpenAI), e WAV.
func convertToPCM8k(data []byte, mimeType string) ([]byte, error) {
	switch mimeType {
	case "audio/mpeg", "audio/mp3":
		// MP3 ÔåÆ decode para PCM (go-mp3 decodifica para stereo interleaved 16-bit signed LE, 44.1kHz)
		// e depois downsample para 8kHz mono
		return decodeMp3ToPCM8k(data)
	case "audio/pcm", "audio/l16":
		// PCM raw ÔÇö assumimos 24kHz (OpenAI TTS format "pcm" output)
		return Resample24to8(data), nil
	case "audio/wav", "audio/x-wav":
		// WAV ÔÇö extrai PCM e resampla
		pcm, sampleRate := extractPCMFromWAV(data)
		if pcm == nil {
			return nil, fmt.Errorf("WAV inv├ílido")
		}
		switch sampleRate {
		case 8000:
			return pcm, nil
		case 16000:
			return Resample16to8(pcm), nil
		case 24000:
			return Resample24to8(pcm), nil
		default:
			return nil, fmt.Errorf("sample rate %d n├úo suportado", sampleRate)
		}
	default:
		return nil, fmt.Errorf("tipo de ├íudio %q n├úo suportado para playback SIP", mimeType)
	}
}

// decodeMp3ToPCM8k decodifica MP3 para PCM 16-bit signed LE 8kHz mono.
// go-mp3 decodifica para PCM stereo interleaved 16-bit signed LE @ sample rate original.
func decodeMp3ToPCM8k(data []byte) ([]byte, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("erro ao decodificar MP3: %w", err)
	}

	// L├¬ todos os samples decodificados (stereo interleaved 16-bit)
	pcmData, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler PCM do MP3: %w", err)
	}

	sampleRate := decoder.SampleRate()

	// Converte stereo ÔåÆ mono (m├®dia dos 2 canais)
	monoData := stereoToMono(pcmData)

	// Downsample para 8kHz
	return resampleGenericTo8k(monoData, sampleRate), nil
}

// stereoToMono converte PCM 16-bit signed LE stereo interleaved para mono.
func stereoToMono(stereo []byte) []byte {
	numFrames := len(stereo) / 4 // 2 channels * 2 bytes per sample
	mono := make([]byte, numFrames*2)

	for i := 0; i < numFrames; i++ {
		srcIdx := i * 4
		left := int16(binary.LittleEndian.Uint16(stereo[srcIdx : srcIdx+2]))
		right := int16(binary.LittleEndian.Uint16(stereo[srcIdx+2 : srcIdx+4]))
		avg := int16((int32(left) + int32(right)) / 2)
		binary.LittleEndian.PutUint16(mono[i*2:], uint16(avg))
	}

	return mono
}

// resampleGenericTo8k faz downsample de PCM mono 16-bit de qualquer rate para 8kHz.
// Usa m├®dia dos samples vizinhos como filtro anti-aliasing simples.
func resampleGenericTo8k(pcm []byte, srcRate int) []byte {
	if srcRate <= 0 || srcRate == 8000 {
		return pcm
	}

	numSamples := len(pcm) / 2
	if numSamples == 0 {
		return nil
	}

	ratio := float64(srcRate) / 8000.0
	if ratio <= 0 {
		return nil
	}

	outSamples := int(float64(numSamples) / ratio)
	// Sanity check: evita alocação gigante com srcRate inválido
	if outSamples <= 0 || outSamples > 10_000_000 {
		return nil
	}
	out := make([]byte, outSamples*2)

	// Tamanho da janela de m├®dia (metade do ratio, m├¡n 1)
	window := int(ratio/2 + 0.5)
	if window < 1 {
		window = 1
	}

	for i := 0; i < outSamples; i++ {
		center := int(float64(i) * ratio)
		if center >= numSamples {
			center = numSamples - 1
		}

		// M├®dia dos samples na janela ao redor do center
		start := center - window
		if start < 0 {
			start = 0
		}
		end := center + window + 1
		if end > numSamples {
			end = numSamples
		}

		var sum int32
		for j := start; j < end; j++ {
			sum += int32(int16(binary.LittleEndian.Uint16(pcm[j*2 : j*2+2])))
		}
		avg := int16(sum / int32(end-start))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(avg))
	}

	return out
}

// extractPCMFromWAV extrai dados PCM brutos e sample rate de um arquivo WAV.
func extractPCMFromWAV(wav []byte) (pcm []byte, sampleRate uint32) {
	if len(wav) < 44 {
		return nil, 0
	}
	// Verifica RIFF header
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, 0
	}
	sampleRate = uint32(wav[24]) | uint32(wav[25])<<8 | uint32(wav[26])<<16 | uint32(wav[27])<<24
	return wav[44:], sampleRate
}

// registerLoop faz REGISTER no servidor SIP e mant├®m o registro renovado.
// Faz retry com backoff em caso de falhas (401 por nonce stale, timeout, etc).
// diago.Register ├® blocking ÔÇö renova automaticamente at├® contexto cancelar.
func (s *SIPAdapter) registerLoop() {
	s.mu.RLock()
	dg := s.dg
	ctx := s.ctx
	s.mu.RUnlock()

	// Verifica se o adapter foi desconectado antes da goroutine iniciar
	if dg == nil || ctx == nil {
		return
	}

	recipient := siplib.Uri{
		Scheme: "sip",
		User:   s.config.User,
		Host:   s.config.Server,
		Port:   s.config.GetNonDefaultPort(),
	}

	// Resolve o servidor SIP uma vez (SRV ÔåÆ A) e fixa o IP.
	// proxy.sipthor.net tem m├║ltiplos IPs (load-balancing): se o sipgo
	// re-resolver DNS entre o REGISTER inicial e o retry com auth digest,
	// pode cair em outro servidor que n├úo conhece o nonce ÔåÆ 401 infinito.
	proxyHost := s.resolveProxyHost(ctx)

	s.mu.Lock()
	s.proxyHost = proxyHost
	s.mu.Unlock()

	opts := diago.RegisterOptions{
		Username:      s.config.User,
		Password:      s.config.Password,
		Expiry:        5 * time.Minute,
		RetryInterval: 30 * time.Second,
		ProxyHost:     proxyHost,
		OnRegistered: func() {
			log.Printf("[SIP] Registrado com sucesso em %s@%s", s.config.User, s.config.Server)
			s.setStatus(messaging.StatusConnected)
		},
	}

	if proxyHost != "" {
		log.Printf("[SIP] Iniciando registro: %s@%s via %s", s.config.User, s.config.Server, proxyHost)
	} else {
		log.Printf("[SIP] Iniciando registro: %s@%s", s.config.User, s.config.Server)
	}

	// Retry com backoff: 401 pode ser nonce stale/timing, n├úo necessariamente senha errada.
	retryDelay := 2 * time.Second
	const maxRetry = 30 * time.Second

	for {
		err := dg.Register(ctx, recipient, opts)
		if err == nil || ctx.Err() != nil {
			return
		}

		var regErr *diago.RegisterResponseError
		if errors.As(err, &regErr) {
			log.Printf("[SIP] Registro falhou: %s (retry em %s)", regErr.Msg, retryDelay)
		} else {
			log.Printf("[SIP] Erro no registro: %v (retry em %s)", err, retryDelay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}

		// Backoff exponencial at├® maxRetry
		retryDelay = retryDelay * 2
		if retryDelay > maxRetry {
			retryDelay = maxRetry
		}
	}
}

// resolveProxyHost faz resolu├º├úo RFC 3263 (SRV ÔåÆ A) e retorna "ip:port" fixo.
// Isso garante que todas as mensagens da mesma transa├º├úo (incluindo retries de
// autentica├º├úo digest) v├úo para o mesmo servidor, evitando problemas com
// load-balancers SIP que distribuem nonces por servidor.
// Se a resolu├º├úo falhar, retorna "" e deixa o sipgo resolver normalmente.
func (s *SIPAdapter) resolveProxyHost(ctx context.Context) string {
	transport := s.config.GetTransport()
	host := s.config.Server

	resolveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// SRV lookup: _sip._udp.example.com (ou _sip._tcp, _sips._tcp)
	service := "sip"
	proto := "udp"
	if transport == "tcp" {
		proto = "tcp"
	} else if transport == "tls" {
		service = "sips"
		proto = "tcp"
	}

	_, srvAddrs, err := net.DefaultResolver.LookupSRV(resolveCtx, service, proto, host)
	if err == nil && len(srvAddrs) > 0 {
		target := srvAddrs[0].Target
		port := srvAddrs[0].Port

		ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, target)
		if err == nil && len(ips) > 0 {
			addr := fmt.Sprintf("%s:%d", ips[0].IP, port)
			log.Printf("[SIP] DNS resolvido: %s ÔåÆ SRV %s ÔåÆ %s", host, target, addr)
			return addr
		}
	}

	// Fallback: A record direto
	ips, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
	if err == nil && len(ips) > 0 {
		port := s.config.GetPort()
		addr := fmt.Sprintf("%s:%d", ips[0].IP, port)
		log.Printf("[SIP] DNS resolvido: %s ÔåÆ A %s", host, addr)
		return addr
	}

	log.Printf("[SIP] DNS falhou para %s: %v", host, err)
	return ""
}

// handleIncomingCall processa uma chamada SIP recebida.
// Atende ÔåÆ notifica gateway ÔåÆ inicia pipeline VADÔåÆSTTÔåÆLLMÔåÆTTS bidirecional.
func (s *SIPAdapter) handleIncomingCall(inDialog *diago.DialogServerSession) {
	// Extrai informa├º├Áes do chamador a partir do SIP Dialog
	callerID := inDialog.FromUser()
	callerName := ""

	// Tenta extrair display name e detectar scanners SIP via InviteRequest
	sipDialog := inDialog.DialogSIP()
	if sipDialog != nil && sipDialog.InviteRequest != nil {
		from := sipDialog.InviteRequest.From()
		if from != nil {
			callerName = from.DisplayName
			if from.Address.Host != "" && callerID != "" {
				callerID = callerID + "@" + from.Address.Host
			}
		}

		// Detecta scanners SIP pelo User-Agent antes de processar SDP.
		// Scanners como "SIPPER", "friendly-scanner", "sipvicious" enviam SDP
		// malformado que causa panic na lib diago (CodecsFromSDPRead bounds overflow).
		// Apenas User-Agent ├® verificado (From user pode dar falso positivo).
		uaVal := ""
		if ua := sipDialog.InviteRequest.GetHeader("User-Agent"); ua != nil {
			uaVal = ua.Value()
			uaLower := strings.ToLower(uaVal)
			for _, scanner := range sipScannerUserAgents {
				if strings.Contains(uaLower, scanner) {
					log.Printf("[SIP] Scanner SIP detectado (User-Agent=%q cont├®m %q) de %s ÔÇö rejeitando com 403", uaVal, scanner, callerID)
					inDialog.Respond(siplib.StatusForbidden, "Forbidden", nil)
					return
				}
			}
		} else {
			// Sem User-Agent ├® comportamento t├¡pico de scanner
			uaVal = "(sem User-Agent)"
		}
		log.Printf("[SIP] INVITE de %s User-Agent=%s", callerID, uaVal)
	}

	if callerID == "" {
		callerID = "unknown"
	}

	dialogID := fmt.Sprintf("sip-%s-%d", callerID, callCounter.Add(1))
	log.Printf("[SIP] Chamada recebida: %s (%s) ÔåÆ dialog=%s", callerID, callerName, dialogID)

	// Cria sess├úo de chamada
	call := NewCallSession(dialogID, callerID, callerName)
	call.Dialog = inDialog
	s.mu.Lock()
	s.calls[dialogID] = call
	s.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SIP] PANIC recuperado na chamada %s: %v", dialogID, r)
		}
		call.SetState(CallStateEnded)
		if call.Pipeline != nil {
			call.Pipeline.Stop()
		}
		s.mu.Lock()
		delete(s.calls, dialogID)
		s.mu.Unlock()
		log.Printf("[SIP] Chamada encerrada: %s (dura├º├úo: %s)", dialogID, call.Duration())
	}()

	// SIP: 100 Trying
	inDialog.Trying()

	// SIP: 200 OK (auto-atendimento)
	if err := inDialog.Answer(); err != nil {
		log.Printf("[SIP] Erro ao atender chamada %s: %v", dialogID, err)
		return
	}

	call.SetState(CallStateActive)
	log.Printf("[SIP] Chamada atendida: %s", dialogID)

	// Notifica o gateway (como qualquer outro canal)
	s.mu.RLock()
	handler := s.handler
	ctx := s.ctx
	speechMgr := s.speechManager
	s.mu.RUnlock()

	if handler != nil {
		msg := messaging.IncomingMessage{
			ID: dialogID,
			From: messaging.Contact{
				ID:          callerID,
				DisplayName: callerName,
				Username:    callerID,
			},
			Text:      "Ol├í",
			Channel:   "sip",
			Timestamp: call.StartedAt,
		}
		handler(ctx, msg)
	}

	// Se SpeechManager dispon├¡vel, inicia pipeline de ├íudio bidirecional
	if speechMgr != nil && handler != nil {
		pipeline := NewAudioPipeline(inDialog, call, handler, speechMgr, s.pipelineCfg)

		// Configura barge-in ÔåÆ cancelamento do LLM streaming
		s.mu.RLock()
		cancelFn := s.CancelStreamingForContact
		s.mu.RUnlock()
		if cancelFn != nil {
			cid := callerID // captura para closure
			pipeline.OnBargeIn = func() {
				cancelFn("sip", cid)
			}
		}

		call.Pipeline = pipeline

		log.Printf("[SIP] Pipeline de ├íudio iniciado para chamada %s", dialogID)
		if err := pipeline.Run(); err != nil {
			log.Printf("[SIP] Erro no pipeline de ├íudio %s: %v", dialogID, err)
		}
	} else {
		// Fallback sem pipeline: aguarda at├® BYE ou contexto cancelado
		log.Printf("[SIP] Pipeline indispon├¡vel (sem SpeechManager), aguardando BYE para %s", dialogID)
		<-inDialog.Context().Done()
	}
}

// findCallByCallerID busca a chamada ativa mais recente pelo CallerID.
// Deve ser chamado com s.mu travado para leitura.
// Se houver múltiplas chamadas ativas do mesmo número, retorna a mais recente.
func (s *SIPAdapter) findCallByCallerID(callerID string) *CallSession {
	var best *CallSession
	count := 0
	for _, call := range s.calls {
		if call.CallerID == callerID && call.GetState() == CallStateActive {
			count++
			if best == nil || call.StartedAt.After(best.StartedAt) {
				best = call
			}
		}
	}
	if count > 1 {
		log.Printf("[SIP] Aviso: %d chamadas ativas para %s, usando a mais recente (%s)",
			count, callerID, best.ID)
	}
	return best
}

// setStatus atualiza o status da conex├úo de forma thread-safe.
func (s *SIPAdapter) setStatus(status messaging.ConnectionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// truncate limita o comprimento de uma string.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Dial inicia uma chamada de sa├¡da para o destino SIP especificado.
// O n├║mero pode ser um ramal ("200"), usu├írio@host ("200@pbx.local"),
// ou SIP URI completo ("sip:200@pbx.local").
// Retorna o CallSession da chamada ativa ou erro.
// A chamada roda em background com pipeline de ├íudio bidirecional.
func (s *SIPAdapter) Dial(ctx context.Context, number string) (*CallSession, error) {
	s.mu.RLock()
	dg := s.dg
	sipCtx := s.ctx
	speechMgr := s.speechManager
	handler := s.handler
	status := s.status
	s.mu.RUnlock()

	if dg == nil || status != messaging.StatusConnected {
		return nil, fmt.Errorf("sip: adapter n├úo conectado (status: %s)", status)
	}

	// Constr├│i URI SIP do destino
	recipient := s.buildRecipientURI(number)

	dialogID := fmt.Sprintf("sip-out-%s-%d", number, callCounter.Add(1))
	log.Printf("[SIP] Iniciando chamada para %s ÔåÆ dialog=%s", number, dialogID)

	// Cria sess├úo de chamada
	call := NewCallSession(dialogID, number, "")
	call.SetState(CallStateRinging)

	s.mu.Lock()
	s.calls[dialogID] = call
	s.mu.Unlock()

	// dg.Invite ├® blocking: envia INVITE, espera resposta final (200 OK),
	// envia ACK e retorna com o di├ílogo estabelecido.
	// Se o destino n├úo atender ou o contexto for cancelado, retorna erro.
	opts := diago.InviteOptions{
		Username: s.config.User,
		Password: s.config.Password,
	}

	outDialog, err := dg.Invite(ctx, recipient, opts)
	if err != nil {
		call.SetState(CallStateEnded)
		s.mu.Lock()
		delete(s.calls, dialogID)
		s.mu.Unlock()
		log.Printf("[SIP] Chamada %s falhou: %v", dialogID, err)
		return nil, fmt.Errorf("sip: erro ao chamar %s: %w", number, err)
	}

	call.Dialog = outDialog
	call.SetState(CallStateActive)
	log.Printf("[SIP] Chamada de sa├¡da atendida: %s ÔåÆ %s", dialogID, number)

	// Pipeline de ├íudio em goroutine (a chamada j├í est├í estabelecida)
	go func() {
		defer func() {
			call.SetState(CallStateEnded)
			if call.Pipeline != nil {
				call.Pipeline.Stop()
			}
			s.mu.Lock()
			delete(s.calls, dialogID)
			s.mu.Unlock()
			log.Printf("[SIP] Chamada de sa├¡da encerrada: %s (dura├º├úo: %s)", dialogID, call.Duration())
		}()

		// Notifica gateway como novo canal ativo
		if handler != nil {
			msg := messaging.IncomingMessage{
				ID: dialogID,
				From: messaging.Contact{
					ID:          number,
					DisplayName: number,
					Username:    number,
				},
				Text:      "Ol├í",
				Channel:   "sip",
				Timestamp: call.StartedAt,
			}
			handler(sipCtx, msg)
		}

		// Inicia pipeline de ├íudio bidirecional
		if speechMgr != nil && handler != nil {
			pipeline := NewAudioPipeline(outDialog, call, handler, speechMgr, s.pipelineCfg)
			call.Pipeline = pipeline

			log.Printf("[SIP] Pipeline de ├íudio iniciado para chamada de sa├¡da %s", dialogID)
			if pErr := pipeline.Run(); pErr != nil {
				log.Printf("[SIP] Erro no pipeline de ├íudio %s: %v", dialogID, pErr)
			}
		} else {
			// Aguarda at├® a chamada terminar
			<-outDialog.Context().Done()
		}
	}()

	return call, nil
}

// HangupCall encerra uma chamada ativa pelo seu ID.
func (s *SIPAdapter) HangupCall(ctx context.Context, callID string) error {
	s.mu.RLock()
	call, ok := s.calls[callID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("sip: chamada %s n├úo encontrada", callID)
	}

	if call.Dialog != nil {
		return call.Dialog.Hangup(ctx)
	}

	return fmt.Errorf("sip: chamada %s sem dialog ativo", callID)
}

// ActiveCalls retorna informa├º├Áes sobre chamadas ativas.
func (s *SIPAdapter) ActiveCalls() []CallInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]CallInfo, 0, len(s.calls))
	for _, call := range s.calls {
		result = append(result, CallInfo{
			ID:        call.ID,
			CallerID:  call.CallerID,
			State:     string(call.GetState()),
			Duration:  call.Duration().String(),
			StartedAt: call.StartedAt,
		})
	}
	return result
}

// CallInfo cont├®m informa├º├Áes de uma chamada para exposi├º├úo ao frontend.
type CallInfo struct {
	ID        string    `json:"id"`
	CallerID  string    `json:"callerId"`
	State     string    `json:"state"`
	Duration  string    `json:"duration"`
	StartedAt time.Time `json:"startedAt"`
}

// buildRecipientURI constr├│i a URI SIP do destino com base no n├║mero/ramal.
// Aceita: "200" ÔåÆ sip:200@server, "200@pbx" ÔåÆ sip:200@pbx, "sip:200@pbx" ÔåÆ sip:200@pbx
func (s *SIPAdapter) buildRecipientURI(number string) siplib.Uri {
	// Se j├í cont├®m "@", ├® user@host
	user := number
	host := s.config.Server
	port := s.config.GetNonDefaultPort()

	for i, c := range number {
		if c == '@' {
			user = number[:i]
			host = number[i+1:]
			port = 0 // usa default para o host customizado
			break
		}
	}

	// Remove prefixo "sip:" se presente
	if len(user) > 4 && user[:4] == "sip:" {
		user = user[4:]
	}

	return siplib.Uri{
		Scheme: "sip",
		User:   user,
		Host:   host,
		Port:   port,
	}
}