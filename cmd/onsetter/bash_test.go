package main

import (
	"sort"
	"testing"
)

// sortedPaths makes assertion order-independent — extractWrittenPaths makes
// no promise about the order it returns paths in.
func sortedPaths(t *testing.T, command string) []string {
	t.Helper()
	got := extractWrittenPaths(command)
	sort.Strings(got)
	return got
}

func TestExtractWrittenPathsRedirects(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"overwrite", `echo hi > out.txt`, []string{"out.txt"}},
		{"append", `echo hi >> out.txt`, []string{"out.txt"}},
		{"heredoc target", "cat > out.txt <<EOF\nbody\nEOF", []string{"out.txt"}},
		{"stdout fd redirect not a write", `cmd 2>&1`, nil},
		{"stderr-to-fd not a write", `cmd >&2`, nil},
		{"combined redirect not a write", `cmd &>out.txt 2>&1`, []string{"out.txt"}},
		{"multiple commands, both redirects", `echo a > a.txt && echo b > b.txt`, []string{"a.txt", "b.txt"}},
		{"piped and redirected", `gen.sh | tee raw.txt > final.txt`, []string{"final.txt", "raw.txt"}},
		{"variable target not resolved", `echo hi > "$OUT"`, nil},
		{"command substitution target not resolved", "echo hi > \"$(name)\"", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedPaths(t, tc.command)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !equalStrings(got, want) {
				t.Errorf("extractWrittenPaths(%q) = %v, want %v", tc.command, got, want)
			}
		})
	}
}

func TestExtractWrittenPathsTee(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"single target", `gen.sh | tee out.txt`, []string{"out.txt"}},
		{"append flag", `gen.sh | tee -a out.txt`, []string{"out.txt"}},
		{"multiple targets", `gen.sh | tee a.txt b.txt`, []string{"a.txt", "b.txt"}},
		{"unresolved target skipped, not aborted", `gen.sh | tee "$OUT" b.txt`, []string{"b.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedPaths(t, tc.command)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !equalStrings(got, want) {
				t.Errorf("extractWrittenPaths(%q) = %v, want %v", tc.command, got, want)
			}
		})
	}
}

func TestExtractWrittenPathsCpMv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"cp", `cp source.txt dest.txt`, []string{"dest.txt"}},
		{"mv with flag", `mv -f old.txt new.txt`, []string{"new.txt"}},
		{"cp recursive dir", `cp -r src/ dest/`, []string{"dest/"}},
		{"unresolved source doesn't block a resolved dest", `cp "$SRC" dest.txt`, []string{"dest.txt"}},
		{"unresolved dest aborts", `cp source.txt "$DEST"`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedPaths(t, tc.command)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !equalStrings(got, want) {
				t.Errorf("extractWrittenPaths(%q) = %v, want %v", tc.command, got, want)
			}
		})
	}
}

func TestExtractWrittenPathsSedInPlace(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"in-place single file", `sed -i 's/x/y/' file.go`, []string{"file.go"}},
		{"in-place suffix", `sed -i.bak 's/x/y/' file.go`, []string{"file.go"}},
		{"in-place multiple files", `sed -i 's/x/y/' a.go b.go`, []string{"a.go", "b.go"}},
		{"not in-place contributes nothing", `sed 's/x/y/' file.go`, nil},
		{"unresolved file aborts", `sed -i 's/x/y/' "$FILE"`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedPaths(t, tc.command)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !equalStrings(got, want) {
				t.Errorf("extractWrittenPaths(%q) = %v, want %v", tc.command, got, want)
			}
		})
	}
}

func TestExtractWrittenPathsSd(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"rewrites in place by default", `sd 'foo' 'bar' file.txt`, []string{"file.txt"}},
		{"stdin form contributes nothing", `cat file.txt | sd 'foo' 'bar'`, nil},
		{"unresolved file aborts", `sd 'foo' 'bar' "$FILE"`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedPaths(t, tc.command)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !equalStrings(got, want) {
				t.Errorf("extractWrittenPaths(%q) = %v, want %v", tc.command, got, want)
			}
		})
	}
}

func TestExtractWrittenPathsEdgeCases(t *testing.T) {
	if got := extractWrittenPaths(""); got != nil {
		t.Errorf("empty command should contribute nothing, got %v", got)
	}
	if got := extractWrittenPaths("if (( ; do done"); got != nil {
		t.Errorf("unparseable command should contribute nothing, got %v", got)
	}
	// A read-only command with no write shape anywhere contributes nothing.
	if got := extractWrittenPaths(`git status`); got != nil {
		t.Errorf("a read-only command should contribute nothing, got %v", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
