package remotecheck

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/RockInMars/openbao-sdk-go/baoerr"
	"github.com/RockInMars/openbao-sdk-go/kv"
)

func (h *harness) readOwner(path, owner string, version int) error {
	r, err := h.kv.ReadVersion(h.ctx, path, version)
	if err != nil {
		return err
	}
	defer r.Data.Zero()
	var data map[string]string
	if r.Data.Decode(&data) != nil || data["sdk_test_owner"] != owner {
		return errors.New("ownership mismatch")
	}
	return nil
}

func (h *harness) writeKV() (ok bool) {
	// No mutation starts without enough request and time budget for verification and cleanup.
	deadline, _ := h.ctx.Deadline()
	if h.b.limit-h.b.count() < 20 || time.Until(deadline) < 60*time.Second {
		return h.check("write_budget", errors.New("cleanup reserve unavailable"))
	}
	path := h.c.prefix + "/" + h.c.runID + "/kv"
	_, status, err := h.raw(http.MethodGet, h.c.mount+"/metadata/"+path, nil)
	// A 404 is only a precondition; CAS=0 remains the authoritative no-overwrite guard.
	if err == nil {
		return h.check("empty_path", errors.New("existing resource"))
	}
	if status != http.StatusNotFound {
		return h.check("empty_path", err)
	}
	if !h.check("empty_path", nil) {
		return false
	}
	return h.createKV(path)
}

func (h *harness) createKV(path string) (ok bool) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return h.check("owner_nonce", errors.New("random unavailable"))
	}
	owner := hex.EncodeToString(nonce[:])
	ownerDigest := sha256.Sum256([]byte(owner))
	index := len(h.output.Resources)
	h.output.Resources = append(h.output.Resources, resource{Kind: "kv", Mount: h.c.mount, Path: path, State: "creation_pending", OwnershipSHA256: hex.EncodeToString(ownerDigest[:])})
	if !h.save() {
		return false
	}
	doc, err := kv.NewDocument(map[string]string{"sdk_test_owner": owner})
	if err != nil {
		return h.check("document", err)
	}
	defer doc.Zero()
	client := h.kv
	written, err := client.Create(h.ctx, path, doc)
	if !h.check("kv_create", err) {
		if err != nil && !baoerr.HasUnknownOutcome(err) {
			h.output.Resources[index].State = "not_created"
			h.save()
		}
		return false
	}
	h.output.Resources[index].Versions = []int{written.Ref.Version}
	h.output.Resources[index].State = "owned"
	if written.Ref.Version != 1 {
		return h.check("kv_created_version", errors.New("unexpected version"))
	}
	if !h.save() {
		return false
	}
	defer func() {
		if h.journalFailed {
			return
		}
		for _, c := range h.output.Cases {
			if c.Unknown {
				h.output.Resources[index].State = "cleanup_pending"
				h.save()
				ok = false
				return
			}
		}
		// Every self-created version must still belong to this run. A newer version prevents deletion.
		meta, readErr := client.ReadMetadata(h.ctx, path)
		if readErr == nil && meta.CurrentVersion != len(h.output.Resources[index].Versions) {
			readErr = errors.New("foreign version")
		}
		if readErr == nil {
			for _, v := range h.output.Resources[index].Versions {
				if readErr = h.readOwner(path, owner, v); readErr != nil {
					break
				}
			}
		}
		if readErr != nil {
			h.check("kv_cleanup", readErr)
			h.output.Resources[index].State = "cleanup_pending"
			h.save()
			ok = false
			return
		}
		h.output.Resources[index].State = "cleanup_pending"
		if !h.save() {
			ok = false
			return
		}
		_, _, deleteErr := h.raw(http.MethodDelete, h.c.mount+"/metadata/"+path, nil)
		if deleteErr == nil {
			_, status, verifyErr := h.raw(http.MethodGet, h.c.mount+"/metadata/"+path, nil)
			if status != http.StatusNotFound {
				deleteErr = verifyErr
				if deleteErr == nil {
					deleteErr = errors.New("cleanup not confirmed")
				}
			}
		}
		if h.check("kv_cleanup", deleteErr) {
			h.output.Resources[index].State = "removed"
		} else {
			ok = false
		}
		h.save()
	}()
	if !h.check("kv_read_created", h.readOwner(path, owner, 1)) {
		return false
	}
	updated, err := client.CompareAndSwap(h.ctx, path, 1, doc)
	if err == nil {
		h.output.Resources[index].Versions = append(h.output.Resources[index].Versions, updated.Ref.Version)
	}
	if !h.check("kv_update", err) {
		return false
	}
	_, err = client.CompareAndSwap(h.ctx, path, 1, doc)
	if !baoerr.IsCode(err, baoerr.CodeCASConflict) {
		if err == nil {
			err = errors.New("expected CAS conflict")
		}
		return h.check("kv_stale_cas", err)
	}
	if !h.check("kv_stale_cas", nil) {
		return false
	}
	if !h.check("kv_read_updated", h.readOwner(path, owner, 2)) {
		return false
	}
	if h.c.softDelete {
		if !h.check("kv_soft_delete", client.DeleteVersions(h.ctx, path, []int{1})) {
			return false
		}
		if !h.check("kv_restore", client.UndeleteVersions(h.ctx, path, []int{1})) {
			return false
		}
		if !h.check("kv_read_restored", h.readOwner(path, owner, 1)) {
			return false
		}
	}
	return true
}
