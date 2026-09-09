//go:build ignore

// Sign a PDF with Liro Bridge, from Go, with nothing but the standard
// library.
//
//	go run sign.go ugovor.pdf
//
// Reads the pairing from LIRO_APP_ID and LIRO_SECRET when they are set,
// and pairs otherwise — the agent shows six digits on the person's
// screen and this asks for them.
//
// Written against docs/PROTOCOL.md. The four mistakes a first
// integration makes are avoided here by construction:
//
//  1. a wrong field name          — every name comes from §5.2's example
//  2. origin missing from confirm — ONE origin variable, used in both calls
//  3. a body altered between being hashed and being sent — the bytes are
//     built once and both hashed and sent
//  4. a Content-Type on a request with no body — only set when there is one
package main

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// --- README ---
var base string // http://127.0.0.1:<port>, from the discovery file

func discover() error {
	b, err := os.ReadFile(filepath.Join(os.Getenv("LOCALAPPDATA"), "Liro", "bridge.json"))
	if err != nil {
		return fmt.Errorf("Liro Bridge is not running: %w", err) // never scan ports
	}
	var info struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return err
	}
	base = fmt.Sprintf("http://127.0.0.1:%d", info.Port)
	return nil
}

// call sends one request, signing it when a secret is given.
func call(method, path string, body any, appID, secret string) (int, []byte, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil { // hashed AND sent: one slice
			return 0, nil, err
		}
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	if len(raw) > 0 {
		req.Header.Set("Content-Type", "application/json") // only when there is a body
	}
	if secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := hex.EncodeToString(randomBytes(16))
		sum := sha256.Sum256(raw)
		canon := strings.Join([]string{method, path, ts, nonce, hex.EncodeToString(sum[:])}, "\n")
		key, err := base64.StdEncoding.DecodeString(secret)
		if err != nil {
			return 0, nil, err
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(canon))
		req.Header.Set("X-Liro-App-Id", appID)
		req.Header.Set("X-Liro-Timestamp", ts)
		req.Header.Set("X-Liro-Nonce", nonce)
		req.Header.Set("X-Liro-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	return resp.StatusCode, out, err // an error body is {"code":"...","details":{...}}
}

// --- /README ---

// randomBytes is where the nonce comes from: a cryptographic source,
// never a pseudo-random one, and a new one on every request including a
// retry.
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run sign.go <document.pdf>")
		os.Exit(2)
	}
	path := os.Args[1]
	must(discover())

	status, body, err := call("GET", "/v2/health", nil, "", "")
	must(err)
	fmt.Printf("health %d %s\n", status, body)

	origin := envOr("LIRO_ORIGIN", "local")
	appID, secret := os.Getenv("LIRO_APP_ID"), os.Getenv("LIRO_SECRET")
	if appID == "" || secret == "" {
		appID, secret = pair(origin, "Go primer")
	}

	if status, body, err = call("GET", "/v2/certificates", nil, appID, secret); err == nil && status == 200 {
		var certs struct {
			Certificates []struct {
				Thumbprint, DisplayName, NotUsableReason string
				Usable, IsTestKey                        bool
			}
		}
		_ = json.Unmarshal(body, &certs)
		for _, c := range certs.Certificates {
			mark := ""
			if c.IsTestKey {
				mark = " [TEST KEY]"
			}
			state := c.NotUsableReason
			if c.Usable {
				state = "usable"
			}
			fmt.Printf("  %s  %s%s  %s\n", last8(c.Thumbprint), c.DisplayName, mark, state)
		}
	}

	pdf, err := os.ReadFile(path)
	must(err)
	status, body, err = call("POST", "/v2/sign/pdf", map[string]any{
		"documents": []map[string]string{{
			"name":    filepath.Base(path),
			"content": base64.StdEncoding.EncodeToString(pdf),
		}},
		"level": "b-b",
	}, appID, secret)
	must(err)
	if status != 202 {
		fmt.Fprintf(os.Stderr, "sign/pdf: HTTP %d %s\n", status, body)
		os.Exit(1)
	}
	var job struct{ JobID, BatchFingerprint, ResultURL string }
	must(json.Unmarshal(body, &job))
	fmt.Printf("job %s — approve it in the agent's window\nbatch fingerprint %s\n", job.JobID, job.BatchFingerprint)

	for {
		time.Sleep(time.Second)
		status, body, err = call("GET", job.ResultURL, nil, appID, secret)
		must(err)
		if status == 202 {
			var p struct {
				State            string
				Completed, Total int
			}
			_ = json.Unmarshal(body, &p)
			fmt.Printf("  %s %d/%d\n", p.State, p.Completed, p.Total)
			continue
		}
		if status != 200 {
			fmt.Fprintf(os.Stderr, "failed: HTTP %d %s\n", status, body)
			os.Exit(1)
		}
		var result struct {
			Documents []struct{ Name, Content, AchievedLevel string }
		}
		must(json.Unmarshal(body, &result))
		out := strings.TrimSuffix(path, filepath.Ext(path)) + "-signed.pdf"
		signed, err := base64.StdEncoding.DecodeString(result.Documents[0].Content)
		must(err)
		must(os.WriteFile(out, signed, 0o600))
		fmt.Printf("saved %s at %s\n", out, result.Documents[0].AchievedLevel)
		return
	}
}

// pair runs the two calls, with the same origin in both.
func pair(origin, name string) (string, string) {
	status, body, err := call("POST", "/v2/pair/request", map[string]string{
		"applicationName": name, "origin": origin,
	}, "", "")
	must(err)
	if status != 200 {
		fmt.Fprintf(os.Stderr, "pair/request: HTTP %d %s\n", status, body)
		os.Exit(1)
	}
	var req struct{ RequestID string }
	must(json.Unmarshal(body, &req))

	fmt.Println("Liro Bridge is showing six digits on the screen.")
	fmt.Print("Code: ")
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	must(err)

	status, body, err = call("POST", "/v2/pair/confirm", map[string]string{
		"requestId": req.RequestID,
		"code":      strings.TrimSpace(code),
		"origin":    origin, // the same origin, from the same variable
	}, "", "")
	must(err)
	if status != 200 {
		fmt.Fprintf(os.Stderr, "pair/confirm: HTTP %d %s\n", status, body)
		os.Exit(1)
	}
	var ok struct{ AppID, DeviceSecret string }
	must(json.Unmarshal(body, &ok))
	fmt.Println("Keep these; the secret is returned once and never again:")
	fmt.Printf("  set LIRO_APP_ID=%s\n  set LIRO_SECRET=%s\n", ok.AppID, ok.DeviceSecret)
	return ok.AppID, ok.DeviceSecret
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func last8(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[len(s)-8:]
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
