# Reference fixture; canonical generator: internal/testenv/policies.go

path "kv/data/fixture/*" { capabilities = ["create", "update", "read"] }
path "kv/metadata/fixture" { capabilities = ["list"] }
path "kv/metadata/fixture/*" { capabilities = ["read", "list"] }
path "kv/delete/fixture/*" { capabilities = ["update"] }
path "kv/undelete/fixture/*" { capabilities = ["update"] }
path "pki/issue/device" { capabilities = ["update"] }
path "pki/sign/device" { capabilities = ["update"] }
path "pki/cert/*" { capabilities = ["read"] }
path "pki/ca_chain" { capabilities = ["read"] }
path "auth/token/renew-self" { capabilities = ["update"] }
