// Package kv contains the public SDK contract types.
package kv

import (
	"time"
)

// String、GoString、Format、LogValue 脱敏；MarshalJSON 返回显式揭示错误。

type Ref struct {
	ClusterAlias string `json:"cluster_alias"`
	Namespace    string `json:"namespace"`
	Mount        string `json:"mount"`
	Path         string `json:"path"`
	Version      int    `json:"version"`
}

type WriteResult struct {
	Ref       Ref
	CreatedAt time.Time
	RequestID string
	Attempts  int
}
type ReadResult struct {
	Ref       Ref
	Data      Document
	CreatedAt time.Time
	RequestID string
	Attempts  int
}
type VersionMetadata struct {
	Version   int
	CreatedAt time.Time
	DeletedAt *time.Time // Raw deletion_time; it may be a future auto-delete deadline.
	Destroyed bool
}
type Metadata struct {
	CurrentVersion     int
	OldestVersion      int
	MaxVersions        int
	CASRequired        bool
	DeleteVersionAfter time.Duration
	CustomMetadata     map[string]string // 键级信息，不是版本快照。
	Versions           []VersionMetadata // 按版本升序。
	RequestID          string
}
type ListEntry struct {
	Name     string
	IsFolder bool
}
type ListResult struct {
	Entries   []ListEntry
	RequestID string
}
