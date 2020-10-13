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
			if strings.HasSuffix(post.URL, ".jpg") {

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

				} else {
					logAndPrint(sugar, os.Stdout,
						"downloaded file",
						"subreddit", subreddit.Name,
						"filePath", filePath,
						"url", post.URL,
					)
				}
			} else {
				logAndPrint(sugar, os.Stderr,
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
		logAndPrint(
			sugar, os.Stderr,
			"editor cmd error",
			"err", err,
		)
		return err
	}

	return nil
}

// newLogger builds a logger. if lumberjackLogger or fp are nil, then that respective sink won't be made
func newLogger(lumberjackLogger *lumberjack.Logger, fp *os.File, lvl zapcore.LevelEnabler) *zap.Logger {
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
		zap.Fields(zap.Int("pid", os.Getpid())),
	)

	return logger
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

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	configPath, err := homedir.Expand(*appConfigPathFlag)
	if err != nil {
		// to early to log
		fmt.Fprintf(os.Stderr, "config error: %#v\n", err)
		panic(err)
	}

	lumberjackLogger, subreddits, cfgErr := readConfig(configPath)

	logger := newLogger(
		lumberjackLogger,
		// os.Stdout, // TODO: get an os.File from readConfig to put in here - os.Stderr
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
			fmt.Fprintf(os.Stderr, "Config error: maybe try `edit-config`: %v\n", cfgErr)
			return cfgErr
		}
		return grab(sugar, subreddits)
	}

	return nil
}

// logAndPrint produces a more human readable error message for the console.
// It's really only designed for simple keys/value messages It prints Info if
// fp == os.Stdout, Error if fp == os.Stderr, and panics otherwise.
func logAndPrint(sugar *zap.SugaredLogger, fp *os.File, msg string, keysAndValues ...interface{}) {

	switch fp {
	case os.Stdout:
		sugar.Infow(msg, keysAndValues...)
		msg = "INFO: " + msg
	case os.Stderr:
		sugar.Errorw(msg, keysAndValues...)
		msg = "ERROR: " + msg
	default:
		log.Panicf("logAndPrint: fp not os.Stderr or os.Stdout: %#v\n", fp)
	}

	length := len(keysAndValues)
	if length%2 != 0 {
		log.Panicf("printMsgKeysAndValues: len() not even: %#v\n", keysAndValues...)
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

func main() {
	err := run()
	if err != nil {
		panic(err)
	}
}
