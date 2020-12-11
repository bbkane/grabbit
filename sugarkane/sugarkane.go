package sugarkane

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// SugarKane is a very opinionated wrapper around a uber/zap sugared logger
// It's designed primarily to simultaneously print "pretty-enough" input for a
// user and useful enough info to a lumberjack logger
// It should really only be used with simple key/value pairs
// It's designed to be fairly easily swappable with the sugared logger
type SugarKane struct {
	errorStream *os.File
	infoStream  *os.File
	logger      *zap.Logger
	sugar       *zap.SugaredLogger
}

// Infow prints a message and keys and values with INFO level
func (s *SugarKane) Infow(msg string, keysAndValues ...interface{}) {
	s.sugar.Infow(msg, keysAndValues...)
	msg = "INFO: " + msg
	Printw(os.Stdout, msg, keysAndValues...)
}

// Errorw prints a message and keys and values with INFO level
func (s *SugarKane) Errorw(msg string, keysAndValues ...interface{}) {
	s.sugar.Errorw(msg, keysAndValues...)
	msg = "ERROR: " + msg
	Printw(os.Stderr, msg, keysAndValues...)
}

// Debugw prints keys and values only to the log, not to the user
func (s *SugarKane) Debugw(msg string, keysAndValues ...interface{}) {
	s.sugar.Debugw(msg, keysAndValues...)
}

// Sync syncs the underlying logger
func (s *SugarKane) Sync() error {
	return s.logger.Sync()
}

// Printw formats and prints a msg and keys and values to a stream.
// Useful when you need to show info but you don't have a log
func Printw(fp *os.File, msg string, keysAndValues ...interface{}) {
	length := len(keysAndValues)
	if length%2 != 0 {
		panic(fmt.Sprintf("len() not even - keysAndValues: %#v\n", keysAndValues))
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

// NewSugarKane creates a new SugarKane all ready to go
func NewSugarKane(lumberjackLogger *lumberjack.Logger, errorStream *os.File, infoStream *os.File, lvl zapcore.LevelEnabler, appVersion string) *SugarKane {
	logger := newLogger(lumberjackLogger, lvl, appVersion)
	return &SugarKane{
		errorStream: errorStream,
		infoStream:  infoStream,
		logger:      logger,
		sugar:       logger.WithOptions(zap.AddCallerSkip(1)).Sugar(),
	}

}

// newLogger builds a logger configured how I like it. If
// lumberjackLogger is null, it returns a no-op logger (useful if I
// don't want logs)
func newLogger(lumberjackLogger *lumberjack.Logger, lvl zapcore.LevelEnabler, appVersion string) *zap.Logger {
	if lumberjackLogger == nil {
		return zap.NewNop()
	}
	encoderConfig := zapcore.EncoderConfig{
		// prefix shared keys with '_' so they show up first when keys are alphabetical
		TimeKey:        "_timestamp",
		LevelKey:       "_level",
		NameKey:        "_name", // TODO: what is this?
		CallerKey:      "_caller",
		FunctionKey:    "_function", // zapcore.OmitKey,
		MessageKey:     "_msg",
		StacktraceKey:  "_stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	customCommonFields := zap.Fields(
		zap.Int("_pid", os.Getpid()),
		zap.String("_version", appVersion),
	)

	jsonCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.AddSync(lumberjackLogger),
		lvl,
	)

	logger := zap.New(
		jsonCore,
		zap.AddCaller(),
		// Using errors package to get better stack traces
		// zap.AddStacktrace(stackTraceLvl),
		customCommonFields,
	)
	return logger
}

// LogOnPanic tries to log a panic. It should be called at the start of each
// goroutine. See panic and recover docs
func (s *SugarKane) LogOnPanic() {
	stackTraceSugar := s.logger.
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
