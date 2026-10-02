// Outis generates and sends fake "user unknown" bounces for received emails.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/atotto/clipboard"
	"golang.org/x/term"

	"github.com/dtonon/outis/internal/bounce"
	"github.com/dtonon/outis/internal/config"
	"github.com/dtonon/outis/internal/secret"
	"github.com/dtonon/outis/internal/sender"
	"github.com/dtonon/outis/internal/version"
)

const usage = `Usage:
  outis [flags] [file|dir ...]   build bounces for emails (files, directories, stdin or clipboard)
  outis init [domain]    add or update an account interactively
  outis accounts         list configured accounts
  outis config           print the config file path
  outis version          print the version

Flags:
  -a, --account DOMAIN report as the account for DOMAIN instead of matching recipients
  -c, --clipboard      read the email from the clipboard
  -r, --recipient      address to report as unknown (default: first at your domain)
  -n, --dry-run        print the bounce, do not send
  -o, --out PATH       also write the bounce to PATH (a directory when processing several files)
  -y, --yes            send without confirmation
  -d, --delete         delete each file after its bounce is sent
`

func main() {
	var err error
	switch first(os.Args[1:]) {
	case "init":
		err = runInit(first(os.Args[2:]))
	case "accounts":
		err = runAccounts()
	case "version", "-v", "--version":
		fmt.Println("outis", version.String())
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

type job struct {
	name string
	acc  *config.Account
	res  *bounce.Result
	err  error
}

func runBounce(args []string) error {
	fs := flag.NewFlagSet("bounce", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var (
		fromClip, dryRun, yes, del bool
		recipient, out, account    string
	)
	fs.BoolVar(&del, "d", false, "")
	fs.BoolVar(&del, "delete", false, "")
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
	var forced *config.Account
	if account != "" {
		if forced = cfg.Find(account); forced == nil {
			return fmt.Errorf("no account for %s, configured: %s", account, strings.Join(cfg.Domains(), ", "))
		}
	}

	inputs, err := collectInputs(fromClip, fs.Args())
	if err != nil {
		return err
	}
	batch := len(inputs) > 1

	var jobs []job
	for _, in := range inputs {
		j := job{name: in}
		j.acc, j.res, j.err = prepare(cfg, forced, in, recipient)
		if j.err != nil && !batch {
			return j.err
		}
		jobs = append(jobs, j)
	}

	if out != "" {
		if err := writeOut(out, jobs, batch); err != nil {
			return err
		}
	}

	if dryRun || (!yes && !batch) {
		for _, j := range jobs {
			if j.err != nil {
				continue
			}
			if batch {
				fmt.Printf("==> %s\n", j.name)
			}
			os.Stdout.Write(j.res.Message)
			fmt.Println()
		}
	}
	if dryRun {
		return failures(jobs)
	}

	pending := 0
	for _, j := range jobs {
		if j.err != nil {
			fmt.Fprintf(os.Stderr, "%s\n-> skipped: %v\n\n", j.name, j.err)
			continue
		}
		pending++
		note := ""
		if j.res.Redirected {
			note = " (From header, Return-Path is in reply_to_from_domains)"
		}
		if batch {
			fmt.Fprintf(os.Stderr, "%s %s\n-> %s %s%s\n\n", j.name, j.res.Recipient, j.res.To, j.acc.Domain, note)
		} else {
			fmt.Fprintf(os.Stderr, "Bounce %s as unknown, sending to %s%s\n", j.res.Recipient, j.res.To, note)
		}
	}
	if pending == 0 {
		return failures(jobs)
	}
	if !yes {
		if batch {
			fmt.Fprintf(os.Stderr, "Send %d bounces? [y/N] ", pending)
		} else {
			fmt.Fprint(os.Stderr, "Send? [y/N] ")
		}
		if !confirm() {
			return fmt.Errorf("aborted")
		}
	}

	for i := range jobs {
		j := &jobs[i]
		if j.err != nil {
			continue
		}
		if j.err = send(j.acc, j.res); j.err != nil {
			fmt.Fprintf(os.Stderr, "%-24s failed: %v\n", j.name, j.err)
			continue
		}
		status := "sent"
		if del && j.name != inputStdin && j.name != inputClipboard {
			if err := os.Remove(j.name); err != nil {
				status = "sent, delete failed: " + err.Error()
			} else {
				status = "sent, deleted"
			}
		}
		if batch {
			fmt.Fprintf(os.Stderr, "%s %s\n", j.name, status)
		} else {
			fmt.Fprintln(os.Stderr, strings.ToUpper(status[:1])+status[1:])
		}
	}
	return failures(jobs)
}

// prepare parses one input and builds its bounce with the matching account.
func prepare(cfg *config.Config, forced *config.Account, name, recipient string) (*config.Account, *bounce.Result, error) {
	src, err := openInput(name)
	if err != nil {
		return nil, nil, err
	}
	defer src.Close()
	orig, err := bounce.Parse(src)
	if err != nil {
		return nil, nil, err
	}
	acc := forced
	if acc == nil {
		if acc, err = cfg.Match(orig.Recipients); err != nil {
			return nil, nil, err
		}
	}
	opt := bounce.Options{
		Domain:    acc.Domain,
		MTAHost:   acc.MTAHost,
		Recipient: recipient,
	}
	if cfg.ReplyToFrom(orig.ReturnPath) && orig.From != "" {
		opt.SendTo = orig.From
	}
	res, err := bounce.Build(orig, opt)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Find(domainOf(res.To)) != nil {
		return nil, nil, fmt.Errorf("destination %s is at your own domain, sender probably forged, nothing to bounce to", res.To)
	}
	return acc, res, nil
}

func send(acc *config.Account, res *bounce.Result) error {
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
	return nil
}

// Special input names for the clipboard and stdin
const (
	inputClipboard = "<clipboard>"
	inputStdin     = "<stdin>"
)

// collectInputs turns the arguments into a flat list of inputs, expanding
// directories to their visible regular files.
func collectInputs(fromClip bool, args []string) ([]string, error) {
	if fromClip {
		if len(args) > 0 {
			return nil, fmt.Errorf("--clipboard cannot be combined with files")
		}
		return []string{inputClipboard}, nil
	}
	if len(args) == 0 {
		if term.IsTerminal(int(syscall.Stdin)) {
			return nil, fmt.Errorf("no input: pass files or a directory, pipe the email, or use --clipboard")
		}
		return []string{inputStdin}, nil
	}
	var out []string
	for _, a := range args {
		if a == "-" {
			out = append(out, inputStdin)
			continue
		}
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			out = append(out, a)
			continue
		}
		entries, err := os.ReadDir(a)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
				out = append(out, filepath.Join(a, e.Name()))
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no files found")
	}
	return out, nil
}

func openInput(name string) (io.ReadCloser, error) {
	switch name {
	case inputClipboard:
		s, err := clipboard.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("clipboard: %w", err)
		}
		return io.NopCloser(strings.NewReader(s)), nil
	case inputStdin:
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(name)
}

// writeOut saves the bounces: to a single file, or into a directory when
// processing a batch.
func writeOut(out string, jobs []job, batch bool) error {
	if !batch {
		for _, j := range jobs {
			if j.err == nil {
				return os.WriteFile(out, j.res.Message, 0o600)
			}
		}
		return nil
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		return err
	}
	for _, j := range jobs {
		if j.err != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(j.name), filepath.Ext(j.name))
		if err := os.WriteFile(filepath.Join(out, base+".bounce.eml"), j.res.Message, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func failures(jobs []job) error {
	n := 0
	for _, j := range jobs {
		if j.err != nil {
			n++
		}
	}
	if n > 0 {
		return fmt.Errorf("%d of %d inputs failed", n, len(jobs))
	}
	return nil
}

func confirm() bool {
	s, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return ""
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
