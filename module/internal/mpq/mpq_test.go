package mpq

import (
	"os"
	"strings"
	"testing"
)

// The client archives (assets/client/Data, or MPQ_DIR) when present.
func TestClient(t *testing.T) {
	dir := os.Getenv("MPQ_DIR")
	if dir == "" {
		dir = "../../../assets/client/Data"
	}
	s, err := OpenClient(dir)
	if err != nil {
		t.Skip(err)
	}
	defer s.Close()
	lf, err := s.Read("(listfile)")
	if err != nil {
		t.Fatal("listfile:", err)
	}
	t.Logf("listfile: %d lines", strings.Count(string(lf), "\n"))
	for _, l := range strings.Split(string(lf), "\n") {
		if strings.Contains(strings.ToLower(l), "azeroth_32_48") || strings.HasSuffix(strings.ToLower(strings.TrimSpace(l)), ".wdt") {
			t.Log(l)
		}
	}
	w, err := s.Read(`World\Maps\Azeroth\Azeroth.wdt`)
	if err != nil {
		t.Fatal("wdt:", err)
	}
	if string(w[:4]) != "REVM" {
		t.Fatalf("wdt starts with %q", w[:4])
	}
}
