package dialogs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// "Not there" is an answer, including a path through a file (ENOTDIR).
func TestLocalFileSystemExistsMissingIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(dir, "missing"), filepath.Join(file, "child")} {
		exists, isDir, err := LocalFileSystem{}.Exists(p)
		if exists || isDir || err != nil {
			t.Errorf("Exists(%q) = (%v, %v, %v), want (false, false, nil)", p, exists, isDir, err)
		}
	}
}

// Any other Stat failure is "couldn't ask" and must reach the caller: the
// save-overwrite guard prompts on it, and swallowing it skipped the prompt.
func TestLocalFileSystemExistsReportsPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	exists, _, err := LocalFileSystem{}.Exists(filepath.Join(locked, "x"))
	if err == nil {
		t.Fatalf("Exists under an unreadable directory = (%v, nil), want an error", exists)
	}
}

// On Windows the local filesystem has a drive-list level above the drive roots,
// as WindowsPathRules does for a remote host; elsewhere it is unchanged.
func TestLocalFileSystemDriveList(t *testing.T) {
	var lfs LocalFileSystem
	if runtime.GOOS != "windows" {
		if got := lfs.Parent("/"); got != "/" {
			t.Errorf("Parent(/) = %q, want /", got)
		}
		if got := lfs.Clean(""); got == "" {
			t.Error(`Clean("") must stay the working directory off Windows`)
		}
		return
	}
	if got := lfs.Parent(`C:\`); got != "" {
		t.Errorf(`Parent(C:\) = %q, want ""`, got)
	}
	if got := lfs.Clean(""); got != "" {
		t.Errorf(`Clean("") = %q, want ""`, got)
	}
	drives, err := lfs.List("")
	if err != nil || len(drives) == 0 {
		t.Fatalf(`List("") = %v, %v; want at least one drive`, drives, err)
	}
	for _, d := range drives {
		if !d.IsDir || len(d.Name) != 2 || d.Name[1] != ':' {
			t.Errorf("drive entry %+v is not a bare drive like C:", d)
		}
		if got := lfs.Clean(lfs.Join("", d.Name)); got != d.Name+`\` {
			t.Errorf("entering %q gives %q, want its root", d.Name, got)
		}
	}
}
