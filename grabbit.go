package main

import (
	"context"
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
	"time"

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
	Version    string
	Subreddits []subreddit
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

	if fileExists(fileName) && force == false {
		log.Printf("File exists: %v\n", fileName)
		return nil
	}

	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
	}
	defer response.Body.Close()

	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}
	return nil
}

func getTopPosts(ctx context.Context, client *reddit.Client, subredditName string, postLimit int, timeframe string) ([]*reddit.Post, error) {
	posts, _, err := client.Subreddit.TopPosts(ctx, subredditName, &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: postLimit,
		},
		Time: timeframe,
	})
	if err != nil {
		return nil, err
	}
	return posts, nil
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

func grab(configPath string) error {
	configPath, err := homedir.Expand(configPath)
	if err != nil {
		return err
	}

	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}

	userConfig := config{}
	err = yaml.UnmarshalStrict(configBytes, &userConfig)
	if err != nil {
		return err
	}

	client, err := reddit.NewReadonlyClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	for _, subreddit := range userConfig.Subreddits {

		// log.Printf("Processing: %v\n", subreddit.Name)

		fullDest, err := homedir.Expand(subreddit.Destination)
		if err != nil {
			log.Panicf("Cannot expand subreddit destination %v: %v: %v", subreddit.Name, subreddit.Destination, err)
		}

		err = os.MkdirAll(fullDest, os.ModePerm)
		if err != nil {
			log.Panicf("Error creating subreddit destination %v: %v: %v", subreddit.Name, fullDest, err)
		}
		subreddit.Destination = fullDest

		posts, err := getTopPosts(ctx, client, subreddit.Name, subreddit.Limit, subreddit.Timeframe)
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
				}
			} else {
				log.Printf("Could not download: %v: %v\n", subreddit.Name, post.URL)
			}
		}
	}

	return nil
}

func editFile(path string, defaultContent []byte, rm bool) error {

	// TODO: handle erasing

	path, err := homedir.Expand(path)
	if err != nil {
		return err
	}

	// TODO: clean up control flow
	if _, err = os.Stat(path); err == nil {
		// exists, we're good
	} else if os.IsNotExist(err) {
		err = ioutil.WriteFile(path, defaultContent, 0644)
		if err != nil {
			return err
		}
	} else {
		return err
	}

	editor := os.Getenv("EDITOR")
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

	log.Printf("Executing: %s %s", executable, path)

	cmd := exec.Command(executable, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil

}

func editConfig(configPath string) error {
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
	return editFile(configPath, emptyConfigContent, false) // TODO: put this in a flag
}

func run() error {

	// exePath := os.Args[0]
	// exeBytes, err := ioutil.ReadFile(exePath)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("%x", md5.Sum(exeBytes))

	// logger stuff
	// lumberjack.Logger is already safe for concurrent use, so we don't need to
	// lock it.
	lumberjackLogger := &lumberjack.Logger{
		Filename:   "tmp.log",
		MaxSize:    5, // megabytes
		MaxBackups: 0,
		MaxAge:     30, // days
	}
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

	writer := zapcore.AddSync(lumberjackLogger)
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		writer,
		zap.DebugLevel,
	)
	logger := zap.New(core, zap.AddCaller())
	logger = logger.With(zap.Int("pid", os.Getpid()))
	defer logger.Sync()
	sugar := logger.Sugar()

	sugar.Infow("failed to fetch URL",
		// Structured context as loosely typed key-value pairs.
		"url", "https:/www.linkedin.com",
		"attempt", 3,
		"backoff", time.Second,
	)

	// cli and go!
	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/grabbit.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	editConfigCmd := app.Command("edit-config", "Edit or create configuration file. Uses $EDITOR or vim")

	grabCmd := app.Command("grab", "Grab images. Use `edit-config` first to create a config")

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))
	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(*appConfigPathFlag)
	case grabCmd.FullCommand():
		return grab(*appConfigPathFlag)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
