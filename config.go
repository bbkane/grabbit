package main

import (
	_ "embed"
	"errors"

	"github.com/bbkane/glib"
	"go.bbkane.com/logos"
	"go.bbkane.com/warg/command"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

//go:embed embedded/grabbit.yaml
var embeddedConfig []byte

func editConfig(ctx command.Context) error {
	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   ctx.Flags["--log-filename"].(string),
		MaxAge:     ctx.Flags["--log-maxage"].(int),
		MaxBackups: ctx.Flags["--log-maxbackups"].(int),
		MaxSize:    ctx.Flags["--log-maxsize"].(int),
	}

	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberJackLogger, zap.DebugLevel, getVersion(),
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	configPath, configPathExists := ctx.Flags["--config"].(string)
	if !configPathExists {
		err := errors.New("must path --config")
		logos.Errorw(
			"Must pass --config",
			"err", err,
		)
		return err
	}
	editor := ctx.Flags["--editor"].(string)

	err := glib.EditFile(embeddedConfig, configPath, editor)
	if err != nil {
		logos.Errorw(
			"Unable to edit config",
			"configPath", configPath,
			"editorPath", editor,
			"err", err,
		)
		return err
	}
	return nil
}
