# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-10-05

### Added

- `monitor` command that watches the clipboard and confirms each bounce through a native dialog

### Fixed

- Report the address the sender used instead of the mailbox behind an alias, which Delivered-To exposed

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
