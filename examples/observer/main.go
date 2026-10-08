// This example counts a single, explicitly versioned read without exporting data.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	bao "github.com/RockInMars/openbao-sdk-go"
	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/examples/internal/bootstrap"
	"github.com/RockInMars/openbao-sdk-go/observe"
)

// The callback has six fixed buckets, no I/O, and no unbounded label storage.
type counters struct{ values [2][3]atomic.Uint64 }

func (c *counters) Observe(_ context.Context, event observe.Event) {
	op, result := 1, 2
	if event.Operation == "KV_READ_VERSION" {
		op = 0
	}
	if event.ErrorCode == "" {
		result = 0
	} else if event.ErrorCode == string(baoerr.CodePermissionDenied) {
		result = 1
	}
	c.values[op][result].Add(1)
}

// Output happens after the operation, outside the synchronous callback.
func (c *counters) writeTo(w io.Writer) error {
	for op, label := range []string{"kv_read_version", "other"} {
		for result, code := range []string{"success", "permission_denied", "error"} {
			if _, err := fmt.Fprintf(w, "operation=%s result=%s count=%d\n", label, code, c.values[op][result].Load()); err != nil {
				return err
			}
		}
	}
	return nil
}

func readVersion(ctx context.Context, c *bao.Client, mount, path string, version int) error {
	if mount == "" || path == "" || version <= 0 {
		return errors.New("explicit mount, path and positive version are required")
	}
	k, err := c.KVv2(mount)
	if err != nil {
		return err
	}
	r, err := k.ReadVersion(ctx, path, version)
	if err != nil {
		return err
	}
	defer r.Data.Zero()
	return nil
}

func main() {
	mount := flag.String("mount", "", "explicit KV v2 mount")
	path := flag.String("path", "", "explicit secret path (never used as a metric label)")
	version := flag.Int("version", 0, "positive, explicit secret version")
	flag.Parse()
	c := &counters{}
	err := bootstrap.Run(func(ctx context.Context, client *bao.Client) error {
		return readVersion(ctx, client, *mount, *path, *version)
	}, bao.WithObserver(c))
	outputErr := c.writeTo(os.Stdout)
	if err != nil || outputErr != nil {
		fmt.Fprintln(os.Stderr, "observer example failed; secret data omitted")
		os.Exit(1)
	}
}
