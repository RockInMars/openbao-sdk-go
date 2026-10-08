// Package remotecheck provides an opt-in test harness, not an SDK API.
package remotecheck

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/RockInMars/openbao-sdk-go/internal/engine"
)

type configuration struct {
	address, environment, namespaceMode, namespace, token, mode, runID string
	mount, path, marker                                                string
	version                                                            int
	prefix                                                             string
	softDelete                                                         bool
	caFile, certFile, keyFile                                          string
	transitMount, transitKey, transitType                              string
	pkiMount, pkiRole, pkiDNS, pkiRoot                                 string
	pkiRevoke                                                          bool
	negativeToken, negativeMount, negativePath                         string
	full                                                               bool
	fullPart                                                           string
	transitSignKey, transitHMACKey                                     string
	appRoleMount, appRoleRoleID, appRoleSecretID                       string
	manifestSHA                                                        string
	expiresAt                                                          time.Time
}

// All errors are fixed strings; invalid values can contain secrets.
func loadConfig(get func(string) string) (configuration, error) {
	c := configuration{address: get("BAO_REMOTE_ADDRESS"), environment: get("BAO_REMOTE_ENVIRONMENT"),
		namespaceMode: get("BAO_REMOTE_NAMESPACE_MODE"), namespace: get("BAO_REMOTE_NAMESPACE"),
		token: get("BAO_REMOTE_TOKEN"), mode: get("BAO_REMOTE_MODE"), runID: get("BAO_REMOTE_RUN_ID"),
		mount: get("BAO_REMOTE_KV_MOUNT"), path: get("BAO_REMOTE_KV_PATH"), marker: get("BAO_REMOTE_KV_MARKER"),
		prefix: get("BAO_REMOTE_WRITE_PREFIX"), softDelete: get("BAO_REMOTE_ALLOW_SOFT_DELETE") == "yes",
		caFile: get("BAO_REMOTE_CA_FILE"), certFile: get("BAO_REMOTE_CLIENT_CERT_FILE"), keyFile: get("BAO_REMOTE_CLIENT_KEY_FILE"),
		transitMount: get("BAO_REMOTE_TRANSIT_MOUNT"), transitKey: get("BAO_REMOTE_TRANSIT_KEY"), transitType: get("BAO_REMOTE_TRANSIT_TYPE"),
		pkiMount: get("BAO_REMOTE_PKI_MOUNT"), pkiRole: get("BAO_REMOTE_PKI_ROLE"), pkiDNS: get("BAO_REMOTE_PKI_DNS"), pkiRoot: get("BAO_REMOTE_PKI_ROOT_FILE"),
		pkiRevoke: get("BAO_REMOTE_ALLOW_REVOKE") == "yes", negativeToken: get("BAO_REMOTE_NEGATIVE_TOKEN"),
		negativeMount: get("BAO_REMOTE_NEGATIVE_MOUNT"), negativePath: get("BAO_REMOTE_NEGATIVE_PATH"),
		full: get("BAO_REMOTE_FULL") == "yes", fullPart: get("BAO_REMOTE_FULL_PART"), transitSignKey: get("BAO_REMOTE_TRANSIT_SIGN_KEY"),
		transitHMACKey: get("BAO_REMOTE_TRANSIT_HMAC_KEY"), appRoleMount: get("BAO_REMOTE_APPROLE_MOUNT"),
		appRoleRoleID: get("BAO_REMOTE_APPROLE_ROLE_ID"), appRoleSecretID: get("BAO_REMOTE_APPROLE_SECRET_ID")}
	bad := func() (configuration, error) {
		return configuration{}, errors.New("invalid or incomplete remote configuration")
	}
	u, err := url.Parse(c.address)
	if err != nil || u.Scheme != "https" || u.Hostname() != "kms.jiup9.com" || (u.Port() != "" && u.Port() != "443") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return bad()
	}
	if get("BAO_REMOTE_CONFIRMED") != "yes" || c.environment != "nonproduction" || !validToken(c.token) {
		return bad()
	}
	if c.namespaceMode == "root" {
		if c.namespace != "" {
			return bad()
		}
	} else if c.namespaceMode != "named" || engine.ValidatePath(c.namespace) != nil {
		return bad()
	}
	if c.mode != "readonly" && c.mode != "isolated" {
		return bad()
	}
	c.version, err = strconv.Atoi(get("BAO_REMOTE_KV_VERSION"))
	if err != nil || c.version <= 0 || engine.ValidatePath(c.mount) != nil || engine.ValidatePath(c.path) != nil || c.marker == "" || len(c.marker) > 256 || engine.ValidateSegment(c.runID) != nil {
		return bad()
	}
	if (c.certFile == "") != (c.keyFile == "") {
		return bad()
	}
	if c.mode == "isolated" && (engine.ValidatePath(c.prefix) != nil || get("BAO_REMOTE_ALLOW_WRITE") != "yes" || get("BAO_REMOTE_ALLOW_CLEANUP") != "yes") {
		return bad()
	}
	if c.mode == "readonly" && (c.softDelete || c.pkiRevoke || c.transitMount != "" || c.pkiMount != "") {
		return bad()
	}
	if c.transitMount != "" || c.transitKey != "" || c.transitType != "" {
		if engine.ValidatePath(c.transitMount) != nil || engine.ValidateSegment(c.transitKey) != nil {
			return bad()
		}
		switch c.transitType {
		case "aes256-gcm96", "ecdsa-p256", "rsa-2048", "rsa-3072", "rsa-4096", "ed25519":
		default:
			return bad()
		}
	}
	if c.pkiMount != "" || c.pkiRole != "" || c.pkiDNS != "" || c.pkiRoot != "" {
		if engine.ValidatePath(c.pkiMount) != nil || engine.ValidateSegment(c.pkiRole) != nil || !validDNS(c.pkiDNS) || c.pkiRoot == "" || get("BAO_REMOTE_ALLOW_SIGN_CSR") != "yes" {
			return bad()
		}
	}
	if c.negativeToken != "" || c.negativeMount != "" || c.negativePath != "" {
		if c.mode != "isolated" || !validToken(c.negativeToken) || c.negativeToken == c.token || engine.ValidatePath(c.negativeMount) != nil || engine.ValidatePath(c.negativePath) != nil {
			return bad()
		}
	}
	if get("BAO_REMOTE_FULL") != "" && get("BAO_REMOTE_FULL") != "yes" {
		return bad()
	}
	if c.full {
		if bindManifest(&c, get) != nil {
			return bad()
		}
		if c.namespaceMode != "named" || c.namespace != "sdk-test" ||
			(c.fullPart != "core" && c.fullPart != "transit") ||
			c.prefix != "sdk-validation" || c.path != c.prefix+"/"+c.runID+"/seed" ||
			c.negativeMount != c.mount || c.negativePath != c.path ||
			c.transitKey != "cipher" || c.transitSignKey != "sign" || c.transitHMACKey != "mac" ||
			c.pkiRole != "device" || c.pkiDNS != "node.sdk-test.invalid" ||
			c.mode != "isolated" || !c.softDelete || c.transitType != "aes256-gcm96" || c.pkiMount == "" || !c.pkiRevoke ||
			c.negativeToken == "" || c.negativeMount == "" || c.negativePath == "" ||
			!strings.HasPrefix(c.path, c.prefix+"/"+c.runID+"/") ||
			engine.ValidateSegment(c.transitSignKey) != nil || engine.ValidateSegment(c.transitHMACKey) != nil ||
			c.transitSignKey == c.transitHMACKey || c.transitSignKey == c.transitKey || c.transitHMACKey == c.transitKey ||
			engine.ValidatePath(c.appRoleMount) != nil || !validToken(c.appRoleRoleID) || !validToken(c.appRoleSecretID) {
			return bad()
		}
	} else if c.fullPart != "" || c.transitSignKey != "" || c.transitHMACKey != "" || c.appRoleMount != "" || c.appRoleRoleID != "" || c.appRoleSecretID != "" {
		return bad()
	}
	return c, nil
}

func validToken(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}
func validDNS(value string) bool {
	if len(value) > 253 || !strings.Contains(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}
