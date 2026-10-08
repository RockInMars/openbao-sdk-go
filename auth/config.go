// Package auth contains the public SDK contract types.
package auth

import (
	"context"
	"github.com/RockInMars/openbao-sdk-go/sensitive"
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

// TokenSnapshot contains one credential observation. Its erasure ownership is
// defined by the provider, not by copying this struct or its sensitive.Bytes.
type TokenSnapshot struct {
	Token      sensitive.Bytes
	Generation string    // 本地非秘密标识，不记录 Token 内容。
	ValidUntil time.Time // 零值表示未知，不表示永久有效。
}

// TokenProvider supplies credentials without transferring erasure ownership by
// default: the SDK borrows Token, clears its own revealed copy, and never Zeroes
// an unknown provider's handle. The provider controls the lifetime and concurrent
// use of any shared value.
//
// A provider may explicitly transfer every returned snapshot, including a value
// returned together with an error, by also implementing
// SnapshotOwnedByConsumer(actual any) bool. The SDK passes the actual provider and
// queries this method before Snapshot, inside the provider panic boundary. A true
// result permits Zero after consumption; each snapshot must then be independent
// of the provider and all other calls. The query must not block. An embedding
// wrapper must not claim ownership of a different Snapshot implementation.
// Built-in providers require exact instance identity before returning true.
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

// SecretIDSnapshot describes a login credential and its single-use generation.
// Its handle follows the provider's ownership protocol, as for TokenSnapshot.
type SecretIDSnapshot struct {
	SecretID   sensitive.Bytes
	Generation string
	Use        SecretIDUse
}

// SecretIDProvider supplies AppRole credentials. Unknown providers retain their
// handles; the optional SnapshotOwnedByConsumer protocol documented on
// TokenProvider can transfer each independent snapshot to the consumer instead.
type SecretIDProvider interface {
	Current(ctx context.Context) (SecretIDSnapshot, error)
}
