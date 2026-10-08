"""Credential-free, fail-closed plan for the fixed sdk-test full profile."""
import datetime as dt
import hashlib
import json
import pathlib
import re

TARGET = "https://kms.jiup9.com:443"
PARTS = ("core", "transit")
PART_REQUEST_LIMIT = {"core": 43, "transit": 32}
CLEANUP_REQUEST_RESERVE = 15
OPERATIONS = ["bootstrap_approle", "create_isolated_mounts", "create_acl_policies_cas",
              "create_runtime_tokens", "seed_kv_cas", "configure_owned_transit_pki_approle",
              "run_sdk_core", "run_sdk_transit", "cleanup_owned_resources"]
FIELDS = {"schema_version", "target_allowlist", "target", "environment", "namespace_mode",
          "namespace", "run_id", "resource_prefix", "bootstrap_mount", "issued_at", "expires_at",
          "request_budget", "resource_budget", "ownership", "cleanup", "allowed_operations"}


class ManifestError(ValueError):
    """Only fixed codes may leave the process; input can contain secrets."""


def _object(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ManifestError("manifest_duplicate_field")
        obj[key] = value
    return obj


def _instant(value):
    if not isinstance(value, str) or not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", value):
        raise ManifestError("manifest_time_invalid")
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        raise ManifestError("manifest_time_invalid") from None


class Manifest:
    def __init__(self, raw, *, now=None):
        try:
            if len(raw) > 16384:
                raise ManifestError("manifest_too_large")
            value = json.loads(raw, object_pairs_hook=_object)
        except (ValueError, UnicodeError, TypeError):
            raise ManifestError("manifest_json_invalid") from None
        if not isinstance(value, dict) or set(value) != FIELDS:
            raise ManifestError("manifest_fields_invalid")
        fixed = {"schema_version": 1, "target_allowlist": [TARGET], "target": TARGET,
                 "environment": "nonproduction", "namespace_mode": "named", "namespace": "sdk-test",
                 "bootstrap_mount": "auth/approle", "resource_budget": 9,
                 "ownership": "create_only_with_identity",
                 "cleanup": "reverse_created_resources_before_expiry",
                 "allowed_operations": OPERATIONS}
        if any(value.get(k) != v for k, v in fixed.items()) or type(value["schema_version"]) is not int or type(value["resource_budget"]) is not int:
            raise ManifestError("manifest_scope_invalid")
        budget = value["request_budget"]
        if (not isinstance(budget, dict) or budget != {"total": 120, "core": 43, "transit": 32, "cleanup": 15}
                or any(type(v) is not int for v in budget.values())):
            raise ManifestError("manifest_budget_invalid")
        run_id = value["run_id"]
        if not isinstance(run_id, str) or not re.fullmatch(r"remote-sdk-[0-9]{8}-[a-f0-9]{16}", run_id):
            raise ManifestError("manifest_run_id_invalid")
        if value["resource_prefix"] != run_id.removeprefix("remote-"):
            raise ManifestError("manifest_prefix_invalid")
        self.issued = _instant(value["issued_at"])
        self.expires = _instant(value["expires_at"])
        if not 0 < (self.expires-self.issued).total_seconds() <= 1200:
            raise ManifestError("manifest_window_invalid")
        self.value, self.raw = value, raw
        self.sha256 = hashlib.sha256(raw).hexdigest()
        self.run_id, self.prefix = run_id, value["resource_prefix"]
        self.assert_active(now)

    def assert_active(self, now=None):
        now = now or dt.datetime.now(dt.timezone.utc)
        if not self.issued <= now < self.expires:
            raise ManifestError("manifest_expired_or_not_yet_valid")

    def remaining_seconds(self):
        self.assert_active()
        return (self.expires-dt.datetime.now(dt.timezone.utc)).total_seconds()

    def child_environment(self, part):
        if part not in PARTS:
            raise ManifestError("manifest_part_invalid")
        seed = f"sdk-validation/{self.run_id}-{part}/seed"
        values = {"ADDRESS": TARGET, "ENVIRONMENT": "nonproduction", "NAMESPACE_MODE": "named",
                  "NAMESPACE": "sdk-test", "WRITE_PREFIX": "sdk-validation", "KV_MOUNT": self.prefix+"-kv",
                  "KV_PATH": seed, "KV_MARKER": "owned-"+self.run_id,
                  "NEGATIVE_MOUNT": self.prefix+"-kv", "NEGATIVE_PATH": seed,
                  "TRANSIT_MOUNT": self.prefix+"-transit", "TRANSIT_KEY": "cipher",
                  "TRANSIT_SIGN_KEY": "sign", "TRANSIT_HMAC_KEY": "mac",
                  "PKI_MOUNT": self.prefix+"-pki", "PKI_ROLE": "device", "PKI_DNS": "node.sdk-test.invalid",
                  "APPROLE_MOUNT": self.prefix+"-approle", "FULL_PART": part}
        return {"BAO_REMOTE_"+key: value for key, value in values.items()}

    def resource_names(self):
        return {("bootstrap_token", "bootstrap"), ("token", "runtime"), ("token", "negative"),
                *(("mount", self.prefix+"-"+kind) for kind in ("kv", "pki", "transit")),
                ("auth_mount", self.prefix+"-approle"),
                ("policy", self.prefix+"-runtime"), ("policy", self.prefix+"-negative")}


def load_manifest(path, expected_sha256=None, *, now=None):
    try:
        with pathlib.Path(path).open("rb") as stream:
            raw = stream.read(16385)
    except (OSError, TypeError, ValueError):
        raise ManifestError("manifest_unavailable") from None
    if expected_sha256 is not None and (not isinstance(expected_sha256, str)
            or not re.fullmatch(r"[a-f0-9]{64}", expected_sha256)
            or hashlib.sha256(raw).hexdigest() != expected_sha256):
        raise ManifestError("manifest_digest_mismatch")
    return Manifest(raw, now=now)
