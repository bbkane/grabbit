# Changelog

All notable changes to this project will be documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

Note the the latest version is usually work in progress and may have not yet been released.

# v5.0.0

## Changed

Changed both the flags and the config file to make grabbit less verbose.

Old CLI invocation:

```bash
grabbit grab \
    --subreddit-destination . \
    --subreddit-name wallpapers \
    --subreddit-timeframe day \
    --subreddit-limit 5 \
    --subreddit-destination . \
    --subreddit-name earthporn \
    --subreddit-timeframe week \
    --subreddit-limit 10
```

New CLI invocation:

```bash
# --destination is now shared between all subreddits
grabbit grab \
    --destination . \
    --subreddit-info wallpapers,day,5 \
    --subreddit-info earthporn,week,10
```

Old config format:

```yaml
lumberjacklogger:
  filename: ~/.config/grabbit.jsonl
  maxage: 30 # days
  maxbackups: 0
  maxsize: 5 # megabytes
subreddits:
  - destination: .
    limit: 5
    name: wallpapers
    timeframe: day
  - destination: .
    limit: 10
    name: earthporn
    timeframe: week
```

New config format:

```yaml
version: 5 # version key required
lumberjacklogger:
  filename: ~/.config/grabbit.jsonl
  maxage: 30 # days
  maxbackups: 0
  maxsize: 5 # megabytes
destination: .
subreddits:
  - limit: 5
    name: wallpapers
    timeframe: day
  - limit: 10
    name: earthporn
    timeframe: week
```

I'm requiring the version key so I can check for it and direct folks to the CHANGELOG.md notes if it can't be found.

# v4.4.22

## Changed

Updated warg, got tab completion!
