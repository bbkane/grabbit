package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"github.com/mitchellh/go-homedir"
	"github.com/natefinch/lumberjack"
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

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors. Notably, it ignores other errors LOL
func fileExists(fileName string) bool {
	// https://golangcode.com/check-if-a-file-exists/
	info, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func downloadFile(URL string, fileName string) error {
	// https://golangbyexample.com/download-image-file-url-golang/

	response, err := http.Get(URL)
	if err != nil {
		return errors.WithStack(err)
	}
	defer response.Body.Close()

	file, err := os.Create(fileName)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func genFilePath(destinationDir string, title string, fullURL string) (string, error) {
	// https://www.golangprograms.com/golang-download-image-from-given-url.html
	fileURL, err := url.Parse(fullURL)
	if err != nil {
		return "", errors.WithStack(err)
	}

	path := fileURL.Path
	segments := strings.Split(path, "/")

	fileName := segments[len(segments)-1]

	for _, s := range []string{" ", "/", "\\", "\n", "\r", "\x00"} {
		title = strings.ReplaceAll(title, s, "_")
	}
	fileName = title + "_" + fileName
	fileName = filepath.Join(destinationDir, fileName)
	return fileName, nil
}

func readConfig(configBytes []byte) (*lumberjack.Logger, []subreddit) {

	cfg := config{}
	err := yaml.UnmarshalStrict(configBytes, &cfg)
	if err != nil {
		// not ok to get invalid YAML
		log.Panicf("readConfig: yaml decode: %+v\n", errors.WithStack(err))
	}

	var lumberjackLogger *lumberjack.Logger = nil

	// we can get a valid config with a nil logger
	if cfg.LumberjackLogger != nil {
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			log.Panicf("readConfig: expand lumberjack: %+v\n", errors.WithStack(err))
		}
		cfg.LumberjackLogger.Filename = f
		lumberjackLogger = cfg.LumberjackLogger
	}

	subreddits := make([]subreddit, 0)
	for _, sr := range cfg.Subreddits {
		fullDest, err := homedir.Expand(sr.Destination)
		if err != nil {
			log.Panicf("readConfig: Cannot expand subreddit destination %v: %v: %v", sr.Name, sr.Destination, err)
		}
		sr.Destination = fullDest
		subreddits = append(subreddits, sr)
	}

	return lumberjackLogger, subreddits
}

func isImage(URL string) error {
	if strings.HasSuffix(URL, ".jpg") {
		return nil
	}
	return errors.New("string doesn't end in .jpg")
}

func grab(sugar *zap.SugaredLogger, subreddits []subreddit) error {

	client, err := reddit.NewReadonlyClient()
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

		info, err := os.Stat(subreddit.Destination)
		// NOTE: should I move these checks to config parsing time?
		if err != nil {
			logAndPrint(sugar, os.Stderr,
				"destination error - skipping all posts",
				"subreddit", subreddit.Name,
				"directory", subreddit.Destination,
				"err", errors.WithStack(err),
			)
			continue
		}
		if !info.IsDir() {
			logAndPrint(sugar, os.Stderr,
				"Is not directory - skipping all posts",
				"subreddit", subreddit.Name,
				"directory", subreddit.Destination,
			)
			continue
		}

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

			filePath, err := genFilePath(subreddit.Destination, post.Title, post.URL)
			if err != nil {
				logAndPrint(sugar, os.Stderr,
					"genFilePath err",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", errors.WithStack(err),
				)
				continue
			}
			if fileExists(filePath) {
				logAndPrint(sugar, os.Stdout,
					"file already exists!",
					"filePath", filePath,
				)
				continue
			}
			err = downloadFile(post.URL, filePath)
			if err != nil {
				logAndPrint(sugar, os.Stderr,
					"downloadFile error",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", errors.WithStack(err),
				)
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
		logAndPrint(
			sugar, os.Stdout,
			"wrote default config",
			"configPath", configPath,
		)
		if err != nil {
			logAndPrint(
				sugar, os.Stderr,
				"can't write config",
				"err", err,
			)
			return err
		}
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
		"Opening editor",
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

	configBytes, cfgErr := ioutil.ReadFile(configPath)

	// if configBytes == []byte{}, then this will be
	// defaulted to nothing and the logging won't work
	// Stuff will still be printed out with logAndPrint though
	lumberjackLogger, subreddits := readConfig(configBytes)

	logger := newLogger(
		lumberjackLogger,
		nil,
		zap.DebugLevel,
	)

	defer logger.Sync()
	sugar := logger.Sugar()

	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(sugar, configPath, *editConfigCmdEditorFlag)
	case grabCmd.FullCommand():
		if cfgErr != nil {
			logAndPrint(
				sugar, os.Stderr,
				"Config error - try `edit-config`",
				"cfgErr", cfgErr,
				"cfgErrMsg", cfgErr.Error(),
			)
			return cfgErr
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
