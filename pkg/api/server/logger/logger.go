package logger

import (
	"log"

	"go.uber.org/zap"
)

// Get returns instance sugared zap logger with production configuration
func Get(level string) *zap.SugaredLogger {
	config := zap.NewProductionConfig()
	if len(level) > 0 {
		var err error
		if config.Level, err = zap.ParseAtomicLevel(level); err != nil {
			log.Fatalf("Failed to parse log level from config: %v", err)
		}
	}

	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize zap logger: %v", err)
	}

	return logger.Sugar()
}
