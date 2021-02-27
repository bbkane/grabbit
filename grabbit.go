package main

import (
	"context"
	_ "embed"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bbkane/grabbit/sugarkane"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/vartanbeno/go-reddit/reddit"
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v2"
)

// These will be overwritten by goreleaser
var version = "devVersion"
var commit = "devCommit"
var date = "devDate"
var builtBy = "devBuiltBy"

//go:embed embedded/grabbit.yaml
var embeddedConfig []byte

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

// downloadImage does not overwrite existing files
func downloadImage(URL string, fileName string) error {

	// TODO: add tests! This is tricksy

	// TODO: use the following process to get the right file extension
	// We need the file extension to check whether the file exists when we open
	// it
	// - try to parse it from the URL
	// - try a HEAD request from the server
	// - download 512 bytes from a GET request and check the mime type maybe (or give up :D)

	// putting the file logic first because it's the cheapest
	// O_EXCL - used with O_CREATE, file must not exist

	// Gonna use os.O_TRUNC now to always create the file for testing
	// file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()

	err = func() error {
		response, err := http.Get(URL)
		if err != nil {
			return errors.WithStack(err)
		}
		defer response.Body.Close()

		// -- make sure Content-Type is an image

		contentBytes := make([]byte, 512)

		_, err = response.Body.Read(contentBytes)
		if err != nil {
			return errors.Wrapf(err, "Could not read contentBytes: %+v\n", URL)
		}

		// https://golang.org/pkg/net/http/#DetectContentType
		contentType := http.DetectContentType(contentBytes)

		if !(contentType == "image/jpeg" || contentType == "image/png") {
			err = errors.Errorf("contentType is not 'image/jpeg' or 'image/png': %+v\n", contentType)
			return err
		}

		_, err = file.Write(contentBytes)
		if err != nil {
			return errors.Wrapf(err, "can't write contentBytes to file: %+v, %+v\n", URL, file.Name())
		}

		_, err = io.Copy(file, response.Body)
		if err != nil {
			return errors.Wrapf(err, "Can't copy to file: %+v, %+v\n", URL, file.Name())
		}
		return nil
	}()
	if err != nil {
		_ = os.Remove(fileName)
	}
	return err
}

func genFilePath(destinationDir string, subreddit string, title string, urlFileName string) (string, error) {

	for _, s := range []string{" ", "/", "\\", "\n", "\r", "\x00"} {
		urlFileName = strings.ReplaceAll(urlFileName, s, "_")
		subreddit = strings.ReplaceAll(subreddit, s, "_")
		title = strings.ReplaceAll(title, s, "_")
	}

	fullFileName := subreddit + "_" + title + "_" + urlFileName
	filePath := filepath.Join(destinationDir, fullFileName)

	// remove chars from title if it's too long for the OS to handle
	const maxPathLength = 250
	if len(filePath) > maxPathLength {
		toChop := len(filePath) - maxPathLength
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
		// Note that if the directories to here don't exist, lumberjack will
		// make them
		f, err := homedir.Expand(cfg.LumberjackLogger.Filename)
		if err != nil {
			return nil, []subreddit{}, errors.WithStack(err)
		}
		cfg.LumberjackLogger.Filename = f
		lumberjackLogger = cfg.LumberjackLogger
	}

	subreddits := make([]subreddit, 0)
	for _, sr := range cfg.Subreddits {
		dirPath, err := validateDirectory(sr.Destination)
		if err != nil {
			return lumberjackLogger, []subreddit{}, errors.Wrapf(err, "Directory in config error: %v\n", sr.Destination)
		}
		sr.Destination = dirPath

		subreddits = append(subreddits, sr)
	}

	return lumberjackLogger, subreddits, nil
}

// validateDirectory expands a directory and checks that it exists
// it returns the full path to the directory on success
// validateDirectory("~/foo") -> ("/home/bbkane/foo", nil)
func validateDirectory(dir string) (string, error) {
	dirPath, err := homedir.Expand(dir)
	if err != nil {
		return "", errors.WithStack(err)
	}
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		return "", errors.Wrapf(err, "Directory does not exist: %v\n", dirPath)
	}
	if err != nil {
		return "", errors.Wrapf(err, "Directory error: %v\n", dirPath)

	}
	if !info.IsDir() {
		return "", errors.Errorf("Directory is a file, not a directory: %#v\n", dirPath)
	}
	return dirPath, nil
}

// validateImageURL tries to extract a valid image file name from a URL
// validateImageURL("https://bob.com/img.jpg?abc") -> nil, "img.jpg"
func validateImageURL(fullURL string) (string, error) {
	fileURL, err := url.Parse(fullURL)
	if err != nil {
		return "", errors.WithStack(err)
	}

	path := fileURL.Path
	segments := strings.Split(path, "/")

	urlFileName := segments[len(segments)-1]
	allowedImageExtensions := []string{".jpg", ".jpeg", ".png"}
	for _, suffix := range allowedImageExtensions {
		if strings.HasSuffix(urlFileName, suffix) {
			return urlFileName, nil
		}

	}
	return "", errors.Errorf("urlFileName doesn't end in allowed extension: %#v , %#v\n ", urlFileName, allowedImageExtensions)
}

func grab(sk *sugarkane.SugarKane, subreddits []subreddit) error {
	ua := runtime.GOOS + ":" + "grabbit" + ":" + version + " (github.com/bbkane/grabbit)"
	client, err := reddit.NewReadonlyClient(reddit.WithUserAgent(ua))
	if err != nil {
		err = errors.WithStack(err)
		sk.Errorw(
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
			sk.Errorw(
				"Can't use subreddit",
				"subreddit", subreddit,
				"err", errors.WithStack(err),
			)
			continue
		}

		for _, post := range posts {
			urlFileName, err := validateImageURL(post.URL)
			if err != nil {
				sk.Errorw(
					"can't download image",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", err,
				)
				continue
			}

			filePath, err := genFilePath(subreddit.Destination, subreddit.Name, post.Title, urlFileName)
			if err != nil {
				sk.Errorw(
					"genFilePath err",
					"subreddit", subreddit.Name,
					"url", post.URL,
					"err", errors.WithStack(err),
				)
				continue
			}
			err = downloadImage(post.URL, filePath)
			if err != nil {
				if os.IsExist(errors.Cause(err)) {
					sk.Infow(
						"file exists!",
						"subreddit", subreddit.Name,
						"filePath", filePath,
						"url", post.URL,
					)
				} else {
					sk.Errorw(
						"downloadFile error",
						"subreddit", subreddit.Name,
						"url", post.URL,
						"err", errors.WithStack(err),
					)
				}
				continue

			}
			sk.Infow(
				"downloaded file",
				"subreddit", subreddit.Name,
				"filePath", filePath,
				"url", post.URL,
			)
		}
	}
	return nil
}

func editConfig(configPath string, editor string) error {
	// TODO: make this a serialized config struct
	// so I get a compile warning if there's problems

	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		err = ioutil.WriteFile(configPath, embeddedConfig, 0644)
		if err != nil {
			sugarkane.Printw(os.Stderr,
				"can't write config",
				"err", err,
			)
			return err
		}
		sugarkane.Printw(os.Stdout,
			"wrote default config",
			"configPath", configPath,
		)
	} else if err != nil {
		sugarkane.Printw(os.Stderr,
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
		sugarkane.Printw(os.Stderr,
			"can't find editor",
			"err", err,
		)
		return err
	}

	sugarkane.Printw(os.Stderr,
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
		sugarkane.Printw(os.Stderr,
			"editor cmd error",
			"err", err,
		)
		return err
	}

	return nil
}

func run() error {

	// parse the CLI args
	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')
	defaultConfigPath := "~/.config/grabbit.yaml"
	appConfigPathFlag := app.Flag("config-path", "config filepath").Short('c').Default(defaultConfigPath).String()

	configCmd := app.Command("config", "config commands")
	configCmdEditCmd := configCmd.Command("edit", "Edit or create configuration file. Uses $EDITOR as a fallback")
	configCmdEditCmdEditorFlag := configCmdEditCmd.Flag("editor", "path to editor").Short('e').String()

	grabCmd := app.Command("grab", "Grab images. Use `config edit` first to create a config")

	versionCmd := app.Command("version", "print grabbit build and version information")

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	// work with commands that don't have dependencies (version, editConfig)
	configPath, err := homedir.Expand(*appConfigPathFlag)
	if err != nil {
		err = errors.WithStack(err)
		sugarkane.Printw(os.Stderr,
			"config error",
			"err", err,
		)
	}

	if cmd == configCmdEditCmd.FullCommand() {
		return editConfig(configPath, *configCmdEditCmdEditorFlag)
	}
	if cmd == versionCmd.FullCommand() {
		sugarkane.Printw(os.Stdout,
			"Version and build information",
			"builtBy", builtBy,
			"commit", commit,
			"date", date,
			"version", version,
		)
		return nil
	}

	// get a config
	configBytes, cfgLoadErr := ioutil.ReadFile(configPath)
	if cfgLoadErr != nil {
		if cfgLoadErr != nil {
			sugarkane.Printw(os.Stderr,
				"Config error - try `config edit`",
				"cfgLoadErr", cfgLoadErr,
				"cfgLoadErrMsg", cfgLoadErr.Error(),
			)
			return cfgLoadErr
		}
	}

	lumberjackLogger, subreddits, cfgParseErr := parseConfig(configBytes)
	if cfgParseErr != nil {
		sugarkane.Printw(os.Stderr,
			"Can't parse config",
			"err", cfgParseErr,
		)
		return cfgParseErr
	}

	// get a logger
	sk := sugarkane.NewSugarKane(lumberjackLogger, os.Stderr, os.Stdout, zap.DebugLevel, version)
	defer sk.Sync()
	sk.LogOnPanic()

	// dispatch commands that use dependencies
	switch cmd {
	case grabCmd.FullCommand():
		return grab(sk, subreddits)
	default:
		err = errors.Errorf("Unknown command: %#v\n", cmd)
		sk.Errorw(
			"Unknown command",
			"cmd", cmd,
			"err", err,
		)
		return err
	}
}

func main() {
	err := run()
	if err != nil {
		os.Exit(1)
	}
}
