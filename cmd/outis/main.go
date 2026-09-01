// Outis generates and sends fake "user unknown" bounces for received emails.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/atotto/clipboard"
	"golang.org/x/term"

	"github.com/dtonon/outis/internal/bounce"
	"github.com/dtonon/outis/internal/config"
	"github.com/dtonon/outis/internal/secret"
	"github.com/dtonon/outis/internal/sender"
)

const usage = `Usage:
  outis [flags] [file]   build a bounce for an email (file, stdin or clipboard)
  outis init [domain]    add or update an account interactively
  outis accounts         list configured accounts
  outis config           print the config file path

Flags:
  -a, --account DOMAIN report as the account for DOMAIN instead of matching recipients
  -c, --clipboard      read the email from the clipboard
  -r, --recipient      address to report as unknown (default: first at your domain)
  -n, --dry-run        print the bounce, do not send
  -o, --out FILE       also write the bounce to FILE
  -y, --yes            send without confirmation
`

func main() {
	var err error
	switch first(os.Args[1:]) {
	case "init":
		err = runInit(first(os.Args[2:]))
	case "accounts":
		err = runAccounts()
	case "config":
		var p string
		p, err = config.Path()
		fmt.Println(p)
	case "", "-h", "--help", "help":
		fmt.Print(usage)
	case "bounce":
		err = runBounce(os.Args[2:])
	default:
		err = runBounce(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runInit(domain string) error {
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.Config{}
	}
	in := bufio.NewReader(os.Stdin)
	ask := func(label, def string) string {
		if def != "" {
			fmt.Printf("%s [%s]: ", label, def)
		} else {
			fmt.Printf("%s: ", label)
		}
		s, _ := in.ReadString('\n')
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
		return def
	}

	if domain == "" {
		domain = ask("Email domain (e.g. example.com)", "")
	}
	a := config.Account{Domain: domain, SMTP: config.SMTP{Port: 587}}
	if cur := cfg.Find(domain); cur != nil {
		a = *cur
	}
	a.MTAHost = ask("Mail server hostname shown in the bounce", firstNonEmpty(a.MTAHost, "mail."+domain))
	a.SMTP.Host = ask("SMTP host", a.SMTP.Host)
	port, err := strconv.Atoi(ask("SMTP port", strconv.Itoa(a.SMTP.Port)))
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	a.SMTP.Port = port
	a.SMTP.Username = ask("SMTP username", a.SMTP.Username)

	fmt.Print("SMTP password (stored in the OS keychain, empty to keep current): ")
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if len(pw) > 0 {
		if err := secret.Set(a.SMTP.Username, string(pw)); err != nil {
			return fmt.Errorf("keychain: %w", err)
		}
	} else if _, err := secret.Get(a.SMTP.Username); err != nil {
		return fmt.Errorf("no password stored for %s", a.SMTP.Username)
	}
	cfg.Upsert(a)
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	fmt.Println("Saved", p)
	return nil
}

func runAccounts() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, a := range cfg.Accounts {
		fmt.Printf("%-24s %s via %s:%d as %s\n", a.Domain, a.MTAHost, a.SMTP.Host, a.SMTP.Port, a.SMTP.Username)
	}
	return nil
}

func runBounce(args []string) error {
	fs := flag.NewFlagSet("bounce", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var (
		fromClip, dryRun, yes   bool
		recipient, out, account string
	)
	fs.StringVar(&account, "a", "", "")
	fs.StringVar(&account, "account", "", "")
	fs.BoolVar(&fromClip, "c", false, "")
	fs.BoolVar(&fromClip, "clipboard", false, "")
	fs.BoolVar(&dryRun, "n", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	fs.BoolVar(&yes, "y", false, "")
	fs.BoolVar(&yes, "yes", false, "")
	fs.StringVar(&recipient, "r", "", "")
	fs.StringVar(&recipient, "recipient", "", "")
	fs.StringVar(&out, "o", "", "")
	fs.StringVar(&out, "out", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	src, err := readSource(fromClip, fs.Arg(0))
	if err != nil {
		return err
	}
	orig, err := bounce.Parse(src)
	if err != nil {
		return err
	}
	var acc *config.Account
	if account != "" {
		if acc = cfg.Find(account); acc == nil {
			return fmt.Errorf("no account for %s, configured: %s", account, strings.Join(cfg.Domains(), ", "))
		}
	} else if acc, err = cfg.Match(orig.Recipients); err != nil {
		return err
	}
	res, err := bounce.Build(orig, bounce.Options{
		Domain:    acc.Domain,
		MTAHost:   acc.MTAHost,
		Recipient: recipient,
	})
	if err != nil {
		return err
	}

	if out != "" {
		if err := os.WriteFile(out, res.Message, 0o600); err != nil {
			return err
		}
	}
	if dryRun || !yes {
		os.Stdout.Write(res.Message)
		fmt.Println()
	}
	if dryRun {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Bounce %s as unknown, sending to %s\n", res.Recipient, res.To)
	if !yes && !confirm() {
		return fmt.Errorf("aborted")
	}

	pw, err := secret.Get(acc.SMTP.Username)
	if err != nil {
		return fmt.Errorf("keychain: %w (run `outis init %s`)", err, acc.Domain)
	}
	envelopes := []string{"", res.From, acc.SMTP.Username}
	if acc.EnvelopeFrom != "" {
		envelopes = []string{acc.EnvelopeFrom}
	}
	env, err := sender.Send(sender.Account{
		Host:     acc.SMTP.Host,
		Port:     acc.SMTP.Port,
		Username: acc.SMTP.Username,
		Password: pw,
	}, envelopes, res.To, res.Message)
	if err != nil {
		return err
	}
	switch env {
	case "":
	case res.From:
		fmt.Fprintf(os.Stderr, "Note: server refused a null sender, used envelope from %s\n", env)
	default:
		fmt.Fprintf(os.Stderr, "Warning: server only accepted %s as envelope sender, it will appear as Return-Path in the raw headers\n", env)
	}
	fmt.Fprintln(os.Stderr, "Sent")
	return nil
}

func readSource(fromClip bool, path string) (io.Reader, error) {
	switch {
	case fromClip:
		s, err := clipboard.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("clipboard: %w", err)
		}
		return strings.NewReader(s), nil
	case path != "" && path != "-":
		return os.Open(path)
	default:
		if term.IsTerminal(int(syscall.Stdin)) {
			return nil, fmt.Errorf("no input: pass a file, pipe the email, or use --clipboard")
		}
		return os.Stdin, nil
	}
}

func confirm() bool {
	fmt.Fprint(os.Stderr, "Send? [y/N] ")
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
