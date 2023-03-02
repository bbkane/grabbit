package main

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bbkane/glib"
	"github.com/pkg/errors"
	"github.com/vartanbeno/go-reddit/v2/reddit"
	"go.bbkane.com/logos"
	"go.bbkane.com/warg/command"
	"go.uber.org/zap"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

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

	_, err := glib.ValidateDirectory(subreddit.Destination)
	if err != nil {
		logger.Errorw(
			"Directory error",
			"directory", subreddit.Destination,
			"err", err,
		)
		return
	}

	posts, _, err := client.Subreddit.TopPosts(
		ctx,
		subreddit.Name, &reddit.ListPostOptions{
			ListOptions: reddit.ListOptions{
				Limit:  subreddit.Limit,
				After:  "",
				Before: "",
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
					"download file error",
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

func grab(ctx command.Context) error {

	timeout := ctx.Flags["--timeout"].(time.Duration)

	// retrieve types:
	lumberJackLogger := &lumberjack.Logger{
		Filename:   ctx.Flags["--log-filename"].(string),
		MaxAge:     ctx.Flags["--log-maxage"].(int),
		MaxBackups: ctx.Flags["--log-maxbackups"].(int),
		MaxSize:    ctx.Flags["--log-maxsize"].(int),
		LocalTime:  true,
		Compress:   false,
	}

	logger := logos.NewLogger(
		logos.NewZapSugaredLogger(
			lumberJackLogger, zap.DebugLevel, getVersion(),
		),
	)
	defer logger.Sync()
	logger.LogOnPanic()

	subredditDestinations := ctx.Flags["--subreddit-destination"].([]string)
	subredditLimits := ctx.Flags["--subreddit-limit"].([]int)
	subredditNames := ctx.Flags["--subreddit-name"].([]string)
	subredditTimeframes := ctx.Flags["--subreddit-timeframe"].([]string)

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
			&net.Dialer{
				Timeout:        time.Second * 30,
				Deadline:       time.Time{},
				LocalAddr:      nil,
				DualStack:      false,
				FallbackDelay:  0,
				KeepAlive:      0,
				Resolver:       nil,
				Cancel:         nil,
				Control:        nil,
				ControlContext: nil,
			},
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

	ua := runtime.GOOS + ":" + "grabbit" + ":" + getVersion() + " (go.bbkane.com/grabbit)"

	// The reddit API does not like HTTP/2
	// Per https://pkg.go.dev/net/http?utm_source=gopls#pkg-overview ,
	// I'm copying http.DefaultTransport and replacing the HTTP/2 stuff
	transport := &http.Transport{
		Dial:                   nil,
		DialTLSContext:         nil,
		DialTLS:                nil,
		TLSClientConfig:        nil,
		DisableKeepAlives:      false,
		DisableCompression:     false,
		MaxIdleConnsPerHost:    0,
		MaxConnsPerHost:        0,
		ResponseHeaderTimeout:  0,
		ProxyConnectHeader:     nil,
		GetProxyConnectHeader:  nil,
		MaxResponseHeaderBytes: 0,
		WriteBufferSize:        0,
		ReadBufferSize:         0,
		Proxy:                  http.ProxyFromEnvironment,
		OnProxyConnectResponse: nil,
		DialContext: (&net.Dialer{
			Timeout:        30 * time.Second,
			KeepAlive:      30 * time.Second,
			Deadline:       time.Time{},
			LocalAddr:      nil,
			DualStack:      false,
			FallbackDelay:  0,
			Resolver:       nil,
			Cancel:         nil,
			Control:        nil,
			ControlContext: nil,
		}).DialContext,

		// change from default
		ForceAttemptHTTP2: false,

		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,

		// use an empty map instead of nil per the link above
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: nil,
		Jar:           nil,
	}

	client, err := reddit.NewReadonlyClient(
		reddit.WithUserAgent(ua),
		reddit.WithHTTPClient(httpClient),
	)
	if err != nil {
		err = errors.WithStack(err)
		logger.Errorw(
			"reddit initializion error",
			"err", err,
		)
		return err
	}

	timeoutCtx := context.Background()

	for i := 0; i < len(subredditDestinations); i++ {
		grabSubreddit(timeoutCtx, logger, client, subreddit{
			Name:        subredditNames[i],
			Destination: subredditDestinations[i],
			Timeframe:   subredditTimeframes[i],
			Limit:       subredditLimits[i],
		})
	}
	return nil
}
