package logger

import "go.uber.org/zap"

func New(service string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.InitialFields = map[string]any{
		"service": service,
	}
	return cfg.Build()
}

func Error(err error) zap.Field {
	return zap.Error(err)
}

func String(key, val string) zap.Field {
	return zap.String(key, val)
}
