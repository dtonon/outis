package bounce

import (
	"strings"
	"testing"
	"time"
)

const sample = `Return-Path: <bounce+123@lists.example.org>
Delivered-To: me@example.com
From: Newsletter <news@example.org>
To: Someone <me@example.com>, other@elsewhere.net
Subject: Hello
Message-ID: <abc123@example.org>
Date: Tue, 30 Sep 2026 08:00:00 +0200
Content-Type: text/plain

Body here.
`

func TestBuild(t *testing.T) {
	o, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if o.ReturnPath != "bounce+123@lists.example.org" {
		t.Fatalf("return path: %q", o.ReturnPath)
	}
	res, err := Build(o, Options{
		Domain:  "example.com",
		MTAHost: "mail.example.com",
		Now:     time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipient != "me@example.com" {
		t.Fatalf("recipient: %q", res.Recipient)
	}
	if res.To != "bounce+123@lists.example.org" {
		t.Fatalf("to: %q", res.To)
	}
	msg := string(res.Message)
	for _, want := range []string{
		"From: MAILER-DAEMON@example.com (Mail Delivery System)",
		"report-type=delivery-status",
		"In-Reply-To: <abc123@example.org>",
		"Status: 5.1.1",
		`<me@example.com>: unknown user: "me"`,
		"Content-Type: message/rfc822",
		"Subject: Hello",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(strings.ReplaceAll(msg, "\r\n", ""), "\n") {
		t.Error("bare LF found")
	}
}

func TestSendTo(t *testing.T) {
	o, _ := Parse(strings.NewReader(sample))
	if o.From != "news@example.org" {
		t.Fatalf("from: %q", o.From)
	}
	res, err := Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com", SendTo: o.From})
	if err != nil {
		t.Fatal(err)
	}
	if res.To != "news@example.org" || !res.Redirected {
		t.Fatalf("to: %q redirected: %v", res.To, res.Redirected)
	}
	if !strings.Contains(string(res.Message), "To: news@example.org\r\n") {
		t.Error("To header not redirected")
	}
	if !strings.Contains(string(res.Message), "X-Postfix-Sender: rfc822; bounce+123@lists.example.org") {
		t.Error("X-Postfix-Sender should keep the envelope sender")
	}
}

func TestAliasRecipient(t *testing.T) {
	src := `Return-Path: <news@example.org>
Delivered-To: real@example.com
X-Envelope-To: alias@example.com
From: news@example.org
To: alias@example.com
Subject: Hello

Body.
`
	o, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipient != "alias@example.com" {
		t.Fatalf("recipient: %q, the mailbox behind the alias must not leak", res.Recipient)
	}
	bcc := strings.Replace(src, "To: alias@example.com", "To: list@example.org", 1)
	o, _ = Parse(strings.NewReader(bcc))
	res, _ = Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com"})
	if res.Recipient != "alias@example.com" {
		t.Fatalf("bcc recipient: %q", res.Recipient)
	}
}

func TestNoRecipientAtDomain(t *testing.T) {
	o, _ := Parse(strings.NewReader(sample))
	_, err := Build(o, Options{Domain: "nope.com", MTAHost: "mail.nope.com"})
	if err == nil {
		t.Fatal("expected error")
	}
}
