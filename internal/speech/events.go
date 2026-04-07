package speech

// ============================================================================
// Constantes de eventos Wails para o subsistema Speech/TTS.
// Devem ser usadas tanto no backend (EventsEmit) quanto correspondem
// às constantes do frontend em frontend/src/lib/speechEvents.ts.
// ============================================================================

const (
	// EventTTSStreamStart é emitido quando um stream TTS inicia.
	EventTTSStreamStart = "tts:stream:start"

	// EventTTSStreamChunk é emitido para cada chunk de áudio.
	EventTTSStreamChunk = "tts:stream:chunk"

	// EventTTSStreamDone é emitido quando o stream TTS finaliza com sucesso.
	EventTTSStreamDone = "tts:stream:done"

	// EventTTSStreamError é emitido quando ocorre um erro no stream TTS.
	EventTTSStreamError = "tts:stream:error"
)
