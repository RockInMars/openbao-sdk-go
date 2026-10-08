#!/usr/bin/env python3
"""Prepare, run and clean only the resources in a newly authorized sdk-test manifest.

No credentials, historical run IDs or prior report hashes are part of the profile.
Use --validate-only before reviewing a plan. --execute requires fresh authorization.
"""
import argparse
import hashlib
import http.client
import json
import os
import pathlib
import ssl
import sys
import time

from tooling import finish_report, start_report
from command_runner import run_command, succeeded
from remote_manifest import (ManifestError, load_manifest, PARTS, PART_REQUEST_LIMIT,
                             CLEANUP_REQUEST_RESERVE)

ROOT = pathlib.Path(__file__).resolve().parents[1]
WRITE_PREFIX = "sdk-validation"
DNS, ROLE = "node.sdk-test.invalid", "device"
KEYS = {"cipher": "cipher", "sign": "sign", "mac": "mac"}


class StopRun(Exception):
    def __init__(self, code):
        super().__init__(code)
        self.code = code


def validate_approle_input(mount, role_id, secret_id, manifest):
    if mount != manifest.value["bootstrap_mount"]:
        raise StopRun("approle_mount_unconfirmed")
    if (not isinstance(role_id, str) or not isinstance(secret_id, str)
            or not role_id or not secret_id or len(role_id) > 4096 or len(secret_id) > 4096
            or any(c in role_id + secret_id for c in "\r\n\x00")):
        raise StopRun("approle_input_invalid")


def reserve_directory(root, manifest):
    runs = pathlib.Path(root)/".artifacts"/"runs"
    if any((runs/(manifest.run_id+"-"+part)).exists() for part in PARTS):
        raise StopRun("remote_run_id_already_exists")
    directory = runs/(manifest.run_id+"-fixture")
    try:
        directory.mkdir(parents=True, exist_ok=False)
        with (directory/"manifest.json").open("xb") as stream:
            stream.write(manifest.raw)
    except FileExistsError:
        raise StopRun("remote_run_id_already_exists") from None
    return directory


class Fixture:
    def __init__(self, manifest, directory):
        self.plan = manifest
        self.directory = pathlib.Path(directory)
        self.prefix = manifest.prefix
        self.run_id = manifest.run_id
        self.mounts = {kind: self.prefix + '-' + kind for kind in ('kv', 'transit', 'pki')}
        self.auth_mount = self.prefix + '-approle'
        self.policies = {name: self.prefix + '-' + name for name in ('runtime', 'negative')}
        self.admin = None
        self.limit = manifest.value['request_budget']['total']
        self.child_requests = 0
        self.runtime_token = None
        self.negative_token = None
        self.role_id = None
        self.secret_id = None
        self.sequence = 0
        self.unsafe_cleanup = False
        self.mutation_started = False
        self.phase = "setup"
        self.started = time.monotonic()
        self.report = start_report("REMOTE_SUPPLEMENTAL_FIXTURE", ROOT)
        self.report.update(
            run_id=self.run_id+"-fixture", remote_run_id=self.run_id,
            manifest_sha256=manifest.sha256, expires_at=manifest.value["expires_at"],
            target="https://kms.jiup9.com:443", namespace="sdk-test", resource_prefix=self.prefix,
            environment="nonproduction", credential_source="AppRole RoleID/SecretID process_environment",
            request_limit=self.limit, admin_requests=0, child_requests=0,
            request_count_status="EXACT",
            resources=[], steps=[], sdk_runs=[], status="NOT_RUN", cleanup="NOT_RUN",
            argv=[sys.executable, "-B", str(pathlib.Path(__file__).resolve())],
            working_directory=str(ROOT),
        )
        self.save()

    def seed_path(self, part):
        return WRITE_PREFIX + "/" + self.run_id + "-" + part + "/seed"

    def register_resource(self, kind, name):
        if ((kind, name) not in self.plan.resource_names()
                or len(self.report["resources"]) >= self.plan.value["resource_budget"]
                or any((x["kind"], x["name"]) == (kind, name) for x in self.report["resources"])):
            raise StopRun("resource_outside_manifest_or_duplicate")
        item = {"kind": kind, "name": name, "state": "creation_pending"}
        self.report["resources"].append(item)
        self.save()
        return item

    def active(self):
        try:
            self.plan.assert_active()
        except ManifestError:
            raise StopRun("manifest_expired_or_not_yet_valid") from None
        if time.monotonic() - self.started >= 1200:
            raise StopRun("time_budget_exhausted")

    def allowed_call(self, method, path, payload):
        pairs = {("GET", "/v1/sys/health"), ("GET", "/v1/auth/token/lookup-self"), ("POST", "/v1/sys/capabilities-self")}
        resources = self.plan.resource_names()
        for kind, name in resources:
            stem = {"mount": "sys/mounts/", "auth_mount": "sys/auth/", "policy": "sys/policies/acl/"}.get(kind)
            if stem:
                pairs.add(("GET", "/v1/"+stem+name))
                pairs.add(("POST", "/v1/"+stem+name))
                if method == "POST" and path == "/v1/"+stem+name and kind == "policy":
                    if not isinstance(payload, dict) or payload.get("cas") != -1 or payload.get("cas_required") is not True:
                        return False
        owned = {(x["kind"], x["name"]) for x in self.report["resources"]
                 if x["state"] in {"owned", "cleanup_pending"}}
        if self.phase == "cleanup":
            pairs = set()
            for kind, name in owned:
                stem = {"mount": "sys/mounts/", "auth_mount": "sys/auth/", "policy": "sys/policies/acl/"}.get(kind)
                if stem:
                    pairs.update({("GET", "/v1/"+stem+name), ("DELETE", "/v1/"+stem+name)})
                elif kind == "bootstrap_token":
                    pairs.add(("POST", "/v1/auth/token/revoke-self"))
                elif kind == "token" and isinstance(payload, dict):
                    token = self.runtime_token if name == "runtime" else self.negative_token
                    if token and payload == {"token": token}:
                        pairs.add(("POST", "/v1/auth/token/revoke"))
            return (method, path) in pairs
        if self.phase != "setup":
            return False
        if ("mount", self.mounts["kv"]) in owned:
            pairs.update(("POST", "/v1/"+self.mounts["kv"]+"/data/"+self.seed_path(part)) for part in PARTS)
        if ("mount", self.mounts["transit"]) in owned:
            tr = "/v1/"+self.mounts["transit"]+"/"
            pairs.update(("POST", tr+suffix) for suffix in ["config/keys", "keys/cipher", "keys/sign", "keys/mac", "keys/cipher/rotate"])
            pairs.add(("GET", tr+"keys/mac"))
        if ("mount", self.mounts["pki"]) in owned:
            pairs.update(("POST", "/v1/"+self.mounts["pki"]+"/"+suffix) for suffix in ["root/generate/internal", "roles/device"])
        if ("auth_mount", self.auth_mount) in owned:
            role = "/v1/auth/"+self.auth_mount+"/role/device"
            pairs.update({("POST", role), ("GET", role+"/role-id"), ("POST", role+"/secret-id")})
        if all(("policy", name) in owned for name in self.policies.values()):
            pairs.add(("POST", "/v1/auth/token/create"))
        return (method, path) in pairs

    def save(self):
        self.sequence += 1
        path = self.directory / f"checkpoint-{self.sequence:03d}.json"
        try:
            with path.open("x", encoding="utf-8", newline="\n") as out:
                json.dump(self.report, out, indent=2)
                out.write("\n")
        except Exception:
            if self.mutation_started:
                self.unsafe_cleanup = True
            raise

    def verify_server(self):
        response = self.step("server_contract", "GET", "/v1/sys/health", expected=(200,), mutation=False)
        if not isinstance(response, dict) or response.get("version") not in {"2.6.2", "2.6.3"} or response.get("initialized") is not True or response.get("sealed") is not False:
            raise StopRun("server_contract_unverified")
        self.report["server_version"] = response["version"]
        self.save()

    def bootstrap_approle(self, mount, role_id, secret_id):
        validate_approle_input(mount, role_id, secret_id, self.plan)
        if self.report["admin_requests"] + 1 + sum(PART_REQUEST_LIMIT.values()) + CLEANUP_REQUEST_RESERVE > self.limit:
            raise StopRun("total_request_reserve_insufficient")
        self.active()
        item = self.register_resource("bootstrap_token", "bootstrap")
        self.report["admin_requests"] += 1
        self.save()
        body = json.dumps({"role_id": role_id, "secret_id": secret_id}, separators=(",", ":")).encode("utf-8")
        headers = {"Accept": "application/json", "Content-Type": "application/json",
                   "X-Vault-Namespace": "sdk-test"}
        connection = http.client.HTTPSConnection("kms.jiup9.com", 443, timeout=5,
                                                 context=ssl.create_default_context())
        try:
            self.mutation_started = True
            connection.request("POST", "/v1/" + mount + "/login", body=body, headers=headers)
            response = connection.getresponse()
            status, raw = response.status, response.read(65537)
        except Exception:
            self.unsafe_cleanup = True
            self.report["request_count_status"] = "UNKNOWN"
            self.report["uncertain_admin_requests"] = 1
            self.report["steps"].append({"name": "approle_bootstrap", "status": "FAIL"})
            self.save()
            raise StopRun("approle_transport_failed")
        finally:
            connection.close()
        if status != 200 or len(raw) > 65536:
            self.unsafe_cleanup = True
            self.report["steps"].append({"name": "approle_bootstrap", "status": "FAIL", "http_status": status})
            self.save()
            raise StopRun("approle_login_outcome_unknown")
        try:
            token = json.loads(raw).get("auth", {}).get("client_token")
        except (ValueError, AttributeError):
            token = None
        if (not isinstance(token, str) or not 1 <= len(token) <= 4096
                or any(ord(c) < 33 or ord(c) > 126 for c in token)):
            self.unsafe_cleanup = True
            self.report["steps"].append({"name": "approle_bootstrap", "status": "FAIL", "http_status": status})
            self.save()
            raise StopRun("approle_token_invalid")
        self.admin = token
        item["state"] = "owned"
        self.report["steps"].append({"name": "approle_bootstrap", "status": "PASS", "http_status": status})
        self.save()

    def call(self, method, path, payload=None, *, mutation=False):
        self.active()
        if not self.allowed_call(method, path, payload):
            raise StopRun("operation_outside_manifest")
        if not path.startswith("/v1/") or ".." in path or "//" in path or "?" in path:
            raise StopRun("invalid_exact_path")
        if self.report["admin_requests"] + self.child_requests >= self.limit:
            raise StopRun("request_budget_exhausted")
        if self.phase == "setup" and self.report["admin_requests"] + 1 + sum(PART_REQUEST_LIMIT.values()) + CLEANUP_REQUEST_RESERVE > self.limit:
            raise StopRun("setup_cleanup_reserve_insufficient")
        body = None if payload is None else json.dumps(payload, separators=(",", ":")).encode("utf-8")
        headers = {"Accept": "application/json", "X-Vault-Namespace": "sdk-test"}
        if self.admin:
            headers["X-Vault-Token"] = self.admin
        if body is not None:
            headers["Content-Type"] = "application/json"
        connection = http.client.HTTPSConnection("kms.jiup9.com", 443, timeout=5, context=ssl.create_default_context())
        self.report["admin_requests"] += 1
        self.save()
        try:
            if mutation:
                self.mutation_started = True
            connection.request(method, path, body=body, headers=headers)
            response = connection.getresponse()
            data = response.read(65537)
            status = response.status
        except Exception:
            self.report["request_count_status"] = "UNKNOWN"
            self.report["uncertain_admin_requests"] = self.report.get("uncertain_admin_requests", 0) + 1
            if mutation:
                self.unsafe_cleanup = True
                raise StopRun("mutation_outcome_unknown")
            raise StopRun("read_request_failed")
        finally:
            connection.close()
        if len(data) > 65536:
            if mutation:
                self.unsafe_cleanup = True
            raise StopRun("response_too_large")
        if 300 <= status < 400:
            if mutation:
                self.unsafe_cleanup = True
            raise StopRun("redirect_blocked")
        if mutation and status >= 500:
            self.unsafe_cleanup = True
            raise StopRun("mutation_http_outcome_unknown")
        try:
            parsed = json.loads(data) if data else {}
        except (ValueError, UnicodeDecodeError):
            parsed = None
        return status, parsed

    def step(self, label, method, path, payload=None, *, expected=(200, 204), mutation=True):
        entry = {"name": label, "status": "PENDING"}
        self.report["steps"].append(entry)
        self.save()
        status, parsed = self.call(method, path, payload, mutation=mutation)
        entry["http_status"] = status
        entry["status"] = "PASS" if status in expected else "FAIL"
        self.save()
        if status not in expected:
            collision = (status == 409 and (path.startswith("/v1/sys/mounts/") or path.startswith("/v1/sys/auth/")))
            policy_collision = (status == 400 and path.startswith("/v1/sys/policies/acl/")
                                and parsed == {"errors": ["check-and-set parameter set to -1 on existing entry"]})
            if mutation and method == "POST" and (collision or policy_collision):
                raise StopRun("definite_rejection_" + label)
            if mutation:
                self.unsafe_cleanup = True
            raise StopRun(label + "_rejected")
        return parsed

    def own(self, kind, label, method, path, payload=None):
        item = self.register_resource(kind, label)
        try:
            self.step("create_" + label, method, path, payload)
        except StopRun as exc:
            if exc.code.startswith("definite_rejection_") or exc.code in {"invalid_exact_path", "request_budget_exhausted", "time_budget_exhausted", "setup_cleanup_reserve_insufficient"}:
                item["state"] = "not_created"
                self.save()
            raise
        if kind in {"mount", "auth_mount", "policy"}:
            self.record_identity(item)
        item["state"] = "owned"
        self.save()

    def read_identity(self, item):
        kind, name = item["kind"], item["name"]
        path = "/v1/sys/mounts/" + name if kind == "mount" else "/v1/sys/auth/" + name if kind == "auth_mount" else "/v1/sys/policies/acl/" + name
        status, response = self.call("GET", path)
        data = response.get("data", response) if isinstance(response, dict) else None
        if status != 200 or not isinstance(data, dict):
            raise StopRun("resource_identity_unavailable")
        if kind == "policy":
            value = data.get("policy")
            if not isinstance(value, str) or not value or len(value) > 16384:
                raise StopRun("resource_identity_invalid")
            version, modified = data.get("version"), data.get("modified")
            if (type(version) is not int or version < 1 or not isinstance(modified, str)
                    or not modified or data.get("cas_required") is not True):
                raise StopRun("policy_cas_identity_unavailable")
            value = json.dumps({"policy": value, "version": version, "modified": modified},
                               sort_keys=True, separators=(",", ":"))
        else:
            value = data.get("accessor")
            expected_type = "approle" if kind == "auth_mount" else next((k for k, v in self.mounts.items() if v == name), None)
            if data.get("type") != expected_type:
                raise StopRun("resource_type_mismatch")
        if not isinstance(value, str) or not value or len(value) > 16384:
            raise StopRun("resource_identity_invalid")
        return hashlib.sha256(value.encode("utf-8")).hexdigest()

    def record_identity(self, item):
        try:
            item["identity_sha256"] = self.read_identity(item)
            self.save()
        except StopRun:
            self.unsafe_cleanup = True
            raise

    def precheck(self):
        if self.limit < 120:
            raise StopRun("total_request_reserve_insufficient")
        if any((ROOT / ".artifacts" / "runs" / (self.run_id + "-" + part)).exists() for part in PARTS):
            raise StopRun("remote_run_id_already_exists")
        identity = self.step("admin_identity", "GET", "/v1/auth/token/lookup-self", mutation=False)
        data = identity.get("data") if isinstance(identity, dict) else None
        if not isinstance(data, dict) or type(data.get("ttl")) is not int or type(data.get("num_uses")) is not int:
            raise StopRun("admin_budget_unknown")
        ttl, uses = data["ttl"], data["num_uses"]
        if ttl < 0 or uses < 0 or (ttl != 0 and ttl < self.plan.remaining_seconds()+30) or (uses != 0 and uses < 50):
            raise StopRun("admin_budget_insufficient")
        targets = ["sys/mounts/" + name for name in self.mounts.values()]
        targets += ["sys/auth/" + self.auth_mount]
        targets += ["sys/policies/acl/" + name for name in self.policies.values()]
        targets += ["auth/token/create", "auth/token/revoke"]
        current = self.step("current_exact_capabilities", "POST", "/v1/sys/capabilities-self",
                            {"paths": targets}, mutation=False)
        caps = current.get("data") if isinstance(current, dict) else None
        if not isinstance(caps, dict):
            raise StopRun("current_capabilities_invalid")
        for path in targets:
            granted = caps.get(path)
            if not isinstance(granted, list) or not ("root" in granted or
                    ("sudo" in granted and "read" in granted and "delete" in granted and ("create" in granted or "update" in granted))):
                raise StopRun("current_capabilities_insufficient")

    def runtime_policy(self):
        kv, tr, pki = self.mounts["kv"], self.mounts["transit"], self.mounts["pki"]
        folder = WRITE_PREFIX + "/" + self.run_id + "-core"
        rules = [
            ('auth/token/lookup-self', 'read'), ('auth/token/renew-self', 'update'),
            ('sys/capabilities-self', 'update'),
            (f'{kv}/data/{self.seed_path("core")}', 'read'),
            (f'{kv}/data/{self.seed_path("transit")}', 'read'),
            (f'{kv}/data/{folder}/kv', 'create", "update", "read'),
            (f'{kv}/metadata/{folder}', 'list'),
            (f'{kv}/metadata/{folder}/*', 'read", "list", "delete'),
            (f'{kv}/delete/{folder}/kv', 'update'),
            (f'{kv}/undelete/{folder}/kv', 'update'),
            (f'{tr}/keys/{KEYS["cipher"]}', 'read'),
            (f'{tr}/keys/{KEYS["sign"]}', 'read'),
            (f'{tr}/keys/{KEYS["mac"]}', 'read'),
            (f'{tr}/encrypt/{KEYS["cipher"]}', 'update'),
            (f'{tr}/decrypt/{KEYS["cipher"]}', 'update'),
            (f'{tr}/rewrap/{KEYS["cipher"]}', 'update'),
            (f'{tr}/sign/{KEYS["sign"]}', 'update'),
            (f'{tr}/verify/{KEYS["sign"]}', 'update'),
            (f'{tr}/hmac/{KEYS["mac"]}/sha2-256', 'update'),
            (f'{tr}/verify/{KEYS["mac"]}/sha2-256', 'update'),
            (f'{pki}/sign/{ROLE}', 'update'),
            (f'{pki}/issue/{ROLE}', 'update'),
            (f'{pki}/cert/*', 'read'),
            (f'{pki}/ca_chain', 'read'),
            (f'{pki}/revoke', 'update'),
        ]
        return "\n".join(f'path "{path}" {{ capabilities = ["{caps}"] }}' for path, caps in rules)

    def setup(self):
        self.precheck()
        for kind, name in self.mounts.items():
            payload = {"type": kind, "options": {"version": "2"}} if kind == "kv" else {"type": kind}
            self.own("mount", name, "POST", "/v1/sys/mounts/" + name, payload)
        self.own("auth_mount", self.auth_mount, "POST", "/v1/sys/auth/" + self.auth_mount, {"type": "approle"})
        pki = self.mounts["pki"]
        ca = self.step("create_private_test_ca", "POST", f"/v1/{pki}/root/generate/internal",
                       {"common_name": "SDK isolated test CA " + self.prefix, "ttl": "1h", "key_type": "ec", "key_bits": 256})
        pem = ca.get("data", {}).get("certificate") if isinstance(ca, dict) else None
        if not isinstance(pem, str) or not pem.startswith("-----BEGIN CERTIFICATE-----") or len(pem) > 8192:
            raise StopRun("test_ca_response_invalid")
        with (self.directory / "test-ca.pem").open("x", encoding="utf-8", newline="\n") as out:
            out.write(pem)
        self.step("create_test_pki_role", "POST", f"/v1/{pki}/roles/{ROLE}",
                  {"allowed_domains": ["sdk-test.invalid"], "allow_bare_domains": False, "allow_subdomains": True,
                   "client_flag": False, "server_flag": True, "key_type": "ec", "key_bits": 256, "max_ttl": "10m", "ttl": "5m"})
        tr = self.mounts["transit"]
        self.step("disable_transit_upsert", "POST", f"/v1/{tr}/config/keys", {"disable_upsert": True})
        for label, kind in (("cipher", "aes256-gcm96"), ("sign", "ecdsa-p256"), ("mac", "hmac")):
            args = {"type": kind, "derived": False, "exportable": False, "allow_plaintext_backup": False}
            if kind == "hmac": args["key_size"] = 32
            self.step("create_test_" + label + "_key", "POST", f"/v1/{tr}/keys/{KEYS[label]}", args)
        self.step("rotate_test_cipher_key", "POST", f"/v1/{tr}/keys/{KEYS['cipher']}/rotate", {})
        for part in PARTS:
            self.step("seed_owned_kv_" + part, "POST", f"/v1/{self.mounts['kv']}/data/{self.seed_path(part)}",
                      {"options": {"cas": 0}, "data": {"sdk_test_marker": ("owned-" + self.run_id)}})
        for label, name in self.policies.items():
            rules = self.runtime_policy() if label == "runtime" else 'path "auth/token/lookup-self" { capabilities = ["read"] }'
            # OpenBao's ACL policy API uses cas=-1 for atomic create-only.
            self.own("policy", name, "POST", "/v1/sys/policies/acl/" + name,
                     {"policy": rules, "cas": -1, "cas_required": True})
        self.step("create_approle_role", "POST", f"/v1/auth/{self.auth_mount}/role/{ROLE}",
                  {"token_policies": [self.policies["runtime"]], "token_no_default_policy": True,
                   "token_ttl": "15m", "token_max_ttl": "30m", "token_num_uses": 0,
                   "secret_id_ttl": "30m", "secret_id_num_uses": 0})
        role = self.step("read_approle_role_id", "GET", f"/v1/auth/{self.auth_mount}/role/{ROLE}/role-id", mutation=False)
        secret = self.step("create_approle_secret_id", "POST", f"/v1/auth/{self.auth_mount}/role/{ROLE}/secret-id", {})
        self.role_id = role.get("data", {}).get("role_id") if isinstance(role, dict) else None
        self.secret_id = secret.get("data", {}).get("secret_id") if isinstance(secret, dict) else None
        if not self.role_id or not self.secret_id:
            raise StopRun("approle_credentials_missing")
        for label in ("runtime", "negative"):
            item = self.register_resource("token", label)
            try:
                created = self.step("create_" + label + "_token", "POST", "/v1/auth/token/create",
                                    {"policies": [self.policies[label]], "no_default_policy": True,
                                     "ttl": "20m", "num_uses": 0})
            except StopRun as exc:
                if exc.code.startswith("definite_rejection_"):
                    item["state"] = "not_created"
                    self.save()
                raise
            token = created.get("auth", {}).get("client_token") if isinstance(created, dict) else None
            if not isinstance(token, str) or not token:
                raise StopRun("test_token_response_invalid")
            if label == "runtime": self.runtime_token = token
            else: self.negative_token = token
            item["state"] = "owned"
            self.save()

    def run_sdk(self):
        if self.report["admin_requests"] + sum(PART_REQUEST_LIMIT.values()) + CLEANUP_REQUEST_RESERVE > self.limit:
            raise StopRun("total_request_reserve_insufficient")
        allowed = {'PATH', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'PATHEXT', 'TEMP', 'TMP',
                   'HOME', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA', 'GOCACHE',
                   'GOPATH', 'GOMODCACHE', 'GOROOT', 'CC', 'CXX', 'CGO_ENABLED'}
        base_env = {key: value for key, value in os.environ.items() if key.upper() in allowed}
        self.report["sdk_status"] = "RUNNING"
        self.save()
        self.phase = "sdk"
        for index, part in enumerate(PARTS):
            future = sum(PART_REQUEST_LIMIT[name] for name in PARTS[index:])
            if self.report["admin_requests"] + self.child_requests + future + CLEANUP_REQUEST_RESERVE > self.limit:
                raise StopRun("total_request_reserve_insufficient")
            self.active()
            remaining_time = self.plan.remaining_seconds() - 60
            if remaining_time <= 0:
                raise StopRun("sdk_cleanup_time_reserve_insufficient")
            run_id = self.run_id + "-" + part
            env = dict(base_env)
            env.update({
                "BAO_REMOTE_ADDRESS": "https://kms.jiup9.com:443", "BAO_REMOTE_CONFIRMED": "yes",
                "BAO_REMOTE_ENVIRONMENT": "nonproduction", "BAO_REMOTE_NAMESPACE_MODE": "named",
                "BAO_REMOTE_NAMESPACE": "sdk-test", "BAO_REMOTE_TOKEN": self.runtime_token,
                "BAO_REMOTE_KV_MOUNT": self.mounts["kv"], "BAO_REMOTE_KV_PATH": self.seed_path(part),
                "BAO_REMOTE_KV_VERSION": "1", "BAO_REMOTE_KV_MARKER": ("owned-" + self.run_id),
                "BAO_REMOTE_WRITE_PREFIX": WRITE_PREFIX, "BAO_REMOTE_ALLOW_WRITE": "yes",
                "BAO_REMOTE_ALLOW_CLEANUP": "yes", "BAO_REMOTE_ALLOW_SOFT_DELETE": "yes",
                "BAO_REMOTE_NEGATIVE_TOKEN": self.negative_token, "BAO_REMOTE_NEGATIVE_MOUNT": self.mounts["kv"],
                "BAO_REMOTE_NEGATIVE_PATH": self.seed_path(part),
                "BAO_REMOTE_TRANSIT_MOUNT": self.mounts["transit"], "BAO_REMOTE_TRANSIT_KEY": KEYS["cipher"],
                "BAO_REMOTE_TRANSIT_TYPE": "aes256-gcm96", "BAO_REMOTE_TRANSIT_SIGN_KEY": KEYS["sign"],
                "BAO_REMOTE_TRANSIT_HMAC_KEY": KEYS["mac"],
                "BAO_REMOTE_PKI_MOUNT": self.mounts["pki"], "BAO_REMOTE_PKI_ROLE": ROLE,
                "BAO_REMOTE_PKI_DNS": DNS, "BAO_REMOTE_PKI_ROOT_FILE": str(self.directory / "test-ca.pem"),
                "BAO_REMOTE_ALLOW_SIGN_CSR": "yes", "BAO_REMOTE_ALLOW_REVOKE": "yes",
                "BAO_REMOTE_APPROLE_MOUNT": self.auth_mount, "BAO_REMOTE_APPROLE_ROLE_ID": self.role_id,
                "BAO_REMOTE_APPROLE_SECRET_ID": self.secret_id, "BAO_REMOTE_FULL": "yes",
                "BAO_REMOTE_FULL_PART": part,
                "BAO_REMOTE_MANIFEST": str(self.directory / "manifest.json"),
                "BAO_REMOTE_MANIFEST_SHA256": self.plan.sha256,
            })
            entry = {"part": part, "run_id": run_id, "status": "RUNNING"}
            self.report["sdk_runs"].append(entry)
            self.save()
            try:
                receipt = run_command([sys.executable, "-B", str(ROOT / "scripts" / "remote-test.py"),
                                       "--execute", "--mode", "isolated", "--run-id", run_id],
                                      cwd=ROOT, env=env, log=self.directory / ("sdk-" + part + ".log"),
                                      timeout_s=min(820, remaining_time), discard_output=True)
            except BaseException:
                self.unsafe_cleanup = True
                self.report["request_count_status"] = "UNKNOWN"
                entry["status"] = "UNKNOWN"
                self.save()
                raise
            finally:
                env.clear()
            entry["receipt"] = receipt
            self.save()
            if receipt.get("completion") in {"timeout", "interrupted"} or receipt.get("cleanup_incomplete"):
                self.unsafe_cleanup = True
                self.report["request_count_status"] = "UNKNOWN"
                entry["status"] = "UNKNOWN"
                self.save()
                raise StopRun("sdk_runner_process_outcome_unknown")
            parent = ROOT / ".artifacts" / "runs" / run_id / "remote"
            try:
                report_paths = sorted(parent.glob("report-*.json")) if parent.is_dir() else []
                raw = report_paths[-1].read_bytes() if report_paths else None
            except OSError:
                self.unsafe_cleanup = True
                self.report["request_count_status"] = "UNKNOWN"
                entry["status"] = "UNKNOWN"
                self.save()
                raise StopRun("inner_remote_report_unavailable")
            if report_paths:
                entry["report_path"] = report_paths[-1].relative_to(ROOT).as_posix()
                entry["report_sha256"] = hashlib.sha256(raw).hexdigest()
                try:
                    outer = json.loads(raw)
                except (ValueError, UnicodeDecodeError):
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("inner_remote_report_invalid")
                if (not isinstance(outer, dict) or outer.get("manifest_sha256") != self.plan.sha256
                        or outer.get("run_id") != run_id
                        or outer.get("source_before") != self.report.get("source_before")
                        or outer.get("source_after") != self.report.get("source_before")):
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("inner_manifest_mismatch")
                commands = outer.get("commands") if isinstance(outer, dict) else None
                inner = [x for x in commands if isinstance(x, dict) and x.get("name") == "remote"] if isinstance(commands, list) else []
                if len(inner) != 1 or inner[0].get("completion") != "exited" or inner[0].get("cleanup_incomplete"):
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    entry["status"] = "UNKNOWN"
                    self.save()
                    raise StopRun("inner_remote_process_outcome_unknown")
                entry["inner_completion"] = "exited"
                entry["inner_exit_code"] = inner[0].get("exit_code")
            else:
                self.unsafe_cleanup = True
                self.report["request_count_status"] = "UNKNOWN"
                entry["status"] = "UNKNOWN"
                self.save()
                raise StopRun("inner_remote_receipt_missing")
            result = parent / "result.json"
            if result.is_file():
                try:
                    result_raw = result.read_bytes()
                    content = json.loads(result_raw)
                    if outer.get("status") == "PASS" and outer.get("result_sha256") != hashlib.sha256(result_raw).hexdigest():
                        raise ValueError("result digest mismatch")
                except (ValueError, UnicodeDecodeError, OSError):
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("sdk_result_invalid")
                if not isinstance(content, dict) or content.get("manifest_sha256") != self.plan.sha256:
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("result_manifest_mismatch")
                resources = content.get("resources")
                if not isinstance(resources, list) or any(not isinstance(x, dict) for x in resources):
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("sdk_resource_journal_invalid")
                requests = content.get("requests")
                if type(requests) is not int or not 0 <= requests <= PART_REQUEST_LIMIT[part]:
                    self.unsafe_cleanup = True
                    self.report["request_count_status"] = "UNKNOWN"
                    raise StopRun("sdk_request_count_invalid")
                self.child_requests += requests
                self.report["child_requests"] = self.child_requests
                entry["requests"] = requests
                entry["result_status"] = content.get("status")
                if any(x.get("state") not in {"removed", "revoked_record_retained"} for x in resources):
                    self.unsafe_cleanup = True
                    self.report["sdk_resource_pending"] = True
            else:
                self.unsafe_cleanup = True
                self.report["request_count_status"] = "UNKNOWN"
                entry["result_status"] = "MISSING"
            entry["status"] = "PASS" if succeeded(receipt) and entry["result_status"] == "PASS" else "FAIL"
            self.save()
            if entry["status"] != "PASS":
                raise StopRun("sdk_part_failed")
        self.report["sdk_status"] = "PASS"
        self.save()

    def cleanup(self):
        self.phase = "cleanup"
        try:
            self.active()
        except StopRun:
            self.report["cleanup"] = "BLOCKED_EXPIRED"
            self.save()
            return
        if self.unsafe_cleanup:
            self.report["cleanup"] = "BLOCKED_UNCERTAIN_OUTCOME"
            self.save()
            return
        if any(item["state"] == "creation_pending" for item in self.report["resources"]):
            self.report["cleanup"] = "BLOCKED_UNCERTAIN_OWNERSHIP"
            self.save()
            return
        self.report["cleanup"] = "RUNNING"
        self.save()
        for item in reversed(self.report["resources"]):
            if item["state"] != "owned":
                continue
            kind, name = item["kind"], item["name"]
            if kind == "bootstrap_token":
                method, path, payload = "POST", "/v1/auth/token/revoke-self", None
            elif kind == "token":
                token = self.runtime_token if name == "runtime" else self.negative_token
                method, path, payload = "POST", "/v1/auth/token/revoke", {"token": token}
            elif kind == "policy":
                method, path, payload = "DELETE", "/v1/sys/policies/acl/" + name, None
            elif kind == "auth_mount":
                method, path, payload = "DELETE", "/v1/sys/auth/" + name, None
            else:
                method, path, payload = "DELETE", "/v1/sys/mounts/" + name, None
            if kind in {"mount", "auth_mount", "policy"}:
                try:
                    current_identity = self.read_identity(item)
                except StopRun:
                    item["state"] = "ownership_unconfirmed"
                    self.report["cleanup"] = "INCOMPLETE"
                    self.save()
                    break
                if current_identity != item.get("identity_sha256"):
                    item["state"] = "ownership_changed"
                    self.report["cleanup"] = "INCOMPLETE"
                    self.save()
                    break
            item["state"] = "cleanup_pending"
            self.save()
            try:
                self.step("remove_" + name, method, path, payload)
                item["state"] = "removed"
                if kind == "bootstrap_token":
                    self.admin = None
            except StopRun as exc:
                item["state"] = "cleanup_unknown" if self.unsafe_cleanup else "cleanup_failed"
                self.report["cleanup"] = "INCOMPLETE"
                self.save()
                break
            self.save()
        if self.report["cleanup"] == "RUNNING":
            self.report["cleanup"] = "PASS" if all(x["state"] in {"removed", "not_created"} for x in self.report["resources"]) else "INCOMPLETE"
        self.save()

    def finish(self, reason):
        known_requests = self.report["admin_requests"] + self.child_requests
        if self.report["request_count_status"] == "UNKNOWN":
            self.report["total_requests"] = None
            self.report["request_count_lower_bound"] = max(
                0, known_requests - self.report.get("uncertain_admin_requests", 0))
            self.report["request_count_upper_bound"] = known_requests + sum(
                PART_REQUEST_LIMIT[entry["part"]] for entry in self.report["sdk_runs"]
                if "requests" not in entry)
        else:
            self.report["total_requests"] = known_requests
        self.report["reason"] = reason
        blocked = {"admin_budget_insufficient", "total_request_reserve_insufficient",
                   "setup_cleanup_reserve_insufficient", "capability_preflight_insufficient",
                   "current_capabilities_insufficient", "capability_preflight_mismatch", "server_contract_unverified"}
        if reason == "sdk_pass" and self.report.get("sdk_status") == "PASS" and self.report["cleanup"] == "PASS" and self.report["total_requests"] <= self.limit:
            self.report["status"] = "PASS"
        elif reason in blocked and self.report["cleanup"] == "PASS" and not any(x["state"] not in {"removed", "not_created"} for x in self.report["resources"]):
            self.report["status"] = "BLOCKED"
        else:
            self.report["status"] = "FAIL"
        finish_report(self.report, ROOT)
        with (self.directory / "report.json").open("x", encoding="utf-8", newline="\n") as out:
            json.dump(self.report, out, indent=2)
            out.write("\n")
        self.admin = None
        self.runtime_token = self.negative_token = self.role_id = self.secret_id = None
        return 0 if self.report["status"] == "PASS" else 2 if self.report["status"] == "BLOCKED" else 1


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--execute", action="store_true")
    group.add_argument("--validate-only", action="store_true")
    args = parser.parse_args(argv)
    try:
        plan = load_manifest(args.manifest)
        if not args.execute:
            print("FIXTURE_RESULT: VALID_PLAN (no network; not an execution approval)")
            return 0 if args.validate_only else 2
        role_id = os.environ.pop("BAO_REMOTE_ROLE_ID", "")
        secret_id = os.environ.pop("BAO_REMOTE_SECRET_ID", "")
        mount = os.environ.pop("BAO_REMOTE_APPROLE_MOUNT", "")
        validate_approle_input(mount, role_id, secret_id, plan)
        directory = reserve_directory(ROOT, plan)
    except (ManifestError, StopRun) as exc:
        print("FIXTURE_RESULT: BLOCKED ("+str(exc)+")")
        return 2
    except OSError:
        print("FIXTURE_RESULT: BLOCKED (local evidence unavailable)")
        return 2
    fixture = Fixture(plan, directory)
    reason = "sdk_pass"
    try:
        fixture.verify_server()
        fixture.bootstrap_approle(mount, role_id, secret_id)
        role_id = secret_id = None
        fixture.setup()
        fixture.run_sdk()
        if fixture.report.get("sdk_status") != "PASS":
            reason = "sdk_not_pass"
    except StopRun as exc:
        reason = exc.code
    except Exception:
        reason = "fixture_internal_error"
    finally:
        role_id = secret_id = None
        try:
            fixture.cleanup()
        except Exception:
            fixture.report["cleanup"] = "INCOMPLETE"
            reason = "cleanup_exception"
        code = fixture.finish(reason)
    print("FIXTURE_RESULT: "+fixture.report["status"])
    return code


if __name__ == "__main__":
    sys.exit(main())
