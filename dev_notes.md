# Dev Notes

## Run from source

```
go run . -c ./test-config.yml --help
```

## Build locally with [goreleaser](https://goreleaser.com/)

This is useful for testing [./.goreleaser.yml](./.goreleaser.yml).

```
goreleaser --snapshot --skip-publish --rm-dist
```

## Tagging (tags that start with v trigger a release when pushed)

```
./tag_it.sh v0.1 -m 'does this build'
```

# TODO

- better image parsing (go/colly - see nagracks)

