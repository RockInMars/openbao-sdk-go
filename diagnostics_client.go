package bao

import (
	"context"
	"errors"

	"git.example.com/infra/openbao-sdk-go/baoerr"
	"git.example.com/infra/openbao-sdk-go/diagnostics"
	"git.example.com/infra/openbao-sdk-go/internal/engine"
)

// ClusterHealth reports the cluster, without a token or namespace. It can be
// used before Start and does not imply that any business resource is readable.
func (c *Client) ClusterHealth(ctx context.Context) (*diagnostics.Health, error) {
	var out *diagnostics.Health
	e := c.execute(ctx, engine.Call{Operation: engine.ClusterHealth, Path: "/v1/sys/health"}, "", func(r *engine.Response) error {
		d, e := engine.DecodeObject(r, engine.ClusterHealth)
		if e != nil {
			return e
		}
		h := diagnostics.Health{HTTPStatus: r.Status}
		for _, v := range []struct {
			name string
			dst  *bool
		}{{"initialized", &h.Initialized}, {"sealed", &h.Sealed}, {"standby", &h.Standby}} {
			b, ok := d[v.name].(bool)
			if !ok {
				return engine.InvalidResponse(r, engine.ClusterHealth)
			}
			*v.dst = b
		}
		for _, v := range []struct {
			name string
			dst  *string
		}{{"version", &h.Version}, {"cluster_id", &h.ClusterID}} {
			if value, exists := d[v.name]; exists {
				s, ok := value.(string)
				if !ok || !healthText(s) {
					return engine.InvalidResponse(r, engine.ClusterHealth)
				}
				*v.dst = s
			}
		}
		out = &h
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func healthText(s string) bool {
	if len(s) > 256 {
		return false
	}
	for _, b := range []byte(s) {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

// CheckReady reads only the requested version. Known negative readiness states
// return a complete Ready=false result. Network/protocol/cancellation failures
// remain errors rather than being confused with a completed negative probe.
func (c *Client) CheckReady(ctx context.Context, p diagnostics.ReadProbe) (*diagnostics.Readiness, error) {
	if e := contextErr(ctx, "CHECK_READY"); e != nil {
		return nil, e
	}
	if p.Version <= 0 || engine.ValidatePath(p.Mount) != nil || engine.ValidatePath(p.Path) != nil {
		return nil, invalid("CHECK_READY")
	}
	k, e := c.KVv2(p.Mount)
	if e != nil {
		return nil, e
	}
	r, e := k.ReadVersion(ctx, p.Path, p.Version)
	if e == nil {
		r.Data.Zero()
		return &diagnostics.Readiness{Ready: true}, nil
	}
	var be *baoerr.Error
	if errors.As(e, &be) && be != nil {
		switch be.Code {
		case baoerr.CodeNotReady, baoerr.CodeAuthenticationFailed, baoerr.CodePermissionDenied, baoerr.CodeNotFoundOrHidden, baoerr.CodeVersionUnavailable:
			return &diagnostics.Readiness{Ready: false, ErrorCode: be.Code}, nil
		}
	}
	return nil, e
}
