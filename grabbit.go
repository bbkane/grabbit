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
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/vartanbeno/go-reddit/reddit"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
)

type Subreddit struct {
	Name        string
	Destination string
	Timeframe   string
	Limit       int
}

type Config struct {
	Version    string
	Subreddits []Subreddit
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

func fileNameFromUrl(fullUrl string) (string, error) {
	// https://www.golangprograms.com/golang-download-image-from-given-url.html
	fileUrl, err := url.Parse(fullUrl)
	if err != nil {
		return "", err
	}

	path := fileUrl.Path
	segments := strings.Split(path, "/")

	fileName := segments[len(segments)-1]
	return fileName, nil
}

func getTopPosts(client *reddit.Client, ctx context.Context, subredditName string, postLimit int, timeframe string) ([]*reddit.Post, error) {
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

func grab(configPath string) error {
	configPath, err := homedir.Expand(configPath)
	if err != nil {
		return err
	}

	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}

	config := Config{}
	err = yaml.UnmarshalStrict(configBytes, &config)
	if err != nil {
		return err
	}

	client, err := reddit.NewReadonlyClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	for _, subreddit := range config.Subreddits {

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

		posts, err := getTopPosts(client, ctx, subreddit.Name, subreddit.Limit, subreddit.Timeframe)
		if err != nil {
			log.Printf("getTopPosts: %v: %v\n", subreddit, err)
		}

		for _, post := range posts {
			if strings.HasSuffix(post.URL, ".jpg") {

				urlFileName, err := fileNameFromUrl(post.URL)
				if err != nil {
					log.Printf("fileNameFromUrl: %v: %v: %v\n", subreddit.Name, post.URL, err)
				}
				fileName := strings.Replace(post.Title, " ", "_", -1) + "_" + urlFileName
				fileName = filepath.Join(subreddit.Destination, fileName)

				err = downloadFile(post.URL, fileName, false)
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
		editor = "vim"
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

func editScheduleFile(scheduleFileType string, rm bool) error {
	defaultContent := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <!-- Not sure these are necessary -->
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/sbin</string>
	</dict>
	<key>Label</key>
	<string>com.bbkane.grabbit</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/local/bin/grabbit</string>
		<string>grab</string>
	</array>
	<key>RunAtLoad</key>
	<false/>
	<key>StartCalendarInterval</key>
    <!-- Every Monday at 9AM -->
	<array>
		<dict>
			<key>Hour</key>
			<integer>9</integer>
			<key>Minute</key>
			<integer>0</integer>
			<key>Weekday</key>
			<integer>1</integer>
		</dict>
	</array>
</dict>
</plist>
`)
	var filePath string
	switch scheduleFileType {
	case "launchctl":
		filePath = "~/Library/LaunchAgents/com.bbkane.grabbit.plist"
	default:
		return fmt.Errorf("scheduleFileType not supported: %v\n", scheduleFileType)
	}
	return editFile(filePath, defaultContent, rm)
}

func run() error {

	defaultConfigPath := "~/.config/grabbit.yaml"

	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')

	editConfigCmd := app.Command("edit-config", "Edit or create configuration file. Uses $EDITOR or vim")
	editConfigCmdConfigPathFlag := editConfigCmd.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	grabCmd := app.Command("grab", "Grab images. Use `edit-config` first to create a config")
	grabCmdConfigPathFlag := grabCmd.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	editScheduleFileCmd := app.Command("edit-schedule-file", "Edit, create, or delete the platform specific way to run this app on a schedule")
	editScheduleFileCmdTypeFlag := editScheduleFileCmd.Flag("type", "type of schedule file").Required().Enum("cron", "launchctl", "systemd")
	editScheduleFileCmdRmFlag := editScheduleFileCmd.Flag("rm", "delete the schedule file").Bool()

	// TODO: write cronjob/systemctl commands

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))
	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(*editConfigCmdConfigPathFlag)
	case grabCmd.FullCommand():
		return grab(*grabCmdConfigPathFlag)
	case editScheduleFileCmd.FullCommand():
		return editScheduleFile(*editScheduleFileCmdTypeFlag, *editScheduleFileCmdRmFlag)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
