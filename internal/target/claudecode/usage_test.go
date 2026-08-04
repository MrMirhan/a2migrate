package claudecode

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
)

func TestTokensToUsage_RoundTrip(t *testing.T) {
	in := domain.Tokens{
		Input:       100,
		Output:      50,
		CacheRead:   1024,
		CacheWrite:  200,
		Reasoning:   10,
		ServiceTier: "standard",
		Speed:       "fast",
	}
	got := tokensToUsage(in)
	js, _ := json.Marshal(got)
	s := string(js)
	for _, want := range []string{
		`"input_tokens":100`,
		`"output_tokens":50`,
		`"cache_read_input_tokens":1024`,
		`"cache_creation_input_tokens":200`,
		`"reasoning_tokens":10`,
		`"service_tier":"standard"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestSessionWriter_EmitsUsageAndCost(t *testing.T) {
	ccRoot := t.TempDir()
	w := NewSessionWriter(ccRoot)
	sess := sampleDomainSession()
	// Inject a token-bearing assistant message.
	sess.Messages[1].Tokens = domain.Tokens{
		Input: 100, Output: 50, CacheRead: 1024, CacheWrite: 200, Reasoning: 10,
	}
	sess.Messages[1].CostUSD = 0.1234
	sess.Messages[1].FinishedAt = time.Date(2026, 7, 20, 10, 0, 2, 0, time.UTC)
	sess.Messages[1].ModelID = "claude-opus-4-5"
	out, err := w.WriteSession(sess, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readFile(t, out)
	s := string(body)
	for _, want := range []string{
		`"usage":`,
		`"input_tokens":100`,
		`"cache_read_input_tokens":1024`,
		`"reasoning_tokens":10`,
		`"cost_usd":0.1234`,
		`"completedAt":`,
		`"model":"claude-opus-4-5"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

func TestSessionWriter_SkipsUsageWhenZero(t *testing.T) {
	ccRoot := t.TempDir()
	w := NewSessionWriter(ccRoot)
	sess := sampleDomainSession()
	out, err := w.WriteSession(sess, "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readFile(t, out)
	if strings.Contains(string(body), `"usage":`) {
		t.Fatalf("expected no usage block, got:\n%s", body)
	}
	if strings.Contains(string(body), `"cost_usd":`) {
		t.Fatalf("expected no cost_usd, got:\n%s", body)
	}
}

func readFile(t *testing.T, p string) ([]byte, error) {
	t.Helper()
	return readWhole(p)
}

func readWhole(p string) ([]byte, error) {
	return readFileRaw(p)
}

func readFileRaw(p string) ([]byte, error) {
	return readBytes(p)
}

func readBytes(p string) ([]byte, error) {
	return doRead(p)
}

func doRead(p string) ([]byte, error) {
	return osReadFile(p)
}
