package remotecheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/diagnostics"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
)

type caseResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Code    string `json:"code,omitempty"`
	Unknown bool   `json:"unknown,omitempty"`
}
type resource struct {
	Kind            string `json:"kind"`
	Mount           string `json:"mount"`
	Path            string `json:"path"`
	State           string `json:"state"`
	Versions        []int  `json:"versions,omitempty"`
	OwnershipSHA256 string `json:"ownership_sha256,omitempty"`
}
type result struct {
	ManifestSHA256    string         `json:"manifest_sha256,omitempty"`
	Status            string         `json:"status"`
	Cases             []caseResult   `json:"cases"`
	Requests          int            `json:"requests"`
	ServerVersion     string         `json:"server_version,omitempty"`
	Resources         []resource     `json:"resources"`
	ElapsedMS         int64          `json:"elapsed_ms"`
	TargetContext     map[string]any `json:"target_context"`
	RequestCategories map[string]int `json:"request_categories"`
}

type harness struct {
	c         configuration
	sdk       *bao.Client
	kv        *bao.KVClient
	http      *http.Client
	transport *http.Transport
	ctx       context.Context
	cancel    context.CancelFunc
	b         *budget
	output    result
	// Called before and after mutations; failure forbids another remote action.
	persist       func(result) error
	journalFailed bool
}

func newHarness(c configuration, persist func(result) error) (*harness, error) {
	limit, duration := 10, 90*time.Second
	if c.mode == "isolated" {
		limit, duration = 60, 10*time.Minute
	}
	if c.full {
		if c.manifestSHA == "" || !c.expiresAt.After(time.Now()) {
			return nil, errors.New("manifest missing or expired")
		}
		if remaining := time.Until(c.expiresAt); remaining < duration {
			duration = remaining
		}
		if c.fullPart == "core" {
			// The fixture uses the same cap and reserves separate setup and cleanup calls.
			limit = 43
		} else {
			limit = 32
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	b := &budget{limit: limit, cancel: cancel, categories: map[string]int{}}
	secret := sensitive.NewBytes([]byte(c.token))
	provider, err := auth.NewStaticToken(secret)
	secret.Zero()
	if err != nil {
		cancel()
		return nil, errors.New("provider unavailable")
	}
	sdk, err := bao.New(bao.Config{Address: c.address, ClusterAlias: "remote-test", Namespace: bao.NamespaceConfig{Mode: bao.NamespaceMode(c.namespaceMode), Path: c.namespace},
		Auth: auth.Config{Mode: auth.ExternalToken, TokenProvider: provider}, TLS: bao.TLSConfig{CAFile: c.caFile, ClientCertFile: c.certFile, ClientKeyFile: c.keyFile},
		Timeouts: bao.TimeoutConfig{Request: 5 * time.Second, Dial: 3 * time.Second, TLSHandshake: 3 * time.Second},
		Limits:   bao.LimitConfig{MaxConcurrentRequests: 1}, ReadRetry: bao.ReadRetryConfig{MaxAttempts: 1}}, bao.WithObserver(b))
	if err != nil {
		cancel()
		return nil, errors.New("SDK configuration rejected")
	}
	kvClient, err := sdk.KVv2(c.mount)
	if err != nil {
		cancel()
		return nil, errors.New("KV configuration rejected")
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.caFile != "" {
		pem, readErr := os.ReadFile(c.caFile)
		if readErr != nil {
			cancel()
			return nil, errors.New("CA unavailable")
		}
		tc.RootCAs = x509.NewCertPool()
		if !tc.RootCAs.AppendCertsFromPEM(pem) {
			cancel()
			return nil, errors.New("CA invalid")
		}
	}
	if c.certFile != "" {
		cert, loadErr := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if loadErr != nil {
			cancel()
			return nil, errors.New("client certificate unavailable")
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	tr := &http.Transport{TLSClientConfig: tc, Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, TLSHandshakeTimeout: 3 * time.Second, MaxResponseHeaderBytes: 64 << 10, DisableCompression: true}
	h := &harness{c: c, sdk: sdk, kv: kvClient, transport: tr, ctx: ctx, cancel: cancel, b: b, persist: persist, output: result{Status: "FAIL", Cases: []caseResult{}, Resources: []resource{}}}
	h.http = &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return h, nil
}

func (h *harness) save() bool {
	h.output.ManifestSHA256 = h.c.manifestSHA
	h.output.Requests = h.b.count()
	h.output.RequestCategories = h.b.counts()
	h.output.TargetContext = map[string]any{"address": h.c.address, "environment": h.c.environment, "namespace_mode": h.c.namespaceMode, "namespace": h.c.namespace,
		"mode": h.c.mode, "kv_mount": h.c.mount, "kv_path": h.c.path, "kv_version": h.c.version,
		"write_prefix": h.c.prefix, "transit_mount": h.c.transitMount, "transit_key": h.c.transitKey,
		"transit_sign_key": h.c.transitSignKey, "transit_hmac_key": h.c.transitHMACKey,
		"pki_mount": h.c.pkiMount, "pki_role": h.c.pkiRole, "pki_dns": h.c.pkiDNS,
		"negative_mount": h.c.negativeMount, "negative_path": h.c.negativePath,
		"approle_mount": h.c.appRoleMount, "full": h.c.full, "full_part": h.c.fullPart}
	if h.persist != nil && !h.journalFailed {
		if h.persist(h.output) != nil {
			h.journalFailed = true
			h.output.Status = "FAIL"
			h.cancel()
		}
	}
	return !h.journalFailed
}

func (h *harness) check(name string, err error) bool {
	item := caseResult{Name: name, Status: "PASS"}
	if err != nil {
		item.Status = "FAIL"
		item.Code = "check_failed"
		var e *baoerr.Error
		if errors.As(err, &e) {
			item.Code = e.Code
			item.Unknown = e.Effect == baoerr.EffectUnknown
		}
	}
	h.output.Cases = append(h.output.Cases, item)
	return h.save() && err == nil
}

func (h *harness) run() (out result) {
	start := time.Now()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.check("close", h.sdk.Close(closeCtx))
		if h.c.full {
			var stateErr error
			if h.sdk.State().Lifecycle != "CLOSED" {
				stateErr = errors.New("client did not close")
			}
			h.check("state_closed", stateErr)
		}
		h.transport.CloseIdleConnections()
		h.cancel()
		h.output.ElapsedMS = time.Since(start).Milliseconds()
		h.output.Requests = h.b.count()
		if !h.journalFailed && len(h.output.Cases) > 0 {
			h.output.Status = "PASS"
			for _, c := range h.output.Cases {
				if c.Status == "FAIL" {
					h.output.Status = "FAIL"
				} else if c.Status == "BLOCKED" && h.output.Status != "FAIL" {
					h.output.Status = "BLOCKED"
				}
			}
		}
		h.save()
		out = h.output
	}()
	health, err := h.sdk.ClusterHealth(h.ctx)
	if err == nil && (!health.Initialized || health.Sealed || health.Standby) {
		err = errors.New("health is not active")
	}
	if !h.check("health", err) {
		return
	}
	if regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`).MatchString(health.Version) && len(health.Version) < 64 {
		h.output.ServerVersion = health.Version
	}
	if !h.check("start", h.sdk.Start(h.ctx)) {
		return
	}
	if h.c.full {
		var stateErr error
		if h.sdk.State().Lifecycle != "READY" {
			stateErr = errors.New("client did not become ready")
		}
		if !h.check("state_ready", stateErr) {
			return
		}
	}
	identity, status, err := h.raw(http.MethodGet, "auth/token/lookup-self", nil)
	if status == http.StatusForbidden {
		h.output.Cases = append(h.output.Cases, caseResult{Name: "identity", Status: "BLOCKED", Code: "self_query_forbidden"})
		h.save()
		if h.c.mode == "isolated" {
			return
		}
	} else {
		if err == nil {
			err = h.checkTokenBudget(identity)
		}
		if !h.check("identity", err) {
			return
		}
	}
	permissions := h.requiredPermissions()
	paths := make([]string, 0, len(permissions))
	for path := range permissions {
		paths = append(paths, path)
	}
	caps, _, err := h.raw(http.MethodPost, "sys/capabilities-self", map[string]any{"paths": paths})
	if err == nil {
		for _, path := range paths {
			var list []string
			raw := caps[path]
			if len(paths) == 1 && raw == nil {
				raw = caps["capabilities"]
			}
			if json.Unmarshal(raw, &list) != nil {
				err = errors.New("capabilities invalid")
				break
			}
			if contains(list, "root") {
				err = errors.New("unrestricted identity rejected")
			}
			if path == h.c.transitMount+"/keys/"+h.c.transitKey ||
				(h.c.full && (path == h.c.transitMount+"/keys/"+h.c.transitSignKey || path == h.c.transitMount+"/keys/"+h.c.transitHMACKey)) {
				for _, management := range []string{"create", "update", "delete", "sudo"} {
					if contains(list, management) {
						err = errors.New("key administration privilege rejected")
					}
				}
			}
			if h.c.transitType == "aes256-gcm96" && path == h.c.transitMount+"/encrypt/"+h.c.transitKey && (contains(list, "create") || contains(list, "root")) {
				err = errors.New("implicit key creation privilege rejected")
			}
			if h.c.full && (path == h.c.transitMount+"/sign/"+h.c.transitSignKey || path == h.c.transitMount+"/hmac/"+h.c.transitHMACKey+"/sha2-256") && (contains(list, "create") || contains(list, "root")) {
				err = errors.New("implicit key creation privilege rejected")
			}
			for _, capability := range permissions[path] {
				if !contains(list, capability) {
					err = errors.New("capability unavailable")
				}
			}
		}
	}
	if !h.check("capabilities", err) {
		return
	}
	read, err := h.kv.ReadVersion(h.ctx, h.c.path, h.c.version)
	if err == nil {
		var marker map[string]string
		err = read.Data.Decode(&marker)
		read.Data.Zero()
		if err == nil && marker["sdk_test_marker"] != h.c.marker {
			err = errors.New("marker mismatch")
		}
	}
	if !h.check("read_version", err) {
		return
	}
	ready, err := h.sdk.CheckReady(h.ctx, diagnostics.ReadProbe{Mount: h.c.mount, Path: h.c.path, Version: h.c.version})
	if err == nil && !ready.Ready {
		err = errors.New("not ready")
	}
	if !h.check("ready", err) {
		return
	}
	if h.c.full && h.c.fullPart == "transit" {
		h.testFullTransit()
		return
	}
	if h.c.full && !h.testFullKV() {
		return
	}
	if h.c.mode == "isolated" {
		if !h.writeKV() {
			return
		}
		if !h.optionalScenarios() {
			return
		}
		if h.c.full && (!h.testFullAppRole() || !h.testFullPKI()) {
			return
		}
	}
	return
}
