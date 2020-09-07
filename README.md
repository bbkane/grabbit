# Grabbit

Grab subreddit images!! Heavily inspired by https://github.com/nagracks/reddit_get_top_images

# Install

TODO once Homebrew tap is set up

# Notes

## Tagging (tags that start with v trigger a release)

```
git tag -a v0.1 -m 'does this build'
git push origin v0.1
```

# TODO

- github actions + homebrew
  - https://medium.com/better-programming/indie-mac-app-devops-with-github-actions-b16764a3ebe7
  - https://flowerinthenight.com/blog/2019/07/30/homebrew-golang
  - https://github.com/goreleaser/goreleaser - do this first!
  - https://github.com/mitchellh/gon#usage-with-goreleaser
- user agent change (reddit API best practices)
- better image parsing (go/colly - see nagracks)
- `write-schedule --path [defaults to ...] --format [cron|systemctl|mac thingie]`
  - Lingon (in Downloads)
- rate limiting ( fetchbot? )
- better logging

