package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Notifications: short, titled messages about Axiom-level conditions (a
// cluster went down, a budget was blown) delivered through the same Slack /
// generic webhook channels — and the same "notify" switch — the warning-event
// dispatcher uses. Delivery is best effort: one attempt, failures are logged,
// and every replica sends its own.

// Notification is one message. Path is an Axiom-relative UI path ("/fleet").
type Notification struct {
	Kind     string // the matching audit action, e.g. "fleet.cluster_unreachable"
	Severity string // SeverityWarning | SeverityInfo
	Title    string // "Axiom: cluster eu-west-1 is unreachable"
	Detail   string // one line
	Path     string
	Time     time.Time
}

// Notifier sends notifications. Implementations must not block for long and
// must never panic the caller.
type Notifier interface {
	Notify(ctx context.Context, n Notification)
}

// NopNotifier drops every notification.
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, Notification) {}

// hubNotifier delivers through the integration hub's Slack/webhook settings.
type hubNotifier struct{ hub *IntegrationHub }

// NewHubNotifier returns a Notifier that sends through hub's configured Slack
// and/or alert webhook when notifications are enabled, and is a no-op
// otherwise. A nil hub yields a NopNotifier.
func NewHubNotifier(hub *IntegrationHub) Notifier {
	if hub == nil {
		return NopNotifier{}
	}
	return hubNotifier{hub: hub}
}

func (h hubNotifier) Notify(ctx context.Context, n Notification) {
	in := h.hub.Get()
	if !notifyEnabled(in) {
		return
	}
	text, payload := formatNotification(n)
	deliver(ctx, in, text, payload)
}

// notifyDetailMax bounds the detail line.
const notifyDetailMax = 300

var notifyURLRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// cleanNotifyText makes s a single line, reduces any embedded URL to its
// scheme://host (a path or query may carry a token) and bounds its length.
func cleanNotifyText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = notifyURLRe.ReplaceAllStringFunc(s, redactedHost)
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

// formatNotification renders the Slack text and the webhook JSON payload.
func formatNotification(n Notification) (string, map[string]any) {
	title := cleanNotifyText(n.Title, 200)
	detail := cleanNotifyText(n.Detail, notifyDetailMax)
	path := n.Path
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.ContainsAny(path, " \t\n") {
		path = ""
	}
	if n.Time.IsZero() {
		n.Time = time.Now()
	}
	icon := ":information_source:"
	if n.Severity == SeverityWarning || n.Severity == SeverityCritical {
		icon = ":warning:"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s *%s*", icon, title)
	if detail != "" {
		b.WriteString("\n" + detail)
	}
	if path != "" {
		b.WriteString("\nIn Axiom: `" + path + "`")
	}
	payload := map[string]any{
		"source":   "zyvor-axiom",
		"kind":     n.Kind,
		"severity": n.Severity,
		"title":    title,
		"detail":   detail,
		"path":     path,
		"time":     n.Time.UTC(),
	}
	return b.String(), payload
}

// NotifyDebounce is the shortest gap between two notifications for the same
// subject and the same target state.
const NotifyDebounce = 10 * time.Minute

// notifyGate implements the debounce rule shared by the fleet prober and the
// budget watcher. Per subject (cluster or namespace) it remembers when each
// "bad" state was last announced and whether a bad state is outstanding:
//   - a bad state is announced unless the same state was announced for the
//     same subject within NotifyDebounce;
//   - the "good" state is announced exactly when a bad one was announced and
//     not yet followed by a good one (so a recovery always follows an
//     announced outage, and an unannounced blip does not announce a recovery).
type notifyGate struct {
	mu          sync.Mutex
	last        map[string]map[string]time.Time // subject -> state -> last sent
	outstanding map[string]bool
}

func newNotifyGate() *notifyGate {
	return &notifyGate{last: map[string]map[string]time.Time{}, outstanding: map[string]bool{}}
}

// allowBad reports whether to announce subject entering bad state now, and
// records it if so.
func (g *notifyGate) allowBad(subject, state string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	m := g.last[subject]
	if m == nil {
		m = map[string]time.Time{}
		g.last[subject] = m
	}
	if t, ok := m[state]; ok && now.Sub(t) < NotifyDebounce {
		return false
	}
	m[state] = now
	g.outstanding[subject] = true
	return true
}

// allowGood reports whether to announce subject returning to its good state.
func (g *notifyGate) allowGood(subject string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.outstanding[subject] {
		return false
	}
	delete(g.outstanding, subject)
	return true
}

// forget drops everything remembered about subject.
func (g *notifyGate) forget(subject string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.last, subject)
	delete(g.outstanding, subject)
}

// humanDuration renders d coarsely: "45s", "12m", "3h 5m", "2d 4h".
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		h := int(d / time.Hour)
		if m := int((d % time.Hour) / time.Minute); m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d / (24 * time.Hour))
	if h := int((d % (24 * time.Hour)) / time.Hour); h > 0 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}
