package database

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gorm.io/gorm/logger"
)

func TestConfiguredGORMLoggerDuplicaNoWriterAdicional(t *testing.T) {
	previousLevel := gormLogLevel
	previousOutput := gormLogOutput
	t.Cleanup(func() {
		SetLogLevel(previousLevel)
		SetLogOutput(previousOutput)
	})

	var output bytes.Buffer
	SetLogLevel(logger.Warn)
	SetLogOutput(&output)

	configuredGORMLogger().Warn(context.Background(), "aviso GORM capturado")

	if !strings.Contains(output.String(), "aviso GORM capturado") {
		t.Fatalf("writer adicional não recebeu o log do GORM: %q", output.String())
	}
}
