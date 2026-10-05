# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Forwarded emails (SRS, Gmail `+caf_=`, or an earlier hop recording a different envelope sender) are bounced to the original sender, reporting the address they used instead of the forward target
- `--to` sends the bounce to a given address instead of the envelope sender

### Changed

- In monitor mode Send is the default button of the dialog: Enter sends, Escape skips

## [0.2.1] - 2026-10-05

### Changed

- Originals over 50 KB are returned as headers only, like Postfix with its default `bounce_size_limit`

### Fixed

- Report the address the sender used instead of the mailbox behind an alias, which Delivered-To exposed

## [0.2.0] - 2026-10-05

### Added

- `monitor` command that watches the clipboard and confirms each bounce through a native dialog

## [0.1.1] - 2026-10-02

### Added

- Skip messages whose bounce destination is at one of your own domains, typically a forged sender

## [0.1.0] - 2026-09-30

### Added

- Fake "user unknown" bounce built as an RFC 3464 DSN modelled on Postfix
- Input from file, stdin or clipboard, with dry run and preview
- Multiple accounts matched by recipient domain
- Batch processing of files and directories with a single confirmation
- `reply_to_from_domains` to avoid providers with shared suppression lists
- `--delete` to remove files after their bounce is sent
- SMTP password stored in the OS keychain
