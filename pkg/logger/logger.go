package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func InitLogger(env string) (*zap.Logger, error) {
	_ = os.MkdirAll("./logs", os.ModePerm)

	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "./logs/app.log",
		MaxSize:    20, // Megabytes
		MaxBackups: 5,
		MaxAge:     30, // Days
		Compress:   true,
	})

	consoleWriter := zapcore.AddSync(os.Stdout)

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleWriter, zap.DebugLevel),
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, zap.InfoLevel),
	)

	Log = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(Log)
	return Log, nil
}
