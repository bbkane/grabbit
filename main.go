package main

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/vartanbeno/go-reddit/v2/reddit"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/bbkane/glib"
	"github.com/bbkane/logos"

	w "github.com/bbkane/warg"
	c "github.com/bbkane/warg/command"
	"github.com/bbkane/warg/configreader/yamlreader"
	f "github.com/bbkane/warg/flag"
	s "github.com/bbkane/warg/section"
	v "github.com/bbkane/warg/value"
)

//go:embed embedded/grabbit.yaml
var embeddedConfig []byte

type subreddit struct {
	Name        string
	Destination string
	Timeframe   string
	Limit       int
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

// validateImageURL tries to extract a valid image file name from a URL
// validateImageURL("https://bob.com/img.jpg?abc") -> "img.jpg", nil
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

func grabSubreddit(ctx context.Context, logger *logos.Logger, client *reddit.Client, subreddit subreddit) {
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
		return
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

func editConfig(passedFlags f.FlagValues) error {
	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   passedFlags["--log-filename"].(string),
		MaxAge:     passedFlags["--log-maxage"].(int),
		MaxBackups: passedFlags["--log-maxbackups"].(int),
		MaxSize:    passedFlags["--log-maxsize"].(int),
	}

	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberJackLogger, zap.DebugLevel, getVersion(),
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	configPath := passedFlags["--config-path"].(string)
	editor := passedFlags["--editor"].(string)

	err := glib.EditFile(embeddedConfig, configPath, editor)
	if err != nil {
		logos.Errorw(
			"Unable to edit config",
			"configPath", configPath,
			"editorPath", editor,
			"err", err,
		)
		return err
	}
	return nil
}

func grab(passedFlags f.FlagValues) error {

	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   passedFlags["--log-filename"].(string),
		MaxAge:     passedFlags["--log-maxage"].(int),
		MaxBackups: passedFlags["--log-maxbackups"].(int),
		MaxSize:    passedFlags["--log-maxsize"].(int),
	}

	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberJackLogger, zap.DebugLevel, getVersion(),
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	subredditDestinations := passedFlags["--subreddit-destination"].([]string)
	subredditLimits := passedFlags["--subreddit-limit"].([]int)
	subredditNames := passedFlags["--subreddit-name"].([]string)
	subredditTimeframes := passedFlags["--subreddit-timeframe"].([]string)

	if !(len(subredditDestinations) == len(subredditLimits) &&
		len(subredditLimits) == len(subredditNames) &&
		len(subredditNames) == len(subredditTimeframes)) {
		logger.Errorw(
			"the following lengths should be equal",
			"len(subredditDestinations)", len(subredditDestinations),
			"len(subredditLimits)", len(subredditLimits),
			"len(subredditNames)", len(subredditNames),
			"len(subredditTimeframes)", len(subredditTimeframes),
		)
		return errors.New("Non-matching lengths")
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

	ua := runtime.GOOS + ":" + "grabbit" + ":" + getVersion() + " (github.com/bbkane/grabbit)"
	client, err := reddit.NewReadonlyClient(reddit.WithUserAgent(ua))
	if err != nil {
		err = errors.WithStack(err)
		logger.Errorw(
			"reddit initializion error",
			"err", err,
		)
		return err
	}

	ctx := context.Background()

	for i := 0; i < len(subredditDestinations); i++ {
		grabSubreddit(ctx, logger, client, subreddit{
			Name:        subredditNames[i],
			Destination: subredditDestinations[i],
			Timeframe:   subredditTimeframes[i],
			Limit:       subredditLimits[i],
		})
	}
	return nil
}

func getVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.Main.Version
}

func printVersion(_ f.FlagValues) error {
	fmt.Println(getVersion())
	return nil
}

func main() {
	grabCmd := c.NewCommand(
		"Grab images. Optionally use `config edit` first to create a config",
		grab,
		c.WithFlag(
			"--subreddit-name",
			"subreddit to grab",
			v.StringSlice,
			f.Default("wallpapers"),
			f.ConfigPath("subreddits[].name"),
		),
		c.WithFlag(
			"--subreddit-destination",
			"Where to store the subreddit",
			v.PathSlice,
			f.Default("~/Pictures/grabbit"),
			f.ConfigPath("subreddits[].destination"),
		),
		c.WithFlag(
			"--subreddit-timeframe",
			"Take the top subreddits from this timeframe",
			v.StringSlice,
			f.Default("week"),
			f.ConfigPath("subreddits[].timeframe"),
		),
		c.WithFlag(
			"--subreddit-limit",
			"max number of links to try to download",
			v.IntSlice,
			f.Default("5"),
			f.ConfigPath("subreddits[].limit"),
		),
	)

	appFooter := `Examples (assuming BASH-like shell):

# Grab top images from wallpapers and earthporn
grabbit grab \
    --subreddit-destination ~/Pictures/wallpapers \
    --subreddit-limit 5 \
    --subreddit-name wallpapers \
    --subreddit-timeframe day \
    --subreddit-destination ~/Pictures/earthporn \
    --subreddit-limit 10 \
    --subreddit-name earthporn \
    --subreddit-timeframe week

# Write a config file
echo 'subreddits:
  - destination: ~/Pictures/wallpapers
    limit: 5
    name: wallpapers
    timeframe: day
  - destination: ~/Pictures/earthporn
    limit: 10
    name: earthporn
    timeframe: week
' > ./grabbit.yaml

# Grab from config file
grabbit grab --config-path ./grabbit.yaml
`

	app := w.New(
		"grabbit",
		s.NewSection(
			"Get top images from subreddits",
			s.AddCommand(
				"grab",
				grabCmd,
			),
			s.Footer(appFooter),
			s.WithCommand(
				"version",
				"Print version",
				printVersion,
			),
			s.WithFlag(
				"--log-filename",
				"log filename",
				v.Path,
				f.Default("~/.config/grabbit.jsonl"),
				f.ConfigPath("lumberjacklogger.filename"),
			),
			s.WithFlag(
				"--log-maxage",
				"max age before log rotation in days",
				v.Int,
				f.Default("30"),
				f.ConfigPath("lumberjacklogger.maxage"),
			),
			s.WithFlag(
				"--log-maxbackups",
				"num backups for the log",
				v.Int,
				f.Default("0"),
				f.ConfigPath("lumberjacklogger.maxbackups"),
			),
			s.WithFlag(
				"--log-maxsize",
				"max size of log in megabytes",
				v.Int,
				f.Default("5"),
				f.ConfigPath("lumberjacklogger.maxsize"),
			),
			s.WithSection(
				"config",
				"config commands",
				s.WithCommand(
					"edit",
					"Edit or create configuration file. Uses $EDITOR as a fallback",
					editConfig,
					c.WithFlag(
						"--editor",
						"path to editor",
						v.String,
						f.Default("vi"),
					),
				),
			),
		),
		w.ConfigFlag(
			"--config-path",
			yamlreader.NewYAMLConfigReader,
			"config filepath",
			f.Default("~/.config/grabbit.yaml"),
		),
		w.OverrideHelp(
			os.Stderr,
			[]string{"-h", "--help"},
			w.DefaultSectionHelp,
			w.DefaultCommandHelp,
		),
	)
	app.MustRun()
}
