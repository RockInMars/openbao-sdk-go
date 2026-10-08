package testenv

import "strings"

func runtimePolicy() string {
	return `
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
`
}
func controlPolicy() string {
	var b strings.Builder
	b.WriteString("path \"auth/token/renew-self\" { capabilities = [\"update\"] }\n")
	for _, key := range []string{"ecdsa", "rsa", "ed25519", "cipher", "derived", "mac"} {
		b.WriteString("path \"transit/keys/" + key + "\" { capabilities = [\"read\"] }\n")
	}
	for _, key := range []string{"ecdsa", "rsa", "ed25519"} {
		for _, op := range []string{"sign", "verify"} {
			b.WriteString("path \"transit/" + op + "/" + key + "\" { capabilities = [\"update\"] }\n")
		}
	}
	for _, key := range []string{"cipher", "derived"} {
		for _, op := range []string{"encrypt", "decrypt", "rewrap"} {
			b.WriteString("path \"transit/" + op + "/" + key + "\" { capabilities = [\"update\"] }\n")
		}
	}
	b.WriteString("path \"transit/hmac/mac/sha2-256\" { capabilities = [\"update\"] }\n")
	b.WriteString("path \"transit/verify/mac/sha2-256\" { capabilities = [\"update\"] }\n")
	return b.String()
}
