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
	"go.uber.org/zap/zapcore"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

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

func readConfig(configPath string) (*lumberjack.Logger, []subreddit, error) {

	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		// It's ok to not have a config (first run, for example)
		return nil, nil, errors.WithStack(err)
	}

	cfg := config{}
	err = yaml.UnmarshalStrict(configBytes, &cfg)
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

	return lumberjackLogger, subreddits, nil
}

func grab(sugar *zap.SugaredLogger, subreddits []subreddit) error {

	client, err := reddit.NewReadonlyClient()
	if err != nil {
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
			sugar.Errorw("Can't use subreddit",
				"subreddit", subreddit,
				"err", errors.WithStack(err),
			)
		}

		for _, post := range posts {
			if strings.HasSuffix(post.URL, ".jpg") {

				filePath, err := genFilePath(subreddit.Destination, post.Title, post.URL)
				if err != nil {
					sugar.Errorw("genFilePath err",
						"subreddit", subreddit.Name,
						"url", post.URL,
						"err", errors.WithStack(err),
					)
				}
				if fileExists(filePath) {
					sugar.Infow(
						"file exists",
						"filePath", filePath,
					)
					continue
				}
				err = downloadFile(post.URL, filePath)
				if err != nil {
					sugar.Errorw(
						"downloadFile",
						"subreddit", subreddit.Name,
						"url", post.URL,
						"err", errors.WithStack(err),
					)

				} else {
					sugar.Infow(
						"downloaded file",
						"subreddit", subreddit.Name,
						"filePath", filePath,
						"url", post.URL,
					)
				}
			} else {
				sugar.Errorw(
					"Could not download",
					"subreddit", subreddit.Name,
					"url", post.URL,
				)
			}
		}
	}

	return nil
}

func editConfig(sugar *zap.SugaredLogger, configPath string, editor string) error {
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
			return err
		}
	} else if err != nil {
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
		return err
	}

	fmt.Printf("Executing: %s %s\n", executable, configPath)
	sugar.Infow("Executing",
		"editor", executable,
		"configPath", configPath,
	)

	cmd := exec.Command(executable, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}

// newLogger builds a logger. if lumberjackLogger or fp are nil, then that respective sink won't be made
func newLogger(lumberjackLogger *lumberjack.Logger, fp *os.File, lvl zapcore.LevelEnabler, stackTraceLvl zapcore.LevelEnabler) *zap.Logger {
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
		zap.AddStacktrace(stackTraceLvl),
		// TODO: replace with version (goreleaser embeds it)
		zap.Fields(zap.Int("pid", os.Getpid())),
	)

	return logger
}

func run() error {

	// cli and go!
	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/grabbit.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	editConfigCmd := app.Command("edit-config", "Edit or create configuration file. Uses $EDITOR as a fallback")
	editConfigCmdEditorFlag := editConfigCmd.Flag("editor", "path to editor").Short('e').String()

	grabCmd := app.Command("grab", "Grab images. Use `edit-config` first to create a config")

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	configPath, err := homedir.Expand(*appConfigPathFlag)
	if err != nil {
		panic(err)
	}

	lumberjackLogger, subreddits, cfgErr := readConfig(configPath)

	logger := newLogger(
		lumberjackLogger,
		nil, // TODO: get an os.File from readConfig to put in here - os.Stderr
		zap.DebugLevel,
		zap.ErrorLevel,
	)

	defer logger.Sync()
	sugar := logger.Sugar()

	logErrAndReturn := func(err error) error {
		if err != nil {
			sugar.Errorw("Error",
				"err", err,
			)
		}
		return err
	}

	switch cmd {
	case editConfigCmd.FullCommand():
		return logErrAndReturn(editConfig(sugar, configPath, *editConfigCmdEditorFlag))
	case grabCmd.FullCommand():
		if cfgErr != nil {
			fmt.Fprintf(os.Stderr, "Config error: maybe try `edit-config`: %v\n", cfgErr)
			logErrAndReturn(cfgErr)
		}
		return logErrAndReturn(grab(sugar, subreddits))
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		panic(err)
	}
}
