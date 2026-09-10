package secfile

import (
	"os"
	"path/filepath"
	"testing"
)

// perm reads back the mode bits a real filesystem enforces, masking off
// anything beyond permission bits (directory/regular-file type bits, etc.).
func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	return info.Mode().Perm()
}

func TestEnsureDirTightensAnAlreadyExistingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != 0o755 {
		t.Fatalf("setup: dir mode = %o, want 0755", got)
	}

	// The naive fix — just changing the literal MkdirAll is called with —
	// leaves an already-existing dir exactly as loose as it was, because
	// MkdirAll only applies mode on create. This is the case that fix
	// silently misses, proven here rather than asserted.
	if err := EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != 0o700 {
		t.Fatalf("EnsureDir on existing dir: mode = %o, want 0700", got)
	}
}

func TestEnsureDirCreatesFreshAtMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fresh", "nested")
	if err := EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != 0o700 {
		t.Fatalf("fresh dir mode = %o, want 0700", got)
	}
}

func TestWriteFileTightensAnAlreadyExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, path); got != 0o644 {
		t.Fatalf("setup: file mode = %o, want 0644", got)
	}

	if err := WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, path); got != 0o600 {
		t.Fatalf("WriteFile on existing file: mode = %o, want 0600", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("content = %q, want %q", got, "new")
	}
}

func TestWriteFileCreatesFreshAtMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh")
	if err := WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, path); got != 0o600 {
		t.Fatalf("fresh file mode = %o, want 0600", got)
	}
}

// TestEnsureDirNeverWidensADeliberatelyRestrictedDir is the regression this
// package exists to guard: an early version of EnsureDir chmod'd
// unconditionally, which widened a directory someone had deliberately made
// non-writable back to writable — since chmod only needs ownership, not
// write access, that succeeded silently and broke
// cmd/onsetter.TestWriteSkillFailsWhenDirNotWritable, which depends on a
// non-writable skill directory staying non-writable through writeSkill.
// EnsureDir must never add a permission bit the path didn't already have,
// even while narrowing others.
func TestEnsureDirNeverWidensADeliberatelyRestrictedDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory write permission bits")
	}
	dir := filepath.Join(t.TempDir(), "restricted")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil { // r-xr-xr-x: no owner-write, on purpose
		t.Fatal(err)
	}

	if err := EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if got := perm(t, dir); got&0o200 != 0 {
		t.Fatalf("EnsureDir added the owner-write bit to a dir that didn't have it: mode = %o", got)
	}
	if _, err := os.Create(filepath.Join(dir, "f")); err == nil {
		t.Fatal("want a write into the still-restricted dir to fail, it succeeded")
	}
}
