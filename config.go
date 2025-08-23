package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"

	"github.com/bbkane/glib"
	"go.bbkane.com/logos"
	"go.bbkane.com/warg"
	"go.bbkane.com/warg/path"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

//go:embed embedded/grabbit.yaml
var embeddedConfig []byte

func editConfig(ctx warg.CmdContext) error {
	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   ctx.Flags["--log-filename"].(path.Path).MustExpand(),
		MaxAge:     ctx.Flags["--log-maxage"].(int),
		MaxBackups: ctx.Flags["--log-maxbackups"].(int),
		MaxSize:    ctx.Flags["--log-maxsize"].(int),
		LocalTime:  true,
		Compress:   false,
	}

	color, err := warg.ConditionallyEnableColor(ctx.Flags, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error enabling color, continuing without: %s", err.Error())
	}

	zapLogger := logos.NewBBKaneZapLogger(lumberJackLogger, zap.DebugLevel, version)
	logger := logos.New(zapLogger, color)
	logger.LogOnPanic()

	configPath, configPathExists := ctx.Flags["--config"]
	if !configPathExists {
		err := errors.New("must path --config")
		logger.Errorw(
			"Must pass --config",
			"err", err,
		)
		return err
	}
	editor := ctx.Flags["--editor"].(string)

	err = glib.EditFile(embeddedConfig, configPath.(path.Path).MustExpand(), editor)
	if err != nil {
		logger.Errorw(
			"Unable to edit config",
			"configPath", configPath,
			"editorPath", editor,
			"err", err,
		)
		return err
	}

	err = logger.Sync()
	if err != nil {
		return fmt.Errorf("could not sync logger: %w", err)
	}
	return nil
}
