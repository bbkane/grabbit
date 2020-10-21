package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/natefinch/lumberjack"
	"github.com/pkg/errors"
	"github.com/vartanbeno/go-reddit/reddit"
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

// These will be overwritten by goreleaser
var version = "devVersion"
var commit = "devCommit"
var date = "devDate"
var builtBy = "devBuiltBy"

type subreddit struct {
	Name        string
	Destination string
	Timeframe   string
	Limit       int
}

type config struct {
	Version          string
	LumberjackLogger *lumberjack.Logger `yaml:"lumberjacklogger"`
	Subreddits       []subreddit
}

// downloadFile does not overwrite existing files
func downloadFile(URL string, fileName string) error {

	// O_EXCL - used with O_CREATE, file must not exist
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()

	response, err := http.Get(URL)
	if err != nil {
		return errors.WithStack(err)
	}
	defer response.Body.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func genFilePath(destinationDir string, subreddit string, title string, fullURL string) (string, error) {
	fileURL, err := url.Parse(fullURL)
	if err != nil {
		return "", errors.WithStack(err)
	}

	path := fileURL.Path
	segments := strings.Split(path, "/")

	urlFileName := segments[len(segments)-1]

	for _, s := range []string{" ", "/", "\\", "\n", "\r", "\x00"} {
		title = strings.ReplaceAll(title, s, "_")
		subreddit = strings.ReplaceAll(subreddit, s, "_")
	}
	fullFileName := subreddit + "_" + title + "_" + urlFileName
	filePath := filepath.Join(destinationDir, fullFileName)

	// remove chars from title if it's too long for the OS to handle
	const MAX_PATH = 250
	if len(filePath) > MAX_PATH {
		toChop := len(filePath) - MAX_PATH
		if toChop > len(title) {
			return "", errors.Errorf("filePath to long and title too short: %#v\n", filePath)
		}

		title = title[:len(title)-toChop]
		fullFileName = subreddit + "_" + title + "_" + urlFileName
		filePath = filepath.Join(destinationDir, fullFileName)
	}
	return filePath, nil
}

func parseConfig(configBytes []byte) (*lumberjack.Logger, []subreddit, error) {

	cfg := config{}
	err := yaml.UnmarshalStrict(configBytes, &cfg)
	if err != nil {
		// not ok to get invalid YAML
		return nil, []subreddit{}, errors.WithStack(err)
	}

	var lumberjackLogger *lumberjack.Logger = nil

	// we can get a valid config with a nil logger
	if cfg.LumberjackLogger != nil {
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			return nil, []subreddit{}, errors.WithStack(err)
		}
		cfg.LumberjackLogger.Filename = f
		lumberjackLogger = cfg.LumberjackLogger
	}

	subreddits := make([]subreddit, 0)
	for _, sr := range cfg.Subreddits {
		fullDest, err := homedir.Expand(sr.Destination)
		if err != nil {
			return nil, []subreddit{}, errors.WithStack(err)
		}
		sr.Destination = fullDest
		info, err := os.Stat(sr.Destination)
		if err != nil {
			return nil, []subreddit{}, errors.Wrapf(err, "Directory error: %v\n", sr.Destination)

		}
		if !info.IsDir() {
			return nil, []subreddit{}, errors.Errorf("not a directory: %#v\n", sr.Destination)
		}

		subreddits = append(subreddits, sr)
	}

	return lumberjackLogger, subreddits, nil
}

func isImage(URL string) error {
	if strings.HasSuffix(URL, ".jpg") {
		return nil
	}
	return errors.New("string doesn't end in .jpg")
}

func grab(sugar *zap.SugaredLogger, subreddits []subreddit) error {
	ua := runtime.GOOS + ":" + "grabbit" + ":" + version + " (github.com/bbkane/grabbit)"
	client, err := reddit.NewReadonlyClient(reddit.WithUserAgent(ua))
	if err != nil {
		err = errors.WithStack(err)
		logAndPrint(
			sugar, os.Stderr,
			"reddit initializion error",
			"err", err,
		)
		return err
	}

	ctx := context.Background()

	for _, subreddit := range subreddits {

		posts, _, err := client.Subreddit.TopPosts(
			ctx,
			subreddit.Name, &reddit.ListPostOptions{
				ListOptions: reddit.ListOptions{
					Limit: subreddit.Limit,
				},
				Time: subreddit.Timeframe,
			})

		if err != nil {
			// not fatal, we can continue with other subreddits
			logAndPrint(sugar, os.Stderr,
				"Can't use subreddit",
				"subreddit", subreddit,
				"err", errors.WithStack(err),
			)
			continue
		}

		for _, post := range posts {
			err = isImage(post.URL)
			if err != nil {
				logAndPrint(sugar, os.Stderr,
					"can't download image",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", err,
				)
				continue
			}

			filePath, err := genFilePath(subreddit.Destination, subreddit.Name, post.Title, post.URL)
			if err != nil {
				logAndPrint(sugar, os.Stderr,
					"genFilePath err",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", errors.WithStack(err),
				)
				continue
			}
			err = downloadFile(post.URL, filePath)
			if err != nil {
				if os.IsExist(errors.Cause(err)) {
					logAndPrint(sugar, os.Stdout,
						"file exists!",
						"subreddit", subreddit.Name,
						"filePath", filePath,
						"url", post.URL,
					)
				} else {
					logAndPrint(sugar, os.Stderr,
						"downloadFile error",
						"subreddit", subreddit.Name,
						"url", post.URL,
						"err", errors.WithStack(err),
					)
				}
				continue

			}
			logAndPrint(sugar, os.Stdout,
				"downloaded file",
				"subreddit", subreddit.Name,
				"filePath", filePath,
				"url", post.URL,
			)
		}
	}
	return nil
}

func editConfig(sugar *zap.SugaredLogger, configPath string, editor string) error {
	// TODO: make this a serialized config struct
	// so I get a compile warning if there's problems
	emptyConfigContent := []byte(`version: 2.0.0
# make lumberjacklogger nil to not log to file
lumberjacklogger:
  filename: ~/.config/grabbit.log
  maxsize: 5  # megabytes
  maxbackups: 0
  maxage: 30  # days
subreddits:
  - name: earthporn
    destination: ~/Pictures/grabbit
    timeframe: "day"
    limit: 5
  - name: cityporn
    destination: ~/Pictures/grabbit
    timeframe: "day"
    limit: 6
`)

	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		err = ioutil.WriteFile(configPath, emptyConfigContent, 0644)
		if err != nil {
			logAndPrint(
				sugar, os.Stderr,
				"can't write config",
				"err", err,
			)
			return err
		}
		logAndPrint(
			sugar, os.Stdout,
			"wrote default config",
			"configPath", configPath,
		)
	} else if err != nil {
		logAndPrint(
			sugar, os.Stderr,
			"can't write config",
			"err", err,
		)
		return err
	}

	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else if runtime.GOOS == "darwin" {
			editor = "open"
		} else if runtime.GOOS == "linux" {
			editor = "xdg-open"
		} else {
			editor = "vim"
		}
	}
	executable, err := exec.LookPath(editor)
	if err != nil {
		logAndPrint(
			sugar, os.Stderr,
			"can't find editor",
			"err", err,
		)
		return err
	}

	logAndPrint(
		sugar, os.Stdout,
		"Opening config",
		"editor", executable,
		"configPath", configPath,
	)

	cmd := exec.Command(executable, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		logAndPrint(
			sugar, os.Stderr,
			"editor cmd error",
			"err", err,
		)
		return err
	}

	return nil
}

func run() error {

	// cli and go!
	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/grabbit.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('c').Default(defaultConfigPath).String()

	editConfigCmd := app.Command("edit-config", "Edit or create configuration file. Uses $EDITOR as a fallback")
	editConfigCmdEditorFlag := editConfigCmd.Flag("editor", "path to editor").Short('e').String()

	grabCmd := app.Command("grab", "Grab images. Use `edit-config` first to create a config")

	versionCmd := app.Command("version", "print grabbit build and version information")

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	configPath, err := homedir.Expand(*appConfigPathFlag)
	if err != nil {
		// to early to log
		fmt.Fprintf(os.Stderr, "config error: %#v\n", err)
		panic(err)
	}

	// This is expected to fail on first run
	configBytes, cfgLoadErr := ioutil.ReadFile(configPath)

	lumberjackLogger, subreddits, cfgParseErr := parseConfig(configBytes)

	logger := newLogger(
		lumberjackLogger,
		nil,
		zap.DebugLevel,
		version,
	)

	defer logger.Sync()
	sugar := logger.Sugar()

	defer logOnPanic(sugar)

	if cfgParseErr != nil {
		logAndPrint(
			sugar, os.Stderr,
			"Can't parse config - maybe create the directory?",
			"err", cfgParseErr,
		)
		return cfgParseErr
	}

	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(sugar, configPath, *editConfigCmdEditorFlag)
	case grabCmd.FullCommand():
		if cfgLoadErr != nil {
			logAndPrint(
				sugar, os.Stderr,
				"Config error - try `edit-config`",
				"cfgLoadErr", cfgLoadErr,
				"cfgLoadErrMsg", cfgLoadErr.Error(),
			)
			return cfgLoadErr
		}
		return grab(sugar, subreddits)
	case versionCmd.FullCommand():
		logAndPrint(
			sugar, os.Stdout,
			"Version and build information",
			"builtBy", builtBy,
			"commit", commit,
			"date", date,
			"version", version,
		)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		os.Exit(1)
	}
}
