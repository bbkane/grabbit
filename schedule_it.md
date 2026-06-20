# Run grabbit on a schedule

## macOS (launchd)

Note: grabbit is now published as a Homebrew cask, so the old formula-based
`brew services start grabbit` flow is no longer used.

Find your grabbit binary path:

```
which grabbit
```

Create `~/Library/LaunchAgents/com.bbkane.grabbit.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
	<dict>
		<key>Label</key>
		<string>com.bbkane.grabbit</string>

		<key>ProgramArguments</key>
		<array>
			<string>/opt/homebrew/bin/grabbit</string>
			<string>grab</string>
		</array>

		<key>RunAtLoad</key>
		<true/>

		<key>StartCalendarInterval</key>
		<dict>
			<key>Weekday</key>
			<integer>1</integer>
			<key>Hour</key>
			<integer>10</integer>
			<key>Minute</key>
			<integer>0</integer>
		</dict>

		<key>StandardOutPath</key>
		<string>/tmp/com.bbkane.grabbit.stdout.log</string>
		<key>StandardErrorPath</key>
		<string>/tmp/com.bbkane.grabbit.stderr.log</string>
	</dict>
</plist>
```

Load and enable it:

```
launchctl unload ~/Library/LaunchAgents/com.bbkane.grabbit.plist
launchctl load -w ~/Library/LaunchAgents/com.bbkane.grabbit.plist
launchctl list | grep com.bbkane.grabbit
```

This runs at login and every Monday at 10:00.

See the following links to read up on `launchd` or generate `plist` files:

- https://www.launchd.info/
- https://github.com/zerowidth/launched
- https://www.peterborgapps.com/lingon/ (the app costs $15)

## Linux with systemd

See https://github.com/bbkane/dotfiles/tree/master/grabbit-systemd

## Mac or Linux with crond

See the following links to read up `crond` or generate entries:

- https://crontab-generator.org/
- https://www.hostinger.com/tutorials/cron-job

## Windows

I don't regularly use Windows so I can't really help here. If you have a nice
way to schedule grabbit on Windows, I'd appreciate a Pull Request updating this
section
