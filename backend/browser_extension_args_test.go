package backend

import (
	"testing"
)

func TestBuildLoadExtensionArgEmptyReturnsEmpty(t *testing.T) {
	if got := buildLoadExtensionArg(nil); got != "" {
		t.Fatalf("empty want=\"\" got=%q", got)
	}
	if got := buildLoadExtensionArg([]string{}); got != "" {
		t.Fatalf("[]string want=\"\" got=%q", got)
	}
}

func TestBuildLoadExtensionArgJoinsWithCommas(t *testing.T) {
	paths := []string{"C:/a/unpacked", "D:/b/unpacked"}
	got := buildLoadExtensionArg(paths)
	want := "--load-extension=C:/a/unpacked,D:/b/unpacked"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestBuildLoadExtensionArgAllowsSpaces(t *testing.T) {
	paths := []string{"C:/Program Files/a"}
	got := buildLoadExtensionArg(paths)
	want := "--load-extension=C:/Program Files/a"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}
