package main

import (
	"context"
	"errors"
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

	"github.com/mitchellh/go-homedir"
	"github.com/natefinch/lumberjack"
	"github.com/vartanbeno/go-reddit/reddit"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

var errNotAtTheDisco = errors.New("Don't panic, just os.Exit(1)")

type subreddit struct {
	Name        string
	Destination string
	Timeframe   string
	Limit       int
}

type config struct {
	Version          string
	LumberjackLogger *lumberjack.Logger
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

func downloadFile(URL string, fileName string, force bool) error {
	// https://golangbyexample.com/download-image-file-url-golang/

	if force == false && fileExists(fileName) {
		log.Printf("File exists: %v\n", fileName)
		return nil
	}

	response, err := http.Get(URL)
	if err != nil {
	}
	defer response.Body.Close()

	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func genFilePath(destinationDir string, title string, fullURL string) (string, error) {
	// https://www.golangprograms.com/golang-download-image-from-given-url.html
	fileURL, err := url.Parse(fullURL)
	if err != nil {
		return "", err
	}

	path := fileURL.Path
	segments := strings.Split(path, "/")

	fileName := segments[len(segments)-1]

	if err != nil {
		return "", err
	}

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
		return nil, nil, err
	}

	cfg := config{}
	err = yaml.UnmarshalStrict(configBytes, &cfg)
	if err != nil {
		return nil, nil, err
	}

	var lumberjackLogger *lumberjack.Logger = nil

	// we can get a valid config with a nil logger
	if cfg.LumberjackLogger != nil {
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			panic(err)
		}
		cfg.LumberjackLogger.Filename = f
		lumberjackLogger = cfg.LumberjackLogger
	}

	// TODO: figure out memory management here
	// why do I have to make a new list? Is it because I get a new
	// copy of subreddit in the for loop and it just disappears each iteration?
	// subreddits := make([]subreddit)
	subreddits := make([]subreddit, 0)
	for _, sr := range cfg.Subreddits {
		fullDest, err := homedir.Expand(sr.Destination)
		if err != nil {
			log.Panicf("Cannot expand subreddit destination %v: %v: %v", sr.Name, sr.Destination, err)
		}
		sr.Destination = fullDest
		subreddits = append(subreddits, sr)
	}

	return lumberjackLogger, subreddits, nil
}

func grab(subreddits []subreddit) error {

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
			log.Printf("getTopPosts: %v: %v\n", subreddit, err)
		}

		for _, post := range posts {
			if strings.HasSuffix(post.URL, ".jpg") {

				filePath, err := genFilePath(subreddit.Destination, post.Title, post.URL)
				if err != nil {
					log.Printf("genFilePath: %v: %v: %v\n", subreddit.Name, post.URL, err)
				}
				err = downloadFile(post.URL, filePath, false)
				if err != nil {
					log.Printf("downloadFile: %v: %v: %v\n", subreddit.Name, post.URL, err)
				} else {
					log.Printf("downloaded file: %v: %v\n", subreddit.Name, post.URL)
				}
			} else {
				log.Printf("Could not download: %v: %v\n", subreddit.Name, post.URL)
			}
		}
	}

	return nil
}

func editConfig(configPath string, editor string) error {
	emptyConfigContent := []byte(`version: 1.0.0
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

	log.Printf("Executing: %s %s", executable, configPath)

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
		zap.Fields(zap.Int("pid", os.Getpid())),
	)

	return logger
}

func run() error {

	// logger stuff
	// lumberjack.Logger is already safe for concurrent use, so we don't need to
	// lock it.
	// lumberjackLogger := &lumberjack.Logger{
	// 	Filename:   "tmp.log",
	// 	MaxSize:    5, // megabytes
	// 	MaxBackups: 0,
	// 	MaxAge:     30, // days
	// }

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
		os.Stderr,
		zap.DebugLevel,
		zap.ErrorLevel,
	)

	defer logger.Sync()
	sugar := logger.Sugar()

	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(configPath, *editConfigCmdEditorFlag)
	case grabCmd.FullCommand():
		if cfgErr != nil {
			sugar.Errorw(
				"Config error: maybe try `edit-config`",
				"err", cfgErr,
			)
			fmt.Fprintf(os.Stderr, "Config error: maybe try `edit-config`: %v\n", cfgErr)
			return errNotAtTheDisco
		}
		return grab(subreddits)
	}

	return nil
}

func main() {
	err := run()
	if err == errNotAtTheDisco {
		// os.Exit(1)
	} else if err != nil {
		panic(err)
	}
}
