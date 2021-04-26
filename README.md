# Grabbit

Grab subreddit images!! Very useful for getting nice wallpapers automatically - grabbit automatically skips images tagged NSFW. Designed to be easy to install/uninstall.

## @bbkane's Setup

See my [personal config](https://github.com/bbkane/dotfiles/blob/master/grabbit/.config/grabbit.yaml).

![My Setup](./reddit_wallpapers.jpg)

## Install

- Homebrew: `brew install bbkane/tap/grabbit`
- Download Mac/Linux/Windows executable: [GitHub releases](https://github.com/bbkane/grabbit/releases)
- Build with [goreleaser](https://goreleaser.com/): `goreleaser --snapshot --skip-publish --rm-dist`
- Build with `go`: `go build .`

## Use

```bash
grabbit config edit --editor /path/to/editor
grabbit grab
```

## Docs

- Run grabbit on a schedule: [./schedule_it.md](./schedule_it.md)
- Hack on grabbit: [./dev_notes.md](./dev_notes.md)
- [MIT Licensed](./LICENSE)

## 3rd Party Credits

- library authors: see [./go.mod](./go.mod)
- inspiration: [reddit_get_top_images](https://github.com/nagracks/reddit_get_top_images)
