// Package sender delivers a message over authenticated SMTP submission.
package sender

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
)

type Account struct {
	Host     string
	Port     int
	Username string
	Password string
}

// Send submits msg to rcpt, trying each envelope sender in order until the
// server accepts one. An empty string means the null sender a real MTA uses.
// It returns the envelope sender that was accepted.
func Send(a Account, envelopes []string, rcpt string, msg []byte) (string, error) {
	c, err := dial(a)
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Auth(auth(a)); err != nil {
		return "", fmt.Errorf("auth: %w", err)
	}

	var used string
	var lastErr error
	for i, env := range envelopes {
		if i > 0 {
			if err := c.Reset(); err != nil {
				return "", fmt.Errorf("reset: %w", err)
			}
		}
		if lastErr = c.Mail(env); lastErr == nil {
			used = env
			break
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("mail from: %w", lastErr)
	}
	if err := c.Rcpt(rcpt); err != nil {
		return used, fmt.Errorf("rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return used, fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return used, err
	}
	if err := w.Close(); err != nil {
		return used, fmt.Errorf("send: %w", err)
	}
	return used, c.Quit()
}

func dial(a Account) (*smtp.Client, error) {
	addr := net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
	tlsCfg := &tls.Config{ServerName: a.Host}

	// Port 465 is implicit TLS, everything else uses STARTTLS
	if a.Port == 465 {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, a.Host)
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		c.Close()
		return nil, errors.New("server does not offer STARTTLS")
	}
	if err := c.StartTLS(tlsCfg); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func auth(a Account) smtp.Auth {
	return &loginOrPlain{
		plain: smtp.PlainAuth("", a.Username, a.Password, a.Host),
		user:  a.Username,
		pass:  a.Password,
	}
}

// loginOrPlain prefers PLAIN and falls back to LOGIN, which the stdlib lacks.
type loginOrPlain struct {
	plain smtp.Auth
	user  string
	pass  string
	login bool
}

func (l *loginOrPlain) Start(s *smtp.ServerInfo) (string, []byte, error) {
	for _, m := range s.Auth {
		if m == "PLAIN" {
			return l.plain.Start(s)
		}
	}
	l.login = true
	return "LOGIN", nil, nil
}

func (l *loginOrPlain) Next(fromServer []byte, more bool) ([]byte, error) {
	if !l.login {
		return l.plain.Next(fromServer, more)
	}
	if !more {
		return nil, nil
	}
	switch string(fromServer) {
	case "Username:":
		return []byte(l.user), nil
	case "Password:":
		return []byte(l.pass), nil
	}
	return nil, fmt.Errorf("unexpected LOGIN challenge %q", fromServer)
}
