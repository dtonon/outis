package bounce

import (
	"strings"
	"testing"
)

const gmailForward = `Return-Path: <someone+caf_=alias=example.com@gmail.com>
Delivered-To: real@example.com
Received: from mail-yw1-x1134.google.com (mail-yw1-x1134.google.com [IPv6:2607:f8b0:4864:20::1134])
    by mx.example.com (Postfix) with ESMTPS id 3CF28109A42
    for <alias@example.com>; Thu, 19 Feb 2026 19:03:02 +0100 (CET)
X-Envelope-To: alias@example.com
Authentication-Results: mx.example.com;
    spf=pass smtp.mailfrom="someone+caf_=alias=example.com@gmail.com"
ARC-Authentication-Results: i=3; mx.google.com;
       spf=pass (google.com: domain of spammer@sender.test designates 209.85.220.41 as permitted sender) smtp.mailfrom=spammer@sender.test
X-Forwarded-To: alias@example.com
X-Forwarded-For: someone@gmail.com alias@example.com
Delivered-To: someone@gmail.com
ARC-Authentication-Results: i=2; mx.google.com;
       spf=pass (google.com: domain of spammer@sender.test designates 209.85.220.41 as permitted sender) smtp.mailfrom=spammer@sender.test
Received: from mail-sor-f41.google.com (mail-sor-f41.google.com. [209.85.220.41])
        by mx.google.com with SMTPS id 38308e7fff4ca
        for <someone@gmail.com>; Thu, 19 Feb 2026 10:02:44 -0800 (PST)
ARC-Authentication-Results: i=1; mx.google.com; arc=none
From: Spammer <spammer@sender.test>
To: undisclosed-recipients:;
Subject: Complete with DocuSign
Message-ID: <abc@sender.test>

Body.
`

func TestForwardedByGmail(t *testing.T) {
	o, err := Parse(strings.NewReader(gmailForward))
	if err != nil {
		t.Fatal(err)
	}
	if o.Forward == nil {
		t.Fatal("forward not detected")
	}
	if o.Forward.By != "gmail.com" || o.Forward.Sender != "spammer@sender.test" || o.Forward.Recipient != "someone@gmail.com" {
		t.Fatalf("forward: %+v", o.Forward)
	}
	res, err := Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.To != "spammer@sender.test" || res.Redirected {
		t.Fatalf("to: %q redirected: %v", res.To, res.Redirected)
	}
	if res.Recipient != "someone@gmail.com" {
		t.Fatalf("recipient: %q", res.Recipient)
	}
	msg := string(res.Message)
	for _, want := range []string{
		"From: MAILER-DAEMON@example.com (Mail Delivery System)",
		"X-Postfix-Sender: rfc822; spammer@sender.test",
		"Final-Recipient: rfc822; someone@gmail.com",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q", want)
		}
	}
	res, _ = Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com", SendTo: "other@sender.test"})
	if res.To != "other@sender.test" || !res.Redirected {
		t.Fatalf("SendTo: %q redirected: %v", res.To, res.Redirected)
	}
}

func TestForwardedBySRS(t *testing.T) {
	src := `Return-Path: <SRS0=k2Pj=4M=sender.test=spammer@forwarder.org>
Delivered-To: me@example.com
Received: from mx.forwarder.org by mx.example.com (Postfix) with ESMTPS id 1
    for <me@example.com>; Thu, 19 Feb 2026 19:03:02 +0100 (CET)
Received: from out.sender.test by mx.forwarder.org (Postfix) with ESMTPS id 2
    for <old@forwarder.org>; Thu, 19 Feb 2026 19:03:00 +0100 (CET)
From: spammer@sender.test
To: old@forwarder.org
Subject: Hello

Body.
`
	o, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if o.Forward == nil || o.Forward.Sender != "spammer@sender.test" || o.Forward.By != "forwarder.org" {
		t.Fatalf("forward: %+v", o.Forward)
	}
	if o.Forward.Recipient != "me@example.com" {
		t.Fatalf("recipient: %q, only one Delivered-To so it is the best guess", o.Forward.Recipient)
	}
	o.Header["Delivered-To"] = nil
	if f := detectForward(o.Header, o.ReturnPath); f.Recipient != "old@forwarder.org" {
		t.Fatalf("recipient from Received: %q", f.Recipient)
	}
}

func TestDecodeSRS(t *testing.T) {
	for in, want := range map[string]string{
		"SRS0=k2Pj=4M=sender.test=user+tag@fwd.org":             "user+tag@sender.test",
		"srs0+k2Pj=4M=sender.test=user@fwd.org":                 "user@sender.test",
		"SRS1=HHH=fwd.org==k2Pj=4M=sender.test=user@second.org": "user@sender.test",
		"SRS0=broken@fwd.org":                                   "",
		"plain@fwd.org":                                         "",
		"short@x":                                               "",
	} {
		if got := decodeSRS(in); got != want {
			t.Errorf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestMailingListIsNotForward(t *testing.T) {
	src := `Return-Path: <list-bounces+me=example.com@lists.example.org>
Delivered-To: me@example.com
ARC-Authentication-Results: i=1; lists.example.org; spf=pass smtp.mailfrom=poster@elsewhere.net
List-Id: <list.lists.example.org>
From: poster@elsewhere.net
To: list@lists.example.org
Subject: Hello

Body.
`
	o, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if o.Forward != nil {
		t.Fatalf("list treated as forward: %+v", o.Forward)
	}
	res, _ := Build(o, Options{Domain: "example.com", MTAHost: "mail.example.com"})
	if res.To != "list-bounces+me=example.com@lists.example.org" {
		t.Fatalf("to: %q", res.To)
	}
}

func TestNotForwardedWhenEnvelopeMatches(t *testing.T) {
	src := `Return-Path: <news@example.org>
Delivered-To: me@example.com
ARC-Authentication-Results: i=1; mx.example.com; spf=pass smtp.mailfrom=News@Example.org
From: news@example.org
To: me@example.com
Subject: Hello

Body.
`
	o, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if o.Forward != nil {
		t.Fatalf("unexpected forward: %+v", o.Forward)
	}
}
