# Dev Notes

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
  - https://github.com/koekeishiya/homebrew-formulae/blob/master/yabai.rb
- user agent change (reddit API best practices)
- better image parsing (go/colly - see nagracks)
- rate limiting ( fetchbot? )
