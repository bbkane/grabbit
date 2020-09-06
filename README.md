# Grabbit

Grab subreddit images!! Heavily inspired by https://github.com/nagracks/reddit_get_top_images

# TODO

- github actions to build
  - first get github actions on tags
  - https://presstige.io/p/Using-GitHub-Actions-with-Go-2ca9744b531f4f21bdae9976d1ccbb58
  - https://github.com/actions/starter-workflows/blob/main/ci/go.yml
  - https://github.com/actions/cache/blob/main/examples.md#go---modules
  - https://docs.github.com/en/actions/reference/events-that-trigger-workflows
- homebrew to pick it up
  - https://flowerinthenight.com/blog/2019/07/30/homebrew-golang
  - need to create a repo for this
- user agent change (reddit API best practices)
- better image parsing (go/colly - see nagracks)
- `write-schedule --path [defaults to ...] --format [cron|systemctl|mac thingie]`
  - Lingon (in Downloads)
- rate limiting ( fetchbot? )
- better logging
