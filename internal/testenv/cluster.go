package testenv

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Secrets are unexported; these test-only types never reveal through formatting.
// Credential fields are explicitly copied for test adapters, never printed.
type Identity struct{ Token, RoleID, SecretID []byte }

func (Identity) String() string             { return "[TEST_IDENTITY_REDACTED]" }
func (Identity) GoString() string           { return "[TEST_IDENTITY_REDACTED]" }
func (Identity) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[TEST_IDENTITY_REDACTED]") }

type Space struct {
	Namespace                                    string
	Provision, Control, Maintenance, Replacement Identity
	IssuerCA                                     []byte
}

func (Space) String() string             { return "[TEST_SPACE_REDACTED]" }
func (Space) GoString() string           { return "[TEST_SPACE_REDACTED]" }
func (Space) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[TEST_SPACE_REDACTED]") }

type Cluster struct {
	Address    string
	CAPEM      []byte
	Spaces     []Space
	Version    string
	dir        string
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	exited     chan struct{}
	dockerName string
	http       *http.Client
	once       sync.Once
}

func (*Cluster) String() string             { return "[ISOLATED_TEST_CLUSTER]" }
func (*Cluster) GoString() string           { return "[ISOLATED_TEST_CLUSTER]" }
func (*Cluster) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[ISOLATED_TEST_CLUSTER]") }

var errFixture = errors.New("isolated OpenBao fixture failed; raw server output suppressed to protect test credentials")

// Launch verifies a pinned executable/image, creates new storage and a new CA,
// binds only loopback on the host, and initializes only that owned instance.
func Launch(ctx context.Context, o Options) (c *Cluster, err error) {
	if ctx == nil {
		return nil, errOptions
	}
	if err = o.Validate(); err != nil {
		return nil, err
	}
	dir, e := os.MkdirTemp("", "bao-sdk-isolated-")
	if e != nil {
		return nil, errFixture
	}
	life, cancel := context.WithCancel(ctx)
	c = &Cluster{dir: dir, cancel: cancel, exited: make(chan struct{}), Version: o.Version}
	defer func() {
		if err != nil {
			c.Close()
			c = nil
		}
	}()
	if err = os.Chmod(dir, 0700); err != nil {
		return c, errFixture
	}
	ca, cert, key, e := tlsFixture()
	if e != nil {
		return c, errFixture
	}
	c.CAPEM = ca
	for name, data := range map[string][]byte{"ca.pem": ca, "server.pem": cert, "server.key": key} {
		if e = os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
			return c, errFixture
		}
	}
	clear(key)
	p, e := freePort()
	if e != nil {
		return c, errFixture
	}
	c.Address = fmt.Sprintf("https://127.0.0.1:%d", p)
	bind := fmt.Sprintf("127.0.0.1:%d", p)
	prefix := dir
	if o.Image != "" {
		bind = "0.0.0.0:8200"
		prefix = "/fixture"
	}
	conf := fmt.Sprintf("disable_mlock = true\nui = false\napi_addr = %q\nstorage \"inmem\" {}\nlistener \"tcp\" {\n address = %q\n tls_cert_file = %q\n tls_key_file = %q\n tls_min_version = \"tls12\"\n}\n", c.Address, bind, filepath.Join(prefix, "server.pem"), filepath.Join(prefix, "server.key"))
	if e = os.WriteFile(filepath.Join(dir, "config.hcl"), []byte(conf), 0600); e != nil {
		return c, errFixture
	}
	if o.Image != "" {
		if _, e = exec.LookPath("docker"); e != nil {
			return c, errOptions
		}
		c.dockerName = filepath.Base(dir)
		// Explicit local daemon; never honor a remote DOCKER_HOST/context.
		c.cmd = exec.CommandContext(life, "docker", "--host", "unix:///var/run/docker.sock", "run", "--rm", "--name", c.dockerName, "--user", "0:0", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--publish", fmt.Sprintf("127.0.0.1:%d:8200", p), "--mount", "type=bind,src="+dir+",dst=/fixture,readonly", "--entrypoint", "bao", o.Image, "server", "-config=/fixture/config.hcl")
	} else {
		vctx, vcancel := context.WithTimeout(life, 10*time.Second)
		out, e := exec.CommandContext(vctx, o.BinaryPath, "version").Output()
		vcancel()
		if e != nil || !strings.Contains(string(out), "OpenBao v"+o.Version+" ") && !strings.HasSuffix(strings.TrimSpace(string(out)), "OpenBao v"+o.Version) {
			return c, errOptions
		}
		c.cmd = exec.CommandContext(life, o.BinaryPath, "server", "-config="+filepath.Join(dir, "config.hcl"))
	}
	c.cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "TMPDIR=" + dir}
	c.cmd.Stdout = io.Discard
	c.cmd.Stderr = io.Discard
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return c, errFixture
	}
	c.http = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if e = c.cmd.Start(); e != nil {
		c.cmd = nil
		return c, errFixture
	}
	go func() { _ = c.cmd.Wait(); close(c.exited) }()
	ready, cancelReady := context.WithTimeout(life, 30*time.Second)
	defer cancelReady()
	for {
		var init struct {
			Initialized bool `json:"initialized"`
		}
		if e = c.call(ready, "GET", "", "sys/init", nil, nil, &init); e == nil {
			if init.Initialized {
				return c, errFixture
			}
			break
		}
		select {
		case <-ready.Done():
			return c, errFixture
		case <-c.exited:
			return c, errFixture
		case <-time.After(100 * time.Millisecond):
		}
	}
	var init struct {
		RootToken string   `json:"root_token"`
		Keys      []string `json:"keys_base64"`
	}
	if e = c.call(life, "POST", "", "sys/init", nil, map[string]any{"secret_shares": 1, "secret_threshold": 1}, &init); e != nil || init.RootToken == "" || len(init.Keys) != 1 {
		return c, errFixture
	}
	admin := []byte(init.RootToken)
	defer clear(admin)
	if e = c.call(life, "PUT", "", "sys/unseal", nil, map[string]any{"key": init.Keys[0]}, nil); e != nil {
		return c, errFixture
	}
	// Version is checked from this new TLS-authenticated instance, not an env label.
	var health struct {
		Version string `json:"version"`
	}
	if e = c.call(life, "GET", "", "sys/health", nil, nil, &health); e != nil || health.Version != o.Version {
		return c, errFixture
	}
	if e = c.bootstrap(life, admin); e != nil {
		return c, e
	}
	return c, nil
}
func (c *Cluster) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.dockerName != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "docker", "--host", "unix:///var/run/docker.sock", "rm", "--force", c.dockerName)
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			_ = cmd.Run()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			select {
			case <-c.exited:
			case <-time.After(5 * time.Second):
				_ = c.cmd.Process.Kill()
				select {
				case <-c.exited:
				case <-time.After(time.Second):
				}
			}
		}
		if c.http != nil {
			c.http.CloseIdleConnections()
		}
		for i := range c.Spaces {
			for _, x := range []*Identity{&c.Spaces[i].Provision, &c.Spaces[i].Control, &c.Spaces[i].Maintenance, &c.Spaces[i].Replacement} {
				clear(x.Token)
				clear(x.RoleID)
				clear(x.SecretID)
			}
		}
		if c.dir != "" {
			_ = os.RemoveAll(c.dir)
		}
	})
}
func freePort() (int, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func tlsFixture() (ca, cert, key []byte, err error) {
	priv, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, nil, nil, e
	}
	server, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, nil, nil, e
	}
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SDK temporary transport CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, e := x509.CreateCertificate(rand.Reader, root, root, priv.Public(), priv)
	if e != nil {
		return nil, nil, nil, e
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "SDK temporary server"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(12 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, root, server.Public(), priv)
	if e != nil {
		return nil, nil, nil, e
	}
	pk, e := x509.MarshalPKCS8PrivateKey(server)
	if e != nil {
		return nil, nil, nil, e
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}), nil
}

// call is ONLY the owned fixture bootstrap transport; never exported as SDK admin.
func (c *Cluster) call(ctx context.Context, method, ns, path string, token []byte, body any, out any) error {
	var b []byte
	var e error
	if body != nil {
		b, e = json.Marshal(body)
		if e != nil {
			return errFixture
		} /* HTTP transport owns its immutable body copy until completion. */
	}
	req, e := http.NewRequestWithContext(ctx, method, c.Address+"/v1/"+path, bytes.NewReader(b))
	if e != nil {
		return errFixture
	}
	if len(token) > 0 {
		req.Header.Set("X-Vault-Token", string(token))
	}
	if ns != "" {
		req.Header.Set("X-Vault-Namespace", ns)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return errFixture
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(data) > 2<<20 {
		return errFixture
	}
	defer clear(data)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errFixture
	}
	if out != nil {
		if json.Unmarshal(data, out) != nil {
			return errFixture
		}
	}
	return nil
}
