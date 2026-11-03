package platform

import (
	"regexp"
	"sort"
	"strings"
)

// Redactor scrubs text before it leaves the cluster for an LLM endpoint.
//
// It is deliberately a deny-by-pattern filter layered on top of an allow-list
// context builder (see ai.go): the builder never reads Secret/ConfigMap data
// or container env values in the first place, and the redactor is the second
// line of defence for credentials that leak into free text (log lines, event
// messages, the user's own question).
//
// Always on: bearer/basic tokens, JWTs, well-known API-key prefixes, PEM
// blocks, kubeconfig documents, URL userinfo, `password=`-style assignments,
// env-dump lines, Secret `data:`/`stringData:` values and base64-looking blobs.
// Optional (maskNetwork, the Settings "redact" switch): IP addresses and DNS
// names.
type Redactor struct {
	maskNetwork bool
	literals    []string
}

const (
	aiRedactedMark = "[REDACTED]"
	aiRedactedBlob = "[REDACTED-BLOB]"
	aiRedactedIP   = "[IP]"
	aiRedactedHost = "[HOST]"
)

// NewRedactor builds a Redactor. maskNetwork additionally masks IPs/hostnames.
func NewRedactor(maskNetwork bool) *Redactor { return &Redactor{maskNetwork: maskNetwork} }

// AddLiteral registers an exact value that must never appear in output, e.g. a
// known secret. Values shorter than 4 bytes are ignored (they would shred
// ordinary text).
func (r *Redactor) AddLiteral(v string) {
	if len(strings.TrimSpace(v)) >= 4 {
		r.literals = append(r.literals, v)
	}
}

var (
	rePEM         = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]+-----.*?(?:-----END [A-Z0-9 ]+-----|$)`)
	reJWT         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}(?:\.[A-Za-z0-9_-]*)?`)
	reBearer      = regexp.MustCompile(`(?i)\b(bearer)([ \t]+)[A-Za-z0-9._~+/=-]{6,}`)
	reBasic       = regexp.MustCompile(`(?i)\b(basic)([ \t]+)([A-Za-z0-9+/]{8,}={0,2})`)
	reURLUser     = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s@]+@`)
	reKnownKeys   = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{12,}\b|\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b|\bglpat-[A-Za-z0-9_-]{16,}\b|\bxox[abprs]-[A-Za-z0-9-]{10,}\b|\bsk-[A-Za-z0-9_-]{16,}\b|\bAIza[0-9A-Za-z_-]{30,}\b|\bnpm_[A-Za-z0-9]{30,}\b|\bdckr_pat_[A-Za-z0-9_-]{16,}\b`)
	reSensitiveKV = regexp.MustCompile(`(?i)(["']?[\w.-]*(?:password|passwd|pwd|passphrase|secret|token|api[_-]?key|apikey|access[_-]?key|private[_-]?key|credential|authorization|auth[_-]?(?:token|key|header)|signature|cookie|session[_-]?(?:id|token|key)|client[_-]?key|client[_-]?certificate|certificate[_-]?authority)[\w.-]*["']?[ \t]*[:=][ \t]*)(\[REDACTED[^\]\n]*\]|"[^"\n]*"|'[^'\n]*'|[^\s,;&}\]"']+)`)
	// An env dump line: NAME=value where NAME is SHOUTY_SNAKE (printenv, `env`
	// in an init container, a crash log that prints its environment).
	reEnvLine = regexp.MustCompile(`(?m)^([ \t]*(?:export[ \t]+)?[A-Z][A-Z0-9_]{2,}=)(.+)$`)
	// kubectl-style env listing: "- name: FOO\n  value: bar".
	reEnvValueYAML = regexp.MustCompile(`(?m)^([ \t]*-?[ \t]*value:[ \t]*)(\S.*)$`)
	reB64Run       = regexp.MustCompile(`[A-Za-z0-9+/_-]{40,}={0,2}`)
	reIPv4         = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?:/\d{1,2})?\b`)
	// Only full (8-group) or "::"-compressed forms, so HH:MM:SS timestamps survive.
	reIPv6    = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b|(?:[0-9a-fA-F]{1,4}:){1,7}:(?:[0-9a-fA-F]{1,4}(?::[0-9a-fA-F]{1,4}){0,6})?|::[0-9a-fA-F]{1,4}(?::[0-9a-fA-F]{1,4}){0,6}`)
	reHost    = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\b`)
	reHostTLD = regexp.MustCompile(`(?i)\.(?:com|net|org|io|dev|local|internal|cluster|svc|lan|corp|cloud|app|ai|co|edu|gov|info|biz|xyz|home|intranet|localdomain)$`)
)

// Redact returns s with secrets (and, if enabled, network identifiers) removed.
// It is idempotent: Redact(Redact(x)) == Redact(x).
func (r *Redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	// 0. Exact literals, longest first so a prefix doesn't shadow the whole value.
	if len(r.literals) > 0 {
		lits := append([]string(nil), r.literals...)
		sort.Slice(lits, func(i, j int) bool { return len(lits[i]) > len(lits[j]) })
		for _, l := range lits {
			s = strings.ReplaceAll(s, l, aiRedactedMark)
		}
	}
	// 1. Whole kubeconfig documents and Secret manifests.
	s = redactKubeconfig(s)
	s = redactSecretManifest(s)
	// 2. Key material and tokens.
	s = rePEM.ReplaceAllString(s, aiRedactedMark)
	s = reJWT.ReplaceAllString(s, aiRedactedMark)
	s = reBearer.ReplaceAllString(s, "${1}${2}"+aiRedactedMark)
	s = reBasic.ReplaceAllStringFunc(s, func(m string) string {
		sub := reBasic.FindStringSubmatch(m)
		if strings.IndexFunc(sub[3], func(c rune) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' }) < 0 {
			return m // plain lower-case prose such as "basic authentication"
		}
		return sub[1] + sub[2] + aiRedactedMark
	})
	s = reURLUser.ReplaceAllString(s, "${1}"+aiRedactedMark+"@")
	s = reKnownKeys.ReplaceAllString(s, aiRedactedMark)
	// 3. Sensitive assignments, env dumps, `value:` lines under env entries.
	s = reSensitiveKV.ReplaceAllString(s, "${1}"+aiRedactedMark)
	s = reEnvLine.ReplaceAllString(s, "${1}"+aiRedactedMark)
	s = redactEnvValueYAML(s)
	// 4. Anything that still looks like a base64 blob.
	s = redactBlobs(s)
	// 5. Network identifiers (optional).
	if r.maskNetwork {
		s = reIPv4.ReplaceAllString(s, aiRedactedIP)
		s = reIPv6.ReplaceAllString(s, aiRedactedIP)
		s = redactHosts(s)
	}
	return s
}

func redactKubeconfig(s string) string {
	l := strings.ToLower(s)
	if strings.Contains(l, "kind: config") && (strings.Contains(l, "clusters:") || strings.Contains(l, "users:") || strings.Contains(l, "contexts:")) {
		return "[REDACTED kubeconfig]"
	}
	if strings.Contains(l, "client-key-data") || strings.Contains(l, "client-certificate-data") || strings.Contains(l, "certificate-authority-data") {
		return "[REDACTED kubeconfig]"
	}
	return s
}

// redactSecretManifest blanks the values under `data:` / `stringData:` of any
// YAML/JSON-ish text that declares `kind: Secret` (or ConfigMap), so a pasted
// or logged manifest cannot smuggle values out.
func redactSecretManifest(s string) string {
	l := strings.ToLower(s)
	isSecret := strings.Contains(l, "kind: secret") || strings.Contains(l, `"kind":"secret"`) || strings.Contains(l, `"kind": "secret"`)
	isConfigMap := strings.Contains(l, "kind: configmap") || strings.Contains(l, `"kind":"configmap"`) || strings.Contains(l, `"kind": "configmap"`)
	if !isSecret && !isConfigMap {
		return s
	}
	lines := strings.Split(s, "\n")
	inData := false
	dataIndent := 0
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		indent := len(ln) - len(strings.TrimLeft(ln, " \t"))
		switch {
		case trim == "data:" || trim == "stringData:" || trim == "binaryData:":
			inData, dataIndent = true, indent
		case inData && trim != "" && indent <= dataIndent:
			inData = false
		}
		if inData && trim != "data:" && trim != "stringData:" && trim != "binaryData:" {
			if k, _, ok := strings.Cut(ln, ":"); ok {
				lines[i] = k + ": " + aiRedactedMark
			} else {
				lines[i] = strings.Repeat(" ", indent) + aiRedactedMark
			}
		}
	}
	out := strings.Join(lines, "\n")
	// JSON form: "data":{"k":"v"} -> blank every string value inside the object.
	out = reJSONDataBlock.ReplaceAllStringFunc(out, func(m string) string {
		return reJSONStringVal.ReplaceAllString(m, `${1}"`+aiRedactedMark+`"`)
	})
	return out
}

var (
	reJSONDataBlock = regexp.MustCompile(`(?s)"(?:data|stringData|binaryData)"\s*:\s*\{.*?\}`)
	reJSONStringVal = regexp.MustCompile(`("[^"]*"\s*:\s*)"[^"]*"`)
)

// redactEnvValueYAML blanks `value:` lines that follow a `name:` line, the shape
// of a pod spec env list pasted into a question or printed in a log.
func redactEnvValueYAML(s string) string {
	if !strings.Contains(s, "value:") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		prev := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i-1]), "-"))
		if !strings.HasPrefix(prev, "name:") {
			continue
		}
		lines[i] = reEnvValueYAML.ReplaceAllString(lines[i], "${1}"+aiRedactedMark)
	}
	return strings.Join(lines, "\n")
}

func redactBlobs(s string) string {
	idx := reB64Run.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range idx {
		run := s[m[0]:m[1]]
		b.WriteString(s[last:m[0]])
		if looksLikeBlob(run) {
			b.WriteString(aiRedactedBlob)
		} else {
			b.WriteString(run)
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// looksLikeBlob is true for runs that mix upper-case, lower-case and digits (or
// are long enough to be certain), which excludes kebab-case resource names,
// file paths and image digests (lower-case hex).
func looksLikeBlob(run string) bool {
	var up, lo, dg bool
	for _, c := range run {
		switch {
		case c >= 'A' && c <= 'Z':
			up = true
		case c >= 'a' && c <= 'z':
			lo = true
		case c >= '0' && c <= '9':
			dg = true
		}
	}
	if strings.HasSuffix(run, "=") {
		return true
	}
	if up && lo && dg {
		return true
	}
	// Very long runs are suspicious even when single-case (e.g. a hex key).
	return len(run) >= 64 && (dg && (up || lo)) && !isLowerHex(run)
}

func isLowerHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func redactHosts(s string) string {
	idx := reHost.FindAllStringIndex(s, -1)
	if len(idx) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range idx {
		h := s[m[0]:m[1]]
		b.WriteString(s[last:m[0]])
		if strings.Count(h, ".") >= 2 || reHostTLD.MatchString(h) {
			b.WriteString(aiRedactedHost)
		} else {
			b.WriteString(h)
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	return b.String()
}
