package bao

import (
	"crypto/tls"
	"github.com/RockInMars/openbao-sdk-go/auth"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/internal/engine"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func invalid(op string) error {
	return &baoerr.Error{Code: baoerr.CodeInvalidArgument, Operation: op, Effect: baoerr.EffectNone, Message: "invalid argument"}
}
func nilInterface(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
func normalizeConfig(c Config, allowLoopbackHTTP bool) (Config, error) {
	fail := func() (Config, error) { return Config{}, invalid("CONFIG") }
	u, e := url.Parse(c.Address)
	if e != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return fail()
	}
	if u.Scheme != "https" {
		if !allowLoopbackHTTP || u.Scheme != "http" || !engine.IsLoopbackHost(u.Hostname()) {
			return fail()
		}
	}
	if port := u.Port(); port != "" {
		p, e := strconv.Atoi(port)
		if e != nil || p < 1 || p > 65535 {
			return fail()
		}
	}
	u.Path = ""
	c.Address = u.String()
	if engine.ValidateSegment(c.ClusterAlias) != nil {
		return fail()
	}
	switch c.Namespace.Mode {
	case NamespaceRoot:
		if c.Namespace.Path != "" {
			return fail()
		}
	case NamespaceNamed:
		if engine.ValidatePath(c.Namespace.Path) != nil {
			return fail()
		}
	default:
		return fail()
	}
	switch c.Auth.Mode {
	case auth.ExternalToken:
		if nilInterface(c.Auth.TokenProvider) || c.Auth.AppRole != nil {
			return fail()
		}
	case auth.ManagedAppRole:
		if c.Auth.AppRole == nil || c.Auth.TokenProvider != nil || engine.ValidatePath(c.Auth.AppRole.Mount) != nil || c.Auth.AppRole.RoleID.Len() == 0 || nilInterface(c.Auth.AppRole.SecretIDProvider) {
			return fail()
		}
	default:
		return fail()
	}
	if c.TLS.CAFile != "" && len(c.TLS.CAPEM) > 0 {
		return fail()
	}
	files := c.TLS.ClientCertFile != "" || c.TLS.ClientKeyFile != ""
	memory := len(c.TLS.ClientCertPEM) > 0 || c.TLS.ClientKeyPEM.Len() > 0
	if files && memory || files && (c.TLS.ClientCertFile == "" || c.TLS.ClientKeyFile == "") || memory && (len(c.TLS.ClientCertPEM) == 0 || c.TLS.ClientKeyPEM.Len() == 0) {
		return fail()
	}
	if c.TLS.MinVersion == 0 {
		c.TLS.MinVersion = tls.VersionTLS12
	}
	if c.TLS.MinVersion != tls.VersionTLS12 && c.TLS.MinVersion != tls.VersionTLS13 {
		return fail()
	}
	if strings.ContainsAny(c.TLS.ServerName, "\x00\r\n") {
		return fail()
	}
	if c.Network.ProxyURL != "" {
		p, e := url.Parse(c.Network.ProxyURL)
		if e != nil || p.Host == "" || (p.Scheme != "https" && p.Scheme != "http") || p.User != nil || p.Path != "" && p.Path != "/" || p.RawQuery != "" || p.Fragment != "" {
			return fail()
		}
	}
	durations := []struct {
		p *time.Duration
		d time.Duration
	}{{&c.Timeouts.Request, 10 * time.Second}, {&c.Timeouts.PKIIssue, 30 * time.Second}, {&c.Timeouts.Login, 10 * time.Second}, {&c.Timeouts.Renew, 5 * time.Second}, {&c.Timeouts.Dial, 5 * time.Second}, {&c.Timeouts.TLSHandshake, 5 * time.Second}, {&c.Network.IdleConnTimeout, 90 * time.Second}, {&c.ReadRetry.BaseDelay, 100 * time.Millisecond}, {&c.ReadRetry.MaxDelay, 2 * time.Second}}
	for _, x := range durations {
		if *x.p < 0 {
			return fail()
		}
		if *x.p == 0 {
			*x.p = x.d
		}
	}
	ints := []struct {
		p *int
		d int
	}{{&c.Network.MaxIdleConnections, 64}, {&c.Network.MaxIdlePerHost, 32}, {&c.Limits.MaxConcurrentRequests, 32}, {&c.ReadRetry.MaxAttempts, 3}}
	for _, x := range ints {
		if *x.p < 0 {
			return fail()
		}
		if *x.p == 0 {
			*x.p = x.d
		}
	}
	for _, x := range []struct {
		p *int64
		d int64
	}{{&c.Limits.MaxRequestBytes, 1 << 20}, {&c.Limits.MaxResponseBytes, 2 << 20}, {&c.Limits.MaxResponseHeaderBytes, 64 << 10}} {
		if *x.p < 0 || *x.p >= int64(math.MaxInt) {
			return fail()
		}
		if *x.p == 0 {
			*x.p = x.d
		}
	}
	if c.ReadRetry.MaxAttempts > 3 || c.ReadRetry.BaseDelay > c.ReadRetry.MaxDelay {
		return fail()
	}
	// Do not create owned secrets until all configuration validation succeeds.
	// The caller keeps its original handles; the returned copies have one owner.
	if c.Auth.AppRole != nil {
		a := *c.Auth.AppRole
		b := a.RoleID.RevealCopy()
		a.RoleID = sensitive.NewBytes(b)
		clear(b)
		c.Auth.AppRole = &a
	}
	c.TLS.CAPEM = append([]byte(nil), c.TLS.CAPEM...)
	c.TLS.ClientCertPEM = append([]byte(nil), c.TLS.ClientCertPEM...)
	b := c.TLS.ClientKeyPEM.RevealCopy()
	c.TLS.ClientKeyPEM = sensitive.NewBytes(b)
	clear(b)
	return c, nil
}

// zeroConfigSecrets applies only to copies returned by normalizeConfig, never
// caller-owned input. Value copies intentionally share each erasure handle.
func zeroConfigSecrets(c Config) {
	if c.Auth.AppRole != nil {
		c.Auth.AppRole.RoleID.Zero()
	}
	c.TLS.ClientKeyPEM.Zero()
}
