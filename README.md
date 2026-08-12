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
./outis init                  # domain, mail host, SMTP; password goes to the OS keychain
./outis bounce message.eml    # preview, then confirm
./outis bounce -c             # read the email from the clipboard
./outis bounce -n message.eml # dry run, print only
./outis bounce -y -r me@example.com message.eml
```

Config lives in `os.UserConfigDir()/outis/config.toml`
(`~/Library/Application Support/outis` on macOS, `~/.config/outis` on Linux).
Override with `OUTIS_CONFIG`.
