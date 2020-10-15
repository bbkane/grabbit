package main

import (
	"fmt"
	"os"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// logAndPrint produces a more human readable error message for the console.
// It's really only designed for simple keys/value messages It prints Info if
// fp == os.Stdout, Error if fp == os.Stderr, and panics otherwise.
func logAndPrint(sugar *zap.SugaredLogger, fp *os.File, msg string, keysAndValues ...interface{}) {

	// adjust stack frame to see caller of logAndPrint in the log
	// NOTE: could cache this somehow - make it an arg?
	callerSugar := sugar.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar()

	switch fp {
	case os.Stdout:
		callerSugar.Infow(msg, keysAndValues...)
		msg = "INFO: " + msg
	case os.Stderr:
		callerSugar.Errorw(msg, keysAndValues...)
		msg = "ERROR: " + msg
	default:
		sugar.Panicw(
			"fp not os.Stderr or os.Stdout",
			"fp", fp,
		)
	}

	length := len(keysAndValues)
	if length%2 != 0 {
		sugar.Panicw(
			"len() not even",
			"keysAndValues", keysAndValues,
		)
	}

	keys := make([]string, length/2)
	values := make([]interface{}, length/2)
	for i := 0; i < length/2; i++ {
		keys[i] = keysAndValues[i*2].(string)
		values[i] = keysAndValues[i*2+1]
	}

	fmtStr := msg + "\n"
	for i := 0; i < length/2; i++ {
		fmtStr += ("  " + keys[i] + ": %#v\n")
	}

	fmtStr += "\n"
	fmt.Fprintf(fp, fmtStr, values...)
}

// newLogger builds a logger. If lumberjackLogger or fp are nil, then that respective sink won't be made
func newLogger(lumberjackLogger *lumberjack.Logger, fp *os.File, lvl zapcore.LevelEnabler, appVersion string) *zap.Logger {
	encoderConfig := zapcore.EncoderConfig{
		// Keys can be anything except the empty string.
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "name", // TODO: what is this?
		CallerKey:      "caller",
		FunctionKey:    "function", // zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	coreSlice := make([]zapcore.Core, 0, 2)

	if lumberjackLogger != nil {
		jsonCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(lumberjackLogger),
			lvl,
		)
		coreSlice = append(coreSlice, jsonCore)
	}

	if fp != nil {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		fpCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zapcore.Lock(fp),
			lvl,
		)
		coreSlice = append(coreSlice, fpCore)
	}

	combinedCore := zapcore.NewTee(
		coreSlice...,
	)

	logger := zap.New(
		combinedCore,
		zap.AddCaller(),
		// Using errors package to get better stack traces
		// zap.AddStacktrace(stackTraceLvl),
		// TODO: replace with version (goreleaser embeds it)
		zap.Fields(
			zap.Int("pid", os.Getpid()),
			zap.String("version", appVersion),
		),
	)

	return logger
}

func logOnPanic(sugar *zap.SugaredLogger) {
	stackTraceSugar := sugar.
		Desugar().
		WithOptions(
			zap.AddStacktrace(zap.PanicLevel),
		).
		Sugar()
	if err := recover(); err != nil {
		stackTraceSugar.Panicw(
			"panic found!",
			"err", err,
		)
	}
}
