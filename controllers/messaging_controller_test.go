package controllers

import (
	"testing"
	"time"

	"assistente/internal/channels"
)

func TestBuildSIPPipelineConfigAppliesChannelOverrides(t *testing.T) {
	cfg := &channels.ChannelConfig{
		SIPAudioTuningConfigured: true,
		SIPDenoise:               false,
		SIPAGC:                   false,
		SIPNoiseSuppressDB:       -18,
		SIPAGCTarget:             22000,
		SIPAGCMaxGainDB:          24,
		SIPVADMode:               3,
		SIPVADSpeechMS:           120,
		SIPVADSilenceMS:          650,
		SIPBargeInThreshold:      0.11,
	}

	pipelineCfg := buildSIPPipelineConfig(cfg)

	if pipelineCfg.Preprocess.EnableDenoise {
		t.Fatalf("expected denoise disabled")
	}
	if pipelineCfg.Preprocess.EnableAGC {
		t.Fatalf("expected AGC disabled")
	}
	if got := pipelineCfg.Preprocess.NoiseSuppressDB; got != -18 {
		t.Fatalf("unexpected NoiseSuppressDB: got=%d want=%d", got, -18)
	}
	if got := pipelineCfg.Preprocess.AGCTarget; got != 22000 {
		t.Fatalf("unexpected AGCTarget: got=%d want=%d", got, 22000)
	}
	if got := pipelineCfg.Preprocess.AGCMaxGainDB; got != 24 {
		t.Fatalf("unexpected AGCMaxGainDB: got=%d want=%d", got, 24)
	}
	if got := pipelineCfg.VAD.Mode; got != 3 {
		t.Fatalf("unexpected VAD mode: got=%d want=%d", got, 3)
	}
	if got := pipelineCfg.VAD.SpeechDuration; got != 120*time.Millisecond {
		t.Fatalf("unexpected speech duration: got=%s want=%s", got, 120*time.Millisecond)
	}
	if got := pipelineCfg.VAD.SilenceDuration; got != 650*time.Millisecond {
		t.Fatalf("unexpected silence duration: got=%s want=%s", got, 650*time.Millisecond)
	}
	if got := pipelineCfg.BargeInRMSThreshold; got != 0.11 {
		t.Fatalf("unexpected barge-in threshold: got=%f want=%f", got, 0.11)
	}
}

func TestBuildSIPPipelineConfigKeepsDefaultsWhenNotConfigured(t *testing.T) {
	defaults := buildSIPPipelineConfig(nil)
	cfg := buildSIPPipelineConfig(&channels.ChannelConfig{
		SIPDenoise: false,
		SIPAGC:     false,
	})

	if cfg.Preprocess.EnableDenoise != defaults.Preprocess.EnableDenoise {
		t.Fatalf("expected default denoise when tuning is not configured")
	}
	if cfg.Preprocess.EnableAGC != defaults.Preprocess.EnableAGC {
		t.Fatalf("expected default AGC when tuning is not configured")
	}
}

func TestMergePreservedChannelStateKeepsSIPTuningAndConversations(t *testing.T) {
	existing := &channels.ChannelConfig{
		Conversations:            map[string]uint{"abc": 42},
		SIPAudioTuningConfigured: true,
		SIPDenoise:               false,
		SIPAGC:                   false,
		SIPNoiseSuppressDB:       -18,
		SIPAGCTarget:             22000,
		SIPAGCMaxGainDB:          24,
		SIPVADMode:               2,
		SIPVADSpeechMS:           100,
		SIPVADSilenceMS:          500,
		SIPBargeInThreshold:      0.12,
	}
	incoming := &channels.ChannelConfig{
		SIPServer: "pbx.local",
	}

	mergePreservedChannelState("sip", existing, incoming)

	if got := incoming.Conversations["abc"]; got != 42 {
		t.Fatalf("expected conversations to be preserved, got=%d", got)
	}
	if !incoming.SIPAudioTuningConfigured {
		t.Fatalf("expected SIP tuning flag to be preserved")
	}
	if incoming.SIPDenoise {
		t.Fatalf("expected denoise to be preserved as false")
	}
	if incoming.SIPAGC {
		t.Fatalf("expected AGC to be preserved as false")
	}
	if incoming.SIPNoiseSuppressDB != -18 || incoming.SIPAGCTarget != 22000 || incoming.SIPAGCMaxGainDB != 24 {
		t.Fatalf("expected preprocess tuning to be preserved")
	}
	if incoming.SIPVADMode != 2 || incoming.SIPVADSpeechMS != 100 || incoming.SIPVADSilenceMS != 500 {
		t.Fatalf("expected VAD tuning to be preserved")
	}
	if incoming.SIPBargeInThreshold != 0.12 {
		t.Fatalf("expected barge-in threshold to be preserved")
	}
}
