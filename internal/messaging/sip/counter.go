package sip

import "sync/atomic"

// callCounter ├® um contador global para gerar IDs ├║nicos de chamadas.
var callCounter atomic.Int64