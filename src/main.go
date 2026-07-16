// Webhook / ERP connector — a WASI command (GOOS=wasip1 GOARCH=wasm) run
// in-process by the till's wazero runtime for every `sale.completed` event.
// It mirrors each completed sale to any external system: it POSTs the raw
// SaleCompletedEvent payload (forwarded verbatim) to a configurable endpoint
// with a configurable auth header. See ADR-0014.
//
// Offline-first is the point (ADR-0003): the tender path never waits on the
// ERP. If a POST fails (network down, ERP 5xx, non-2xx), the sale is queued
// in this plugin's storage and retried on the next invocation. The queue is
// bounded; the till keeps selling regardless of the target system. Every
// invocation flushes the queue first, then handles the current sale, and
// ALWAYS exits 0 — a connector must never fail the sale.
//
// This is the reference implementation and the TEMPLATE the SAP and
// Microsoft Dynamics / LS Central connectors are cloned from: they swap the
// transform in `postSale` (raw JSON → IDoc/BAPI/OData) and keep the queue,
// retry and settings plumbing unchanged.
package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"unsafe"
)

// --- host functions (module "ut", see reference/plugin-host-functions.md) ---
// Buffer ABI: data-returning calls write min(len, dstCap) bytes into dst and
// return the FULL length; a guest seeing len > cap retries with a bigger
// buffer. Negative returns are errors: -1 not found, -2 denied, -3 internal,
// -4 invalid.

//go:wasmimport ut log_write
func utLogWrite(ptr, n uint32)

//go:wasmimport ut storage_get
func utStorageGet(kPtr, kLen, dstPtr, dstCap uint32) int32

//go:wasmimport ut storage_set
func utStorageSet(kPtr, kLen, vPtr, vLen uint32) int32

//go:wasmimport ut http_request
func utHTTPRequest(reqPtr, reqLen, dstPtr, dstCap uint32) int32

//go:wasmimport ut settings_get
func utSettingsGet(kPtr, kLen, dstPtr, dstCap uint32) int32

const (
	hostErrNotFound = -1 // key/setting not set

	queueKey = "delivery_queue" // storage key for the bounded retry queue
	maxQueue = 200              // cap; oldest entries are dropped past this
)

// ptrOf returns the linear-memory address and length of b (the exact pattern
// the payment-demo plugin uses). For a destination buffer the length doubles
// as the capacity the host may write.
func ptrOf(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0]))), uint32(len(b))
}

func logf(msg string) {
	p, n := ptrOf([]byte(msg))
	utLogWrite(p, n)
}

// callBuf runs a data-returning host call, honoring the buffer ABI: it grows
// the destination and retries once if the first buffer was too small. Returns
// the written bytes, or a negative host error code.
func callBuf(fn func(dstPtr, dstCap uint32) int32) ([]byte, int32) {
	buf := make([]byte, 8192)
	p, c := ptrOf(buf)
	n := fn(p, c)
	if n < 0 {
		return nil, n
	}
	if int(n) > len(buf) {
		buf = make([]byte, n)
		p, c = ptrOf(buf)
		n = fn(p, c)
		if n < 0 {
			return nil, n
		}
		if int(n) > len(buf) {
			n = int32(len(buf))
		}
	}
	return buf[:n], n
}

// settingsGet reads one of this plugin's own settings. Returns (value, true)
// or ("", false) when unset/not-found.
func settingsGet(key string) (string, bool) {
	kb := []byte(key)
	out, code := callBuf(func(dp, dc uint32) int32 {
		kp, kl := ptrOf(kb)
		return utSettingsGet(kp, kl, dp, dc)
	})
	if code < 0 {
		return "", false
	}
	return string(out), true
}

// storageGet reads a plugin-storage key. Returns (value, true) or (nil, false)
// when the key is missing (hostErrNotFound) or on any error.
func storageGet(key string) ([]byte, bool) {
	kb := []byte(key)
	out, code := callBuf(func(dp, dc uint32) int32 {
		kp, kl := ptrOf(kb)
		return utStorageGet(kp, kl, dp, dc)
	})
	if code < 0 {
		return nil, false
	}
	return out, true
}

// storageSet writes a plugin-storage key. Returns 0 on success.
func storageSet(key string, val []byte) int32 {
	kp, kl := ptrOf([]byte(key))
	vp, vl := ptrOf(val)
	return utStorageSet(kp, kl, vp, vl)
}

// httpRequest performs one outbound HTTP call. req is the request JSON
// ({method,url,headers,body_b64}); returns the response JSON
// ({status,headers,body_b64}) or a negative host error code.
func httpRequest(req []byte) ([]byte, int32) {
	return callBuf(func(dp, dc uint32) int32 {
		rp, rl := ptrOf(req)
		return utHTTPRequest(rp, rl, dp, dc)
	})
}

// postSale delivers one raw sale payload to the endpoint. Returns true only on
// a 2xx response. THIS is the seam an ERP-specific connector overrides: it
// would transform `payload` (the SaleCompletedEvent JSON) into the target
// shape (SAP IDoc/BAPI/OData, Dynamics/LS Business Central OData) before the
// POST. The generic webhook forwards it verbatim.
func postSale(url, authHeader, authValue string, payload []byte) bool {
	headers := map[string]string{"Content-Type": "application/json"}
	if authValue != "" {
		headers[authHeader] = authValue
	}
	reqBytes, err := json.Marshal(map[string]any{
		"method":   "POST",
		"url":      url,
		"headers":  headers,
		"body_b64": base64.StdEncoding.EncodeToString(payload),
	})
	if err != nil {
		return false
	}
	respBytes, code := httpRequest(reqBytes)
	if code < 0 {
		logf("webhook: delivery failed (host error, queued for retry)")
		return false
	}
	var resp struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return false
	}
	if resp.Status >= 200 && resp.Status < 300 {
		return true
	}
	logf("webhook: endpoint returned non-2xx (queued for retry)")
	return false
}

// loadQueue reads the bounded retry queue. Robust to missing/empty/malformed
// storage — any problem yields an empty queue.
func loadQueue() []json.RawMessage {
	raw, ok := storageGet(queueKey)
	if !ok || len(raw) == 0 {
		return nil
	}
	var q []json.RawMessage
	if err := json.Unmarshal(raw, &q); err != nil {
		return nil
	}
	return q
}

// saveQueue persists the queue, dropping the oldest entries beyond maxQueue.
func saveQueue(q []json.RawMessage) {
	if len(q) > maxQueue {
		q = q[len(q)-maxQueue:]
	}
	b, err := json.Marshal(q)
	if err != nil {
		return
	}
	if code := storageSet(queueKey, b); code != 0 {
		logf("webhook: failed to persist retry queue")
	}
}

func main() {
	raw, _ := io.ReadAll(os.Stdin)

	// Settings: the endpoint is required. A fresh install with no endpoint is
	// a clean no-op — never an error (ADR-0014: reusable, configured per install).
	endpoint, ok := settingsGet("endpoint_url")
	if !ok || endpoint == "" {
		logf("webhook: not configured (endpoint_url empty) — skipping")
		os.Exit(0)
	}
	authHeader, ok := settingsGet("auth_header")
	if !ok || authHeader == "" {
		authHeader = "Authorization"
	}
	authValue, _ := settingsGet("auth_value")

	// 1. Flush the retry queue first: re-POST queued sales, drop the ones that
	//    now deliver, keep the rest.
	var remaining []json.RawMessage
	for _, p := range loadQueue() {
		if len(p) == 0 {
			continue
		}
		if !postSale(endpoint, authHeader, authValue, p) {
			remaining = append(remaining, p)
		}
	}

	// 2. Handle the current sale. Forward the payload (the SaleCompletedEvent)
	//    verbatim. On failure, enqueue it for retry.
	var ev struct {
		Payload json.RawMessage `json:"payload"`
	}
	_ = json.Unmarshal(raw, &ev)
	if len(ev.Payload) > 0 {
		if !postSale(endpoint, authHeader, authValue, ev.Payload) {
			remaining = append(remaining, ev.Payload)
		}
	}

	saveQueue(remaining)

	// A connector must never fail the tender path — always exit 0.
	os.Exit(0)
}
