package remotecheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
)

// Only harness-owned constant paths and validated resource paths reach this helper.
// Never return raw error text, response bytes, or the lookup-self data to logging.
func (h *harness) raw(method, path string, payload any) (map[string]json.RawMessage, int, error) {
	category := "raw_kv"
	if path == "auth/token/lookup-self" {
		category = "identity"
	}
	if path == "sys/capabilities-self" {
		category = "capabilities"
	}
	if h.journalFailed || h.ctx.Err() != nil || !h.b.reserve(category) {
		return nil, 0, errors.New("budget exhausted")
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, 0, errors.New("request invalid")
		}
	}
	req, err := http.NewRequestWithContext(h.ctx, method, strings.TrimRight(h.c.address, "/")+"/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, errors.New("request invalid")
	}
	req.Header.Set("X-Vault-Token", h.c.token)
	if h.c.namespaceMode == "named" {
		req.Header.Set("X-Vault-Namespace", h.c.namespace)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.http.Do(req)
	if err != nil {
		if method == http.MethodDelete {
			return nil, 0, &baoerr.Error{Code: baoerr.CodeUnavailable, Effect: baoerr.EffectUnknown}
		}
		return nil, 0, errors.New("transport failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if method == http.MethodDelete {
			return nil, resp.StatusCode, &baoerr.Error{Code: baoerr.CodeUnavailable, HTTPStatus: resp.StatusCode, Effect: baoerr.EffectUnknown}
		}
		return nil, resp.StatusCode, errors.New("remote status rejected")
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, resp.StatusCode, nil
	}
	if method == http.MethodDelete {
		return nil, resp.StatusCode, &baoerr.Error{Code: baoerr.CodeInvalidResponse, Effect: baoerr.EffectUnknown}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, resp.StatusCode, errors.New("response rejected")
	}
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Data == nil {
		return nil, resp.StatusCode, errors.New("response rejected")
	}
	return envelope.Data, resp.StatusCode, nil
}

func contains(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}

func (h *harness) checkTokenBudget(data map[string]json.RawMessage) error {
	deadline, _ := h.ctx.Deadline()
	return checkRemainingTokenBudget(data, int64(time.Until(deadline).Seconds())+5, int64(h.b.limit-h.b.count()+1))
}

func checkRemainingTokenBudget(data map[string]json.RawMessage, minimumTTL, minimumUses int64) error {
	var ttl, uses int64
	if len(data["ttl"]) == 0 || len(data["num_uses"]) == 0 || string(data["ttl"]) == "null" || string(data["num_uses"]) == "null" || json.Unmarshal(data["ttl"], &ttl) != nil || json.Unmarshal(data["num_uses"], &uses) != nil || ttl < 0 || uses < 0 {
		return errors.New("token budget unavailable")
	}
	// A zero value is OpenBao's unlimited/non-expiring value. Never persist these fields.
	if ttl != 0 && ttl < minimumTTL {
		return errors.New("token lifetime insufficient")
	}
	if uses != 0 && uses < minimumUses {
		return errors.New("token uses insufficient")
	}
	return nil
}

func (h *harness) requiredPermissions() map[string][]string {
	c := h.c
	permissions := map[string][]string{c.mount + "/data/" + c.path: {"read"}}
	if c.mode != "isolated" {
		return permissions
	}
	path := c.prefix + "/" + c.runID + "/kv"
	if !c.full || c.fullPart == "core" {
		permissions[c.mount+"/data/"+path] = []string{"create", "update", "read"}
		permissions[c.mount+"/metadata/"+path] = []string{"read", "delete"}
		if c.softDelete {
			permissions[c.mount+"/delete/"+path] = []string{"update"}
			permissions[c.mount+"/undelete/"+path] = []string{"update"}
		}
	}
	if c.transitMount != "" {
		permissions[c.transitMount+"/keys/"+c.transitKey] = []string{"read"}
		operations := []string{"sign", "verify"}
		if c.transitType == "aes256-gcm96" {
			operations = []string{"encrypt", "decrypt"}
		}
		for _, op := range operations {
			permissions[c.transitMount+"/"+op+"/"+c.transitKey] = []string{"update"}
		}
	}
	if c.pkiMount != "" && (!c.full || c.fullPart == "core") {
		permissions[c.pkiMount+"/sign/"+c.pkiRole] = []string{"update"}
		if c.pkiRevoke {
			permissions[c.pkiMount+"/revoke"] = []string{"update"}
		}
	}
	if c.full {
		if c.fullPart == "core" {
			permissions[c.mount+"/metadata/"+c.path] = []string{"read"}
			permissions[c.mount+"/metadata/"+c.prefix+"/"+c.runID] = []string{"list"}
			permissions[c.pkiMount+"/issue/"+c.pkiRole] = []string{"update"}
			permissions[c.pkiMount+"/ca_chain"] = []string{"read"}
		} else {
			permissions[c.transitMount+"/keys/"+c.transitSignKey] = []string{"read"}
			permissions[c.transitMount+"/keys/"+c.transitHMACKey] = []string{"read"}
			for _, op := range []string{"sign", "verify"} {
				permissions[c.transitMount+"/"+op+"/"+c.transitSignKey] = []string{"update"}
			}
			for _, op := range []string{"hmac", "verify"} {
				permissions[c.transitMount+"/"+op+"/"+c.transitHMACKey+"/sha2-256"] = []string{"update"}
			}
			permissions[c.transitMount+"/rewrap/"+c.transitKey] = []string{"update"}
		}
	}
	return permissions
}
