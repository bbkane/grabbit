package main

import (
	_ "embed"
	"errors"

	"github.com/bbkane/glib"
	"go.bbkane.com/logos"
	"go.bbkane.com/warg/flag"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

//go:embed embedded/grabbit.yaml
var embeddedConfig []byte

func editConfig(passedFlags flag.PassedFlags) error {
	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   passedFlags["--log-filename"].(string),
		MaxAge:     passedFlags["--log-maxage"].(int),
		MaxBackups: passedFlags["--log-maxbackups"].(int),
		MaxSize:    passedFlags["--log-maxsize"].(int),
	}

	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberJackLogger, zap.DebugLevel, getVersion(),
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	configPath, configPathExists := passedFlags["--config-path"].(string)
	if !configPathExists {
		err := errors.New("must path --config-path")
		logos.Errorw(
			"Must pass --config-path",
			"err", err,
		)
		return err
	}
	editor := passedFlags["--editor"].(string)

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
