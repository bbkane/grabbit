package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/vartanbeno/go-reddit/reddit"
)


func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func downloadFile(URL, fileName string) error {
	// https://golangbyexample.com/download-image-file-url-golang/
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

func fileNameFromUrl(fullUrl string) (string,  error) {
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

func getTopPosts(client *reddit.Client, ctx context.Context, subredditName string, postLimit int, timeFrame string) ([]*reddit.Post, error){
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

func run() error {
	// Let's get the top 200 posts of r/golang.
	// Reddit returns a maximum of 100 posts at a time,
	// so we'll need to separate this into 2 requests.

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
