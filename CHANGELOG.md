# Changelog

## v1.1.0

- Add error-returning counterparts to the `Blade` methods, so saw can be
  embedded as a library without a failed API call killing the host process:
  `LogGroups`, `LogStreams`, `Events` and `Stream`. The existing
  `GetLogGroups`, `GetLogStreams`, `GetEvents` and `StreamEvents` are now thin
  wrappers over them and keep the CLI behavior of printing the error and
  exiting 2.
- `Events` and `Stream` hand events to a callback rather than printing them,
  and take a `context.Context`. `Stream` returns on cancellation instead of
  looping forever.
- Fix two panics in the streaming path on events with a nil `EventId` or
  `Timestamp`.
- Bump GitHub Actions to the Node 24 releases.

## v1.0.0

- Upgrade from aws-sdk-go v1 to aws-sdk-go-v2. v1 reached end-of-support in
  July 2025.
- Drop the deprecated `interleaved` parameter on FilterLogEvents. AWS has
  ignored it and assumed true since June 17, 2019.
- Fix a nil dereference when sorting streams: a log stream that has never
  received an event has no LastEventTimestamp and used to panic the comparator.
  Such streams now sort last.
- Remove AUR packaging, which was pinned to the 2019 dep-era release.
- Add `blade.NewBladeWithConfig` and `blade.NewBladeWithClient` so saw can be
  embedded as a library without reaching into unexported fields. The `cwl`
  field is now a `blade.CloudWatchLogsClient` interface, so a fake can be
  injected in tests.
- Minimum Go version is now 1.24.

## v0.2.2

 - Added support for parsing additonal time formats (@andrewpage)

## v0.2.1

- Feature - de-duplicate newlines event messages (@klichukb)
- Feature - Added Dockerfile (@shnhrrsn)

## v0.2.0

- Added --raw flag to watch subcommand, disables decorations (@will-ockmore)
- Added --pretty flag to get subcommand, enables decorations (@will-ockmore)

The defaults are for the watch output to be pretty and the get output to be raw.

## v0.1.8

- Support filter option for get (@cynipe)

## v0.1.7

- Fix usage output for get command
- Rename get command `end` flag to `stop`
- Unexport some exported vars in `cmd` package

## v0.1.6

- Add MFA (assumerole) support (@perriea)
- Add usage for get command (@will-ockmore)

## v0.1.5

- Add region and profile support
