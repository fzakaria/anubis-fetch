package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/imroc/req/v3"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const (
	wasmSHA256         = "sha256"
	wasmArgon2ID       = "argon2id"
	wasmHashX          = "hashx"
	wasmModulePath     = "/.within.website/x/cmd/anubis/static/wasm/baseline/"
	maxWASMModuleBytes = 8 * 1024 * 1024
	maxWASMDataBytes   = 4096
	// Match the verifier's 32 MiB memory cap (512 pages of 64 KiB).
	maxWASMMemoryPages = 512
	initialWASMNonce   = 0
	wasmNonceStride    = 1
)

func isWASMMethod(method string) bool {
	switch method {
	case wasmSHA256, wasmArgon2ID, wasmHashX:
		return true
	default:
		return false
	}
}

func challengeEndpoint(origin *url.URL, c *challenge, endpoint string) *url.URL {
	// Keep challenge assets and submissions on the origin that issued the challenge.
	result := *origin
	result.Path = strings.TrimRight(c.basePrefix, "/") + endpoint
	result.RawPath, result.RawQuery, result.Fragment = "", "", ""
	return &result
}

func fetchWASM(ctx context.Context, client *req.Client, origin *url.URL, c *challenge) ([]byte, error) {
	// Use the challenge's cookie jar and version when fetching the baseline module.
	asset := challengeEndpoint(origin, c, wasmModulePath+c.method+".wasm")
	query := url.Values{}
	query.Set("cacheBuster", c.version)
	asset.RawQuery = query.Encode()
	response, err := client.R().SetContext(ctx).DisableAutoReadResponse().Get(asset.String())
	if err != nil {
		return nil, fmt.Errorf("fetch WASM: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch WASM: HTTP %d", response.StatusCode)
	}

	// Bound the body before compilation; streamed responses may omit Content-Length.
	data, err := io.ReadAll(io.LimitReader(response.Body, maxWASMModuleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read WASM: %w", err)
	}
	if len(data) > maxWASMModuleBytes {
		return nil, fmt.Errorf("WASM module exceeds %d bytes", maxWASMModuleBytes)
	}
	return data, nil
}

func solveWASM(ctx context.Context, code []byte, randomData string, difficulty int) (uint32, string, error) {
	// WASM challenges use decoded bytes rather than the legacy hex string input.
	if len(randomData) == 0 || len(randomData) > hex.EncodedLen(maxWASMDataBytes) {
		return 0, "", fmt.Errorf("WASM challenge must contain 1 to %d bytes", maxWASMDataBytes)
	}
	data, err := hex.DecodeString(randomData)
	if err != nil {
		return 0, "", fmt.Errorf("decode WASM challenge: %w", err)
	}
	if difficulty < 0 || uint64(difficulty) > math.MaxUint32 {
		return 0, "", fmt.Errorf("invalid WASM difficulty %d", difficulty)
	}

	// Expose only the nonce callback, with bounded memory and cancellable execution.
	config := wazero.NewRuntimeConfig().WithMemoryLimitPages(maxWASMMemoryPages).WithCloseOnContextDone(true)
	runtime := wazero.NewRuntimeWithConfig(ctx, config)
	defer runtime.Close(context.Background())
	_, err = runtime.NewHostModuleBuilder("anubis").NewFunctionBuilder().
		WithFunc(func(uint32) {}).Export("anubis_update_nonce").Instantiate(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("create WASM host: %w", err)
	}
	module, err := runtime.Instantiate(ctx, code)
	if err != nil {
		return 0, "", fmt.Errorf("instantiate WASM: %w", err)
	}
	memory := module.ExportedMemory("memory")
	if memory == nil {
		return 0, "", fmt.Errorf("WASM module has no exported memory")
	}

	// Write the challenge into the module's input buffer and set its byte length.
	ptr, err := wasmUint32(ctx, module, "data_ptr")
	if err != nil {
		return 0, "", err
	}
	if !memory.Write(ptr, data) {
		return 0, "", fmt.Errorf("WASM input buffer is outside memory")
	}
	if _, err := callWASM(ctx, module, "set_data_length", uint64(len(data))); err != nil {
		return 0, "", err
	}

	// Let the server's module define the hash and difficulty rules.
	nonce, err := wasmUint32(ctx, module, "anubis_work", uint64(difficulty), initialWASMNonce, wasmNonceStride)
	if err != nil {
		return 0, "", err
	}
	ptr, err = wasmUint32(ctx, module, "result_hash_ptr")
	if err != nil {
		return 0, "", err
	}
	size, err := wasmUint32(ctx, module, "result_hash_size")
	if err != nil {
		return 0, "", err
	}
	if size == 0 || size > maxWASMDataBytes {
		return 0, "", fmt.Errorf("invalid WASM result size %d", size)
	}
	result, ok := memory.Read(ptr, size)
	if !ok {
		return 0, "", fmt.Errorf("WASM result buffer is outside memory")
	}
	return nonce, hex.EncodeToString(result), nil
}

func callWASM(ctx context.Context, module api.Module, name string, args ...uint64) ([]uint64, error) {
	// Missing exports and traps are normal errors so the caller can fall back.
	function := module.ExportedFunction(name)
	if function == nil {
		return nil, fmt.Errorf("WASM module is missing %s", name)
	}
	results, err := function.Call(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("WASM %s: %w", name, err)
	}
	return results, nil
}

func wasmUint32(ctx context.Context, module api.Module, name string, args ...uint64) (uint32, error) {
	// Validate result types before treating guest values as pointers or nonces.
	results, err := callWASM(ctx, module, name, args...)
	if err != nil {
		return 0, err
	}
	if len(results) != 1 || module.ExportedFunction(name).Definition().ResultTypes()[0] != api.ValueTypeI32 {
		return 0, fmt.Errorf("WASM %s must return one i32", name)
	}
	return uint32(results[0]), nil
}
