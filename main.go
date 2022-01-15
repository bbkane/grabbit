package main

import (
	"os"

	"github.com/bbkane/warg"
	"github.com/bbkane/warg/command"
	"github.com/bbkane/warg/config/yamlreader"
	"github.com/bbkane/warg/flag"
	"github.com/bbkane/warg/section"
	"github.com/bbkane/warg/value"
)

func main() {
	appFooter := `Examples (assuming BASH-like shell):

  # Grab from passed flags
  grabbit grab \
      --subreddit-destination . \
      --subreddit-limit 5 \
      --subreddit-name wallpapers \
      --subreddit-timeframe day

  # Create/Edit config file
  grabbit config edit --editor /path/to/editor

  # Grab from config file
  grabbit grab
`
	grabCmd := command.New(
		command.HelpShort("Grab images. Optionally use `config edit` first to create a config"),
		grab,
		command.Flag(
			flag.Name("--subreddit-name"),
			flag.HelpShort("subreddit to grab"),
			value.StringSlice,
			flag.Alias("-sn"),
			flag.Default("earthporn", "wallpapers"),
			flag.ConfigPath("subreddits[].name"),
			flag.Required(),
		),
		command.Flag(
			flag.Name("--subreddit-destination"),
			flag.HelpShort("Where to store the subreddit"),
			value.PathSlice,
			flag.Alias("-sd"),
			flag.Default(".", "."),
			flag.ConfigPath("subreddits[].destination"),
			flag.Required(),
		),
		command.Flag(
			flag.Name("--subreddit-timeframe"),
			flag.HelpShort("Take the top subreddits from this timeframe"),
			// TODO: this should be a StringEnumSlice once that's implemented
			value.StringSlice,
			flag.Alias("-st"),
			flag.Default("week", "week"),
			flag.ConfigPath("subreddits[].timeframe"),
			flag.Required(),
		),
		command.Flag(
			flag.Name("--subreddit-limit"),
			flag.HelpShort("max number of links to try to download"),
			value.IntSlice,
			flag.Alias("-sl"),
			flag.Default("2", "3"),
			flag.ConfigPath("subreddits[].limit"),
			flag.Required(),
		),
	)

	app := warg.New(
		"grabbit",
		section.New(
			section.HelpShort("Get top images from subreddits"),
			section.ExistingCommand(
				command.Name("grab"),
				grabCmd,
			),
			section.Footer(appFooter),
			section.Command(
				command.Name("version"),
				command.HelpShort("Print version"),
				printVersion,
			),
			section.Flag(
				flag.Name("--color"),
				flag.HelpShort("Use colorized output"),
				value.StringEnum("true", "false", "auto"),
				flag.Default("auto"),
			),
			section.Flag(
				flag.Name("--log-filename"),
				flag.HelpShort("log filename"),
				value.Path,
				flag.Default("~/.config/grabbit.jsonl"),
				flag.ConfigPath("lumberjacklogger.filename"),
				flag.Required(),
			),
			section.Flag(
				flag.Name("--log-maxage"),
				flag.HelpShort("max age before log rotation in days"),
				value.Int,
				flag.Default("30"),
				flag.ConfigPath("lumberjacklogger.maxage"),
				flag.Required(),
			),
			section.Flag(
				flag.Name("--log-maxbackups"),
				flag.HelpShort("num backups for the log"),
				value.Int,
				flag.Default("0"),
				flag.ConfigPath("lumberjacklogger.maxbackups"),
				flag.Required(),
			),
			section.Flag(
				flag.Name("--log-maxsize"),
				flag.HelpShort("max size of log in megabytes"),
				value.Int,
				flag.Default("5"),
				flag.ConfigPath("lumberjacklogger.maxsize"),
				flag.Required(),
			),
			section.Section(
				section.Name("config"),
				section.HelpShort("Config commands"),
				section.Command(
					command.Name("edit"),
					command.HelpShort("Edit or create configuration file."),
					editConfig,
					command.Flag(
						flag.Name("--editor"),
						flag.HelpShort("path to editor"),
						value.String,
						flag.Alias("-e"),
						flag.Default("vi"),
						flag.EnvVars("EDITOR"),
						flag.Required(),
					),
				),
			),
		),
		warg.ConfigFlag(
			flag.Name("--config"),
			yamlreader.New,
			"config filepath",
			flag.Alias("-c"),
			flag.Default("~/.config/grabbit.yaml"),
		),
	)
	app.MustRun(os.Args, os.LookupEnv)
}
