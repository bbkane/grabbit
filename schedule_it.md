# Run grabbit on a schedule

## MacOS Homebrew

```
brew services start grabbit
```

This runs when started and every Monday at 10AM.

See `brew services --help` for more info

## Mac (launchd directly)

Copy the `plist` stuff from [./.goreleaser.yml](./.goreleaser.yml) to a file named `~/Library/LaunchAgents/com.bbkane.grabbit.plist` and change the following:

- `#{plist_name}` -> `com.bbkane.grabbit`
- `>#{opt_bin}/grabbit` -> full path to grabbit binary

Turn it on with:

```
launchctl load -w ~/Library/LaunchAgents/com.bbkane.grabbit.plist
```

See the following links to read up on `launchd` or generate `plist` files:

- https://www.launchd.info/
- https://github.com/zerowidth/launched
- https://www.peterborgapps.com/lingon/ (the app costs $15)

This runs when started and every Monday at 10AM

## Linux with systemd

See the following links to read up on `systemd` or generate timers

- https://techoverflow.net/2019/04/22/simple-systemd-timer-generator/
- https://wiki.archlinux.org/index.php/Systemd/Timers

## Mac or Linux with crond

See the following links to read up `crond` or generate entries:

- https://crontab-generator.org/
- https://www.hostinger.com/tutorials/cron-job
