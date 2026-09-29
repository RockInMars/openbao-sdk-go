// Package auth contains the public SDK contract types.
package auth

import (
	"context"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"time"
)

type Mode string

const (
	ExternalToken  Mode = "external_token"
	ManagedAppRole Mode = "managed_approle"
)

type Config struct {
	Mode          Mode
	TokenProvider TokenProvider  // ExternalToken 必填。
	AppRole       *AppRoleConfig // ManagedAppRole 必填，与 TokenProvider 互斥。
}

type TokenSnapshot struct {
	Token      sensitive.Bytes
	Generation string    // 本地非秘密标识，不记录 Token 内容。
	ValidUntil time.Time // 零值表示未知，不表示永久有效。
}

type TokenProvider interface {
	Snapshot(ctx context.Context) (TokenSnapshot, error)
}

type AppRoleConfig struct {
	Mount            string // 相对 auth/ 的挂载，如 approle。
	RoleID           sensitive.Bytes
	SecretIDProvider SecretIDProvider
}

type SecretIDUse string

const (
	ReusableSecretID  SecretIDUse = "reusable"
	SingleUseSecretID SecretIDUse = "single_use"
)

type SecretIDSnapshot struct {
	SecretID   sensitive.Bytes
	Generation string
	Use        SecretIDUse
}
type SecretIDProvider interface {
	Current(ctx context.Context) (SecretIDSnapshot, error)
}
