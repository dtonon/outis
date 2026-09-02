# outis

Generates and sends a fake "user unknown" bounce for an email you received,
so the sender believes your address does not exist.

The bounce is an RFC 3464 delivery status notification modelled on Postfix:
`multipart/report` with a human-readable part, a `message/delivery-status`
part with status `5.1.1`, and the original message attached as
`message/rfc822`. It is sent to the original `Return-Path` from
`MAILER-DAEMON@<your mail host>` with a null envelope sender when the SMTP
server allows it.

## Requirements

The bounce is only credible if it comes from a domain you control. Use an
SMTP account on that domain that lets you send as `MAILER-DAEMON@...`.
Consumer providers (Gmail, iCloud, ...) rewrite the From header and will
expose you.

## Usage

```
go build -o outis cmd/outis/main.go
./outis init [domain]         # add or update an account; password goes to the OS keychain
./outis accounts              # list accounts
./outis message.eml           # preview, then confirm
./outis -c                    # read the email from the clipboard
./outis -n message.eml        # dry run, print only
./outis -y -r me@example.com message.eml
./outis -a example.com -c     # force an account instead of matching recipients
./outis inbox/                # every file in the directory, one confirmation for the batch
./outis -n -o out/ inbox/     # dry run, write each bounce to out/<name>.bounce.eml
```

## Batch mode

Arguments can be files and directories, mixed. A directory expands to its
visible regular files, any extension, not recursive. Each file is matched to
an account on its own; files that cannot be parsed or matched are reported
and skipped while the others proceed, and the exit code is non-zero if any
failed. With more than one input a summary line per file is shown instead of
the full preview, followed by a single confirmation.

## Multiple accounts

Each account covers one domain. The bounce is built with the account whose
domain matches a recipient (Delivered-To, To or Cc) of the original email,
so the sender, mail host and SMTP server all belong to that domain. If no
account matches, or more than one does, use `--account`.

```toml
[[accounts]]
domain = "example.com"
mta_host = "mail.example.com"
[accounts.smtp]
host = "smtp.example.com"
port = 587
username = "mailer-daemon@example.com"

[[accounts]]
domain = "other.org"
mta_host = "mx.other.org"
[accounts.smtp]
host = "smtp.other.org"
port = 465
username = "mailer-daemon@other.org"
```

## Providers with shared suppression lists

Some providers, notably Amazon SES, keep a suppression list shared across all
their customers: one hard bounce makes every SES sender unable to reach your
address for a while. For Return-Path domains listed in `reply_to_from_domains`
the bounce is sent to the From header address instead. The default is
`["amazonses.com"]` and subdomains match. Set it to `[]` to disable.

```toml
reply_to_from_domains = ["amazonses.com"]
```

`envelope_from` on an account forces the SMTP envelope sender instead of
trying the null sender, `MAILER-DAEMON@domain` and the username in turn.

Config lives in `os.UserConfigDir()/outis/config.toml`
(`~/Library/Application Support/outis` on macOS, `~/.config/outis` on Linux).
Override with `OUTIS_CONFIG`.
