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
	"strings"

	"github.com/mitchellh/go-homedir"
	"github.com/vartanbeno/go-reddit/reddit"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func downloadFile(URL string, fileName string) error {
	// https://golangbyexample.com/download-image-file-url-golang/
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
	}
	defer response.Body.Close()

	// TODO: don't overwrite files

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

func getTopPosts(client *reddit.Client, ctx context.Context, subredditName string, postLimit int, timeFrame string) ([]*reddit.Post, error) {
	posts, _, err := client.Subreddit.TopPosts(ctx, "earthporn", &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: 5,
		},
		Time: "all",
	})
	if err != nil {
		return nil, err
	}
	return posts, nil
}

func grab(configPath string) error {
	client, err := reddit.NewReadonlyClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	posts, err := getTopPosts(client, ctx, "earthporn", 5, "day")
	if err != nil {
		return err
	}

	for _, post := range posts {
		fmt.Println(post.URL)
		if strings.HasSuffix(post.URL, ".jpg") {

			fileName, err := fileNameFromUrl(post.URL)
			if err != nil {
				log.Printf("fileNameFromUrl: %v: %v", post.URL, err)
			}

			err = downloadFile(post.URL, fileName)
			if err != nil {
				log.Printf("downloadFile: %v: %v", post.URL, err)
			}
		}
	}
	return nil
}

func editConfig(configPath string) error {

	configPath, err := homedir.Expand(configPath)
	if err != nil {
		return err
	}

	// TODO: clean up control flow
	if _, err = os.Stat(configPath); err == nil {
		// exists, we're good
	} else if os.IsNotExist(err) {
		err = ioutil.WriteFile(configPath, []byte("test"), 0644)
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

func run() error {

	defaultConfigPath := "~/.config/grabbit.yaml"

	app := kingpin.New("grabbit", "Get top images from subreddits").UsageTemplate(kingpin.DefaultUsageTemplate)
	app.HelpFlag.Short('h')

	editConfigCmd := app.Command("edit-config", "edit configuration file for grabbit")
	editConfigCmdConfigPathFlag := editConfigCmd.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	grabCmd := app.Command("grab", "Grab images")
	grabCmdConfigPathFlag := grabCmd.Flag("config-path", "config filepath").Short('p').Default(defaultConfigPath).String()

	// TODO: write cronjob/systemctl commands

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))
	switch cmd {
	case editConfigCmd.FullCommand():
		return editConfig(*editConfigCmdConfigPathFlag)
	case grabCmd.FullCommand():
		return grab(*grabCmdConfigPathFlag)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
