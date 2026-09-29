# Separate test-only identity, not normal runtime permission.
path "pki/revoke" { capabilities = ["update"] }
