package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/vartanbeno/go-reddit/v2/reddit"
	"go.uber.org/zap"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v2"

	"github.com/bbkane/glib"
	"github.com/bbkane/logos"

	w "github.com/bbkane/warg"
	c "github.com/bbkane/warg/command"
	"github.com/bbkane/warg/configreader/yamlreader"
	f "github.com/bbkane/warg/flag"
	s "github.com/bbkane/warg/section"
	v "github.com/bbkane/warg/value"
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

func genFilePath(destinationDir string, subredditName string, title string, urlFileName string) (string, error) {

	for _, s := range []string{" ", "/", "\\", "\n", "\r", "\x00"} {
		urlFileName = strings.ReplaceAll(urlFileName, s, "_")
		subredditName = strings.ReplaceAll(subredditName, s, "_")
		title = strings.ReplaceAll(title, s, "_")
	}

	fullFileName := subredditName + "_" + title + "_" + urlFileName
	filePath := filepath.Join(destinationDir, fullFileName)

	// remove chars from title if it's too long for the OS to handle
	const maxPathLength = 250
	if len(filePath) > maxPathLength {
		toChop := len(filePath) - maxPathLength
		if toChop > len(title) {
			return "", errors.Errorf("filePath to long and title too short: %#v\n", filePath)
		}

		title = title[:len(title)-toChop]
		fullFileName = subredditName + "_" + title + "_" + urlFileName
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
		dirPath, err := glib.ValidateDirectory(sr.Destination)
		if err != nil {
			return lumberjackLogger, []subreddit{}, errors.Wrapf(err, "Directory in config error: %v\n", sr.Destination)
		}
		sr.Destination = dirPath

		subreddits = append(subreddits, sr)
	}

	return lumberjackLogger, subreddits, nil
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

func grab(logger *logos.Logger, subreddits []subreddit) error {
	ua := runtime.GOOS + ":" + "grabbit" + ":" + version + " (github.com/bbkane/grabbit)"
	client, err := reddit.NewReadonlyClient(reddit.WithUserAgent(ua))
	if err != nil {
		err = errors.WithStack(err)
		logger.Errorw(
			"reddit initializion error",
			"err", err,
		)
		return err
	}

	// Test connection
	{
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: time.Second * 30},
			"tcp",
			net.JoinHostPort("reddit.com", "443"),
			nil,
		)
		if err != nil {
			err = errors.WithStack(err)
			logger.Errorw(
				"Can't connect to reddit",
				"conn", conn,
				"err", err,
			)
			return err
		}
		err = conn.Close()
		if err != nil {
			err = errors.WithStack(err)
			logger.Errorw(
				"Can't close connection",
				"conn", conn,
				"err", err,
			)
			return err
		}
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
			logger.Errorw(
				"Can't use subreddit",
				"subreddit", subreddit.Name,
				"err", errors.WithStack(err),
			)
			continue
		}

		for _, post := range posts {
			if post.NSFW {
				logger.Errorw(
					"Skipping NSFW post",
					"subreddit", subreddit.Name,
					"post", post.Title,
					"url", post.URL,
				)
				continue
			}
			urlFileName, err := validateImageURL(post.URL)
			if err != nil {
				logger.Errorw(
					"can't download image",
					"subreddit", subreddit.Name,
					"post", post.Title,
					"url", post.URL,
					"err", err,
				)
				continue
			}

			filePath, err := genFilePath(subreddit.Destination, subreddit.Name, post.Title, urlFileName)
			if err != nil {
				logger.Errorw(
					"genFilePath err",
					"subreddit", subreddit.Name,
					"post", post.Title,
					"url", post.URL,
					"err", errors.WithStack(err),
				)
				continue
			}
			err = downloadImage(post.URL, filePath)
			if err != nil {
				if os.IsExist(errors.Cause(err)) {
					logger.Infow(
						"file exists!",
						"subreddit", subreddit.Name,
						"post", post.Title,
						"filePath", filePath,
						"url", post.URL,
					)
					continue
				} else {
					logger.Errorw(
						"downloadFile error",
						"subreddit", subreddit.Name,
						"post", post.Title,
						"url", post.URL,
						"err", errors.WithStack(err),
					)
				}
				continue

			}
			logger.Infow(
				"downloaded file",
				"subreddit", subreddit.Name,
				"post", post.Title,
				"filePath", filePath,
				"url", post.URL,
			)
		}
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
		logos.Errorw(
			"config error",
			"err", err,
		)
	}

	if cmd == configCmdEditCmd.FullCommand() {
		err = glib.EditFile(embeddedConfig, *appConfigPathFlag, *configCmdEditCmdEditorFlag)
		if err != nil {
			logos.Errorw(
				"Unable to edit config",
				"configPath", *appConfigPathFlag,
				"editorPath", *configCmdEditCmdEditorFlag,
				"err", err,
			)
			return err
		}
		return nil
	}

	if cmd == versionCmd.FullCommand() {
		logos.Infow(
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
			logos.Errorw(
				"Config error - try `config edit`",
				"cfgLoadErr", cfgLoadErr,
				"cfgLoadErrMsg", cfgLoadErr.Error(),
			)
			return cfgLoadErr
		}
	}

	lumberjackLogger, subreddits, cfgParseErr := parseConfig(configBytes)
	if cfgParseErr != nil {
		logos.Errorw(
			"Can't parse config",
			"err", cfgParseErr,
		)
		return cfgParseErr
	}

	// get a logger
	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberjackLogger, zap.DebugLevel, version,
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	// dispatch commands that use dependencies
	switch cmd {
	case grabCmd.FullCommand():
		return grab(logger, subreddits)
	default:
		err = errors.Errorf("Unknown command: %#v\n", cmd)
		logger.Errorw(
			"Unknown command",
			"cmd", cmd,
			"err", err,
		)
		return err
	}
}

func run2() error {
	app := w.New(
		"grabbit",
		"v0.0.0",
		s.NewSection(
			"Get top images from subreddits",
			s.WithCommand(
				"grab",
				"Grab images. Use `config edit` first to create a config",
				c.DoNothing,
				c.WithFlag(
					"--subreddit-name",
					"subreddit to grab",
					v.StringSliceEmpty,
					f.Default("wallpapers"),
					f.ConfigPath("subreddits[].name", v.StringSliceFromInterface),
				),
				c.WithFlag(
					"--subreddit-destination",
					"Where to store the subreddit",
					v.StringSliceEmpty,
					f.Default("~/Pictures/grabbit"),
					f.ConfigPath("subreddits[].destination", v.StringSliceFromInterface),
				),
				c.WithFlag(
					"--subreddit-timeframe",
					"Take the top subreddits from this timeframe",
					v.StringSliceEmpty,
					f.Default("week"),
					f.ConfigPath("subreddits[].timeframe", v.StringSliceFromInterface),
				),
				c.WithFlag(
					"--subreddit-limit",
					"max number of links to try to download",
					v.IntSliceEmpty,
					f.Default("5"),
					f.ConfigPath("subreddits[].limit", v.IntSliceFromInterface),
				),
			),
			s.WithFlag(
				"--log-filename",
				"log filename",
				v.StringEmpty,
				f.Default("~/.config/grabbit.jsonl"),
				f.ConfigPath("lumberjacklogger.filename", v.StringFromInterface),
			),
			s.WithFlag(
				"--log-maxage",
				"max age before log rotation in days",
				v.IntEmpty,
				f.Default("30"),
				f.ConfigPath("lumberjacklogger.maxage", v.IntFromInterface),
			),
			s.WithFlag(
				"--log-maxbackups",
				"num backups for the log",
				v.IntEmpty,
				f.Default("0"),
				f.ConfigPath("lumberjacklogger.maxbackups", v.IntFromInterface),
			),
			s.WithFlag(
				"--log-maxsize",
				"max size of log in megabytes",
				v.IntEmpty,
				f.Default("5"),
				f.ConfigPath("lumberjacklogger.maxsize", v.IntFromInterface),
			),
			s.WithSection(
				"config",
				"config commands",
				s.WithCommand(
					"edit",
					"Edit or create configuration file. Uses $EDITOR as a fallback",
					c.DoNothing,
					c.WithFlag(
						"--editor",
						"path to editor",
						v.StringEmpty,
						f.Default("vi"),
					),
				),
			),
		),
		w.ConfigFlag(
			"--config-path",
			yamlreader.NewYAMLConfigReader,
			"config filepath",
			// f.Default("/Users/bbkane/tmp.json"),
			f.Default("/Users/bbkane/tmp.yaml"),
		),
	)
	err := app.Run(os.Args)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := run2()
	if err != nil {
		log.Panic(err)
	}
}
