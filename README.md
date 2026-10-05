# Outis

Outis fights spam generating and sends a fake "user unknown" bounce for an email you received, so the sender believes your address does not exist. This should work well for the recent trend of AI-generated automated emails, where the sender may expect a reply; it helps to clean your email from their list.

Outis (Οὖτις) is Greek for "nobody". In the Odyssey, Odysseus gives it as his name to the Cyclops Polyphemus, so that when the blinded giant calls for help and shouts that "Nobody" is hurting him, the other Cyclopes leave. The tool does the same for your mailbox: it tells whoever is asking that there is nobody here.

![](assets/banner.jpg)

The bounce is an RFC 3464 delivery status notification modelled on Postfix: `multipart/report` with a human-readable part, a `message/delivery-status` part with status `5.1.1`, and the original message attached as `message/rfc822`, or only its headers as `text/rfc822-headers` when the original exceeds 50 KB, the Postfix `bounce_size_limit` default. It is sent to the original `Return-Path` from `MAILER-DAEMON@<your domain>` with a null envelope sender when the SMTP server allows it.

## Known limitations

Unlike a real 550 rejection during the SMTP conversation, this bounce is sent after the message was already accepted, so the sender's logs show a successful delivery, the mailbox keeps working for any retry, and the effect only reaches the one sender who receives and processes the notification.

## Requirements

The bounce is only credible if it comes from a domain you control. If possible use an SMTP account on that domain that lets you send as `MAILER-DAEMON@...`. Consumer providers (Gmail, iCloud, ...) rewrite the From header and will expose you.

## Usage

```
go build -o outis cmd/outis/main.go
./outis init [domain]         # add or update an account; password goes to the OS keychain
./outis accounts              # list accounts
./outis message.eml           # preview, then confirm
./outis -c                    # read the email from the clipboard
./outis -n message.eml        # dry run, print only
./outis -y -r me@example.com message.eml
./outis -t sender@example.org message.eml   # send the bounce to this address
./outis -a example.com -c     # force an account instead of matching recipients
./outis inbox/                # every file in the directory, one confirmation for the batch
./outis -n -o out/ inbox/     # dry run, write each bounce to out/<name>.bounce.eml
./outis -d inbox/             # delete each file once its bounce is sent
```

## Batch mode

Arguments can be any mix of files and directories. A directory expands to its visible regular files, any extension, not recursive. Each file is matched to an account on its own; files that cannot be parsed or matched are reported and skipped while the others proceed, and the exit code is non-zero if any failed. With more than one input a summary line per file is shown instead of the full preview, followed by a single confirmation.

## Monitor mode

`outis monitor` watches the clipboard and, whenever you copy an email that matches one of your accounts, opens a native dialog asking whether to send the bounce. Copy the raw source of the message ("Show original", "View source" or similar in your mail client), check the dialog and press Send. Anything that is not an email is ignored, emails that match no account are reported in the terminal and skipped, and the same clipboard content is never offered twice. The dialog defaults to Skip, so a stray Enter does not send anything. Stop with Ctrl-C.

```
./outis monitor               # watch the clipboard, confirm each bounce in a dialog
./outis monitor -n            # dry run, print the bounce instead of sending it
./outis monitor -a example.com
```

On macOS and Windows the dialog is built in; on Linux the `zenity` program must be installed.

## Multiple accounts

Each account covers one domain. The bounce is built with the account whose domain matches a recipient (Delivered-To, To or Cc) of the original email, so the sender, mail host and SMTP server all belong to that domain. If no account matches, or more than one does, use `--account`.

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

## Forwarded emails

When an email reached you through a forwarder, for example a Gmail account that forwards to your domain, the forwarder rewrote the envelope: the `Return-Path` points at the forwarder, so a bounce sent there would never reach the sender and could make the forwarder pause the rule. Outis recognises these messages from the rewritten `Return-Path` (SRS and Gmail's `+caf_=` scheme) or from an earlier hop recording a different envelope sender in `ARC-Authentication-Results`, `Received-SPF` or `Authentication-Results`. The bounce is then sent to that original sender and reports the address the sender used, the earliest `Delivered-To`, since that is the one on their list. The preview says `forwarded by <domain>` when this applies. Mailing lists are not forwarders: a message carrying `List-Id` or similar headers still bounces to its `Return-Path`.

The bounce comes from your domain even though it reports an address elsewhere, like a relay reporting a downstream failure. Use `--recipient` when the original address was not recorded, and `--to` to send the bounce to any other address. A destination at one of your own domains is refused in every case.

## Providers with shared suppression lists

Some providers, notably Amazon SES, keep a suppression list shared across all their customers: one hard bounce makes every SES sender unable to reach your address for a while. For Return-Path domains listed in `reply_to_from_domains` the bounce is sent to the From header address instead. The default is `["amazonses.com"]` and subdomains match too. Set it to `[]` to disable.

```toml
reply_to_from_domains = ["amazonses.com"]
```
### Configuration

`envelope_from` on an account forces the SMTP envelope sender instead of trying the null sender, `MAILER-DAEMON@domain` and the username in turn.

Config lives in `os.UserConfigDir()/outis/config.toml` (`~/Library/Application Support/outis` on macOS, `~/.config/outis` on Linux). Override with `OUTIS_CONFIG`.
