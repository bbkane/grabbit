package main

import (
	"time"

	"go.bbkane.com/warg"
	"go.bbkane.com/warg/config/yamlreader"
	"go.bbkane.com/warg/path"
	"go.bbkane.com/warg/value/scalar"
	"go.bbkane.com/warg/value/slice"
)

var version string

func app() *warg.App {
	appFooter := `Examples (assuming BASH-like shell):

  # Grab from passed flags
  grabbit grab --subreddit-info "wallpapers,week,5" --destination ./images

  # Create/Edit config file
  grabbit config edit --editor /path/to/editor

  # Grab from config file
  grabbit grab

Homepage: https://github.com/bbkane/grabbit
`

	logFlags := warg.FlagMap{
		"--log-filename": warg.NewFlag(
			"Log filename",
			scalar.Path(
				scalar.Default(path.New("~/.config/grabbit.jsonl")),
			),
			warg.ConfigPath("lumberjacklogger.filename"),
			warg.Required(),
		),
		"--log-maxage": warg.NewFlag(
			"Max age before log rotation in days", // TODO: change to duration flag
			scalar.Int(
				scalar.Default(30),
			),
			warg.ConfigPath("lumberjacklogger.maxage"),
			warg.Required(),
		),
		"--log-maxbackups": warg.NewFlag(
			"Num backups for the log",
			scalar.Int(
				scalar.Default(0),
			),
			warg.ConfigPath("lumberjacklogger.maxbackups"),
			warg.Required(),
		),
		"--log-maxsize": warg.NewFlag(
			"Max size of log in megabytes",
			scalar.Int(
				scalar.Default(5),
			),
			warg.ConfigPath("lumberjacklogger.maxsize"),
			warg.Required(),
		),
	}

	app := warg.New(
		"grabbit",
		version,
		warg.NewSection(
			"Get top images from subreddits",
			warg.NewSubCmd(
				"grab",
				"Grab images. Optionally use `config edit` first to create a config",
				grab,
				warg.CmdFlagMap(logFlags),
				warg.NewCmdFlag(
					"--destination",
					"Destination directory for downloads",
					scalar.Path(scalar.Default(path.New("."))),
					warg.Alias("-d"),
					warg.ConfigPath("destination"),
					warg.FlagCompletions(warg.CompletionsDirectoriesFiles()),
					warg.Required(),
				),
				warg.NewCmdFlag(
					"--subreddit-info",
					"<subreddit>,<day|week|month|year|all>,<count>",
					slice.New(
						SubredditInfoTypeInfo(),
						slice.Default([]SubredditInfo{
							{
								Subreddit: "earthporn",
								Timeframe: "week",
								Count:     2,
							},
						}),
					),
					warg.ConfigPath("subreddits"),
					warg.Required(),
				),
				warg.NewCmdFlag(
					"--timeout",
					"Timeout for a single download",
					scalar.Duration(
						scalar.Default(time.Second*30),
					),
					warg.Alias("-t"),
					warg.Required(),
				),
			),
			warg.SectionFooter(appFooter),
			warg.NewSubSection(
				"config",
				"Config commands",
				warg.NewSubCmd(
					"edit",
					"Edit or create configuration file.",
					editConfig,
					warg.CmdFlagMap(logFlags),
					warg.NewCmdFlag(
						"--editor",
						"Path to editor",
						scalar.String(
							scalar.Default("vi"),
						),
						warg.Alias("-e"),
						warg.FlagCompletions(warg.CompletionsDirectoriesFiles()),
						warg.EnvVars("EDITOR"),
						warg.Required(),
					),
				),
			),
		),
		warg.ConfigFlag(
			yamlreader.New,
			warg.FlagMap{
				"--config": warg.NewFlag(
					"Path to YAML config file",
					scalar.Path(
						scalar.Default(path.New("~/.config/grabbit.yaml")),
					),
					warg.Alias("-c"),
					warg.FlagCompletions(warg.CompletionsDirectoriesFiles()),
				),
			},
		),
		warg.SkipValidation(),
	)
	return &app
}

func main() {
	app := app()
	app.MustRun()
}
