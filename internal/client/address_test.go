package client

import "testing"

func TestNormalizeServerAddress(t *testing.T) {
	for _, input := range []string{
		"https://memory.example.com",
		"https://memory.example.com/",
		"https://memory.example.com/mcp",
		"https://memory.example.com/visualizer/",
	} {
		mcp, base, err := NormalizeServerAddress(input)
		if err != nil || mcp != "https://memory.example.com/mcp" || base != "https://memory.example.com" {
			t.Fatalf("%q: mcp=%q base=%q err=%v", input, mcp, base, err)
		}
	}
	for _, input := range []string{
		"http://memory.example.com", "https://user:pass@memory.example.com",
		"https://memory.example.com/?token=secret", "https://memory.example.com/#token",
		"https://memory.example.com/?",
		"https://memory.example.com/other", "https://memory.example.com/%6dcp",
	} {
		if _, _, err := NormalizeServerAddress(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
