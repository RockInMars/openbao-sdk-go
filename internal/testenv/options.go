// Package testenv launches a NEW, disposable, loopback-only OpenBao cluster.
// It has no API for attaching to an existing server. It is test infrastructure,
// not part of the SDK's runtime or administration surface.
package testenv

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Options struct{ Version, BinaryPath, BinarySHA256, Image string }

var versionPattern = regexp.MustCompile(`^2\.[0-9]+\.[0-9]+$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[a-f0-9]{64}$`)
var errOptions = errors.New("integration requires an exact OpenBao version and one verified absolute binary or digest-pinned image; no external servers")

func OptionsFromEnv() (Options, error) {
	if os.Getenv("BAO_TEST_ADDRESS") != "" || os.Getenv("BAO_TEST_TOKEN") != "" {
		return Options{}, errOptions
	}
	o := Options{Version: os.Getenv("BAO_TEST_VERSION"), BinaryPath: os.Getenv("BAO_TEST_BINARY"), BinarySHA256: os.Getenv("BAO_TEST_BINARY_SHA256"), Image: os.Getenv("BAO_TEST_IMAGE")}
	return o, o.Validate()
}
func (o Options) Validate() error {
	if !versionPattern.MatchString(o.Version) || (o.BinaryPath == "") == (o.Image == "") {
		return errOptions
	}
	if o.Image != "" {
		if !imagePattern.MatchString(o.Image) || o.BinarySHA256 != "" {
			return errOptions
		}
		return nil
	}
	if !filepath.IsAbs(o.BinaryPath) || !digestPattern.MatchString(o.BinarySHA256) {
		return errOptions
	}
	f, e := os.Open(o.BinaryPath)
	if e != nil {
		return errOptions
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil || !s.Mode().IsRegular() || s.Mode()&0111 == 0 {
		return errOptions
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return errOptions
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), o.BinarySHA256) {
		return errOptions
	}
	return nil
}
