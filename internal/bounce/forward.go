package bounce

import (
	"net/mail"
	"regexp"
	"strconv"
	"strings"
)

// Forward describes a message that reached us through a forwarder, which
// rewrote the envelope so the Return-Path no longer belongs to the sender.
type Forward struct {
	// By is the domain of the forwarder.
	By string
	// Sender is the envelope sender before the forward.
	Sender string
	// Recipient is the address the sender delivered to, empty when unknown.
	Recipient string
}

var (
	arcIndex    = regexp.MustCompile(`^\s*i=(\d+)`)
	mailFrom    = regexp.MustCompile(`smtp\.mailfrom="?([^";\s]+)"?`)
	envFrom     = regexp.MustCompile(`envelope-from="?<?([^";\s>]+)>?"?`)
	receivedFor = regexp.MustCompile(`\bfor\s+<([^>]+)>`)
)

// detectForward recognises a forwarded message and recovers the original
// envelope. Mailing lists are not forwarders: they re-send on their own
// behalf and handle bounces at the rewritten Return-Path.
func detectForward(h mail.Header, returnPath string) *Forward {
	sender := decodeSRS(returnPath)
	rewritten := sender != "" || strings.Contains(strings.ToLower(returnPath), "+caf_=")
	if sender == "" {
		sender = earliestEnvelopeSender(h)
	}
	if sender == "" || strings.EqualFold(sender, returnPath) {
		return nil
	}
	if !rewritten && (h.Get("List-Id") != "" || h.Get("List-Post") != "" || h.Get("List-Unsubscribe") != "") {
		return nil
	}
	return &Forward{
		By:        domainOf(returnPath),
		Sender:    sender,
		Recipient: earliestRecipient(h),
	}
}

// earliestEnvelopeSender returns the envelope sender recorded by the first
// hop that authenticated the message, from the ARC chain or the SPF checks.
// Headers are prepended, so the last value in the message is the earliest.
func earliestEnvelopeSender(h mail.Header) string {
	best, bestIdx := "", -1
	for _, v := range h["Arc-Authentication-Results"] {
		m := arcIndex.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		if addr := matchAddress(mailFrom, v); addr != "" && (bestIdx < 0 || idx < bestIdx) {
			best, bestIdx = addr, idx
		}
	}
	if best != "" {
		return best
	}
	for _, name := range []string{"Received-Spf", "Authentication-Results"} {
		vals := h[name]
		for i := len(vals) - 1; i >= 0; i-- {
			if addr := matchAddress(envFrom, vals[i]); addr != "" {
				return addr
			}
			if addr := matchAddress(mailFrom, vals[i]); addr != "" {
				return addr
			}
		}
	}
	return ""
}

func matchAddress(re *regexp.Regexp, v string) string {
	m := re.FindStringSubmatch(v)
	if m == nil || !strings.Contains(m[1], "@") {
		return ""
	}
	return m[1]
}

// earliestRecipient returns the address of the first delivery.
func earliestRecipient(h mail.Header) string {
	if vals := h["Delivered-To"]; len(vals) > 0 {
		return firstAddress(vals[len(vals)-1])
	}
	if vals := h["Received"]; len(vals) > 0 {
		if m := receivedFor.FindStringSubmatch(vals[len(vals)-1]); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// decodeSRS recovers the original sender from an SRS0 or SRS1 rewritten
// address, see https://www.libsrs2.org/srs/srs.pdf.
func decodeSRS(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 5 {
		return ""
	}
	local := addr[:at]
	payload := local[5:]
	switch strings.ToUpper(local[:4]) {
	case "SRS0":
		return srs0(payload)
	case "SRS1":
		// SRS1=hash=forwarder==hash=tt=domain=local
		parts := strings.SplitN(payload, "=", 3)
		if len(parts) < 3 {
			return ""
		}
		return srs0(strings.TrimPrefix(parts[2], "="))
	}
	return ""
}

func srs0(payload string) string {
	parts := strings.SplitN(payload, "=", 4)
	if len(parts) < 4 || parts[2] == "" || parts[3] == "" {
		return ""
	}
	return parts[3] + "@" + parts[2]
}
