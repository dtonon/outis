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

func TestNoRecipientAtDomain(t *testing.T) {
	o, _ := Parse(strings.NewReader(sample))
	_, err := Build(o, Options{Domain: "nope.com", MTAHost: "mail.nope.com"})
	if err == nil {
		t.Fatal("expected error")
	}
}
