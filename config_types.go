// Package bao contains the public SDK contract types.
package bao

import (
	"git.example.com/infra/openbao-sdk-go/auth"
	"git.example.com/infra/openbao-sdk-go/sensitive"
	"time"
)

type Config struct {
	Address      string
	ClusterAlias string
	Namespace    NamespaceConfig
	Auth         auth.Config
	TLS          TLSConfig
	Network      NetworkConfig
	Timeouts     TimeoutConfig
	Limits       LimitConfig
	ReadRetry    ReadRetryConfig
}

type NamespaceMode string

const (
	NamespaceRoot  NamespaceMode = "root"
	NamespaceNamed NamespaceMode = "named"
)

type NamespaceConfig struct {
	Mode NamespaceMode // 必填，空值错误。
	Path string        // named 必填；root 必须为空。
}

type TLSConfig struct {
	CAFile         string
	CAPEM          []byte
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string
	ClientCertPEM  []byte
	ClientKeyPEM   sensitive.Bytes
	MinVersion     uint16
}

type NetworkConfig struct {
	ProxyURL           string // 默认不使用环境代理。
	MaxIdleConnections int
	MaxIdlePerHost     int
	IdleConnTimeout    time.Duration
}

type TimeoutConfig struct {
	Request      time.Duration // 默认 10 秒。
	PKIIssue     time.Duration // 默认 30 秒，覆盖 Issue/SignCSR。
	Login        time.Duration // 默认 10 秒。
	Renew        time.Duration // 默认 5 秒。
	Dial         time.Duration // 默认 5 秒。
	TLSHandshake time.Duration // 默认 5 秒。
}

type LimitConfig struct {
	MaxConcurrentRequests  int   // 默认 32。
	MaxRequestBytes        int64 // 默认 1 MiB。
	MaxResponseBytes       int64 // 默认 2 MiB，解压后。
	MaxResponseHeaderBytes int64 // 默认 64 KiB。
}

type ReadRetryConfig struct {
	MaxAttempts int           // 默认 3；1 表示禁止外层重试。
	BaseDelay   time.Duration // 默认 100ms。
	MaxDelay    time.Duration // 默认 2s。
}
