package dialogs

import (
	"os"
	"path/filepath"
	"testing"
)

func dirNameFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"src", "docs"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Enter on a directory name navigates into it instead of choosing it, in both
// modes, and a plain file name still chooses.
func TestConfirmChoiceDirectoryNavigates(t *testing.T) {
	for _, save := range []bool{false, true} {
		root := dirNameFixture(t)
		d := NewFileDialog()
		chosen, overwrite := "", ""
		d.OnConfirmOverwrite = func(p string, proceed func()) { overwrite = p; proceed() }
		show := d.ShowOpen
		if save {
			show = d.ShowSave
		}
		show("t", root+string(filepath.Separator), func(p string) { chosen = p })

		d.nameField.SetValue("src")
		d.confirmChoice()
		if chosen != "" || overwrite != "" {
			t.Fatalf("save=%v: directory was chosen (%q) or prompted", save, chosen)
		}
		if d.dir != filepath.Join(root, "src") || d.nameField.Value() != "" {
			t.Errorf("save=%v: dir=%q name=%q, want navigation into src", save, d.dir, d.nameField.Value())
		}

		d.nameField.SetValue("..")
		d.confirmChoice()
		if d.dir != root || chosen != "" {
			t.Errorf("save=%v: .. gave dir=%q chosen=%q", save, d.dir, chosen)
		}

		d.nameField.SetValue("nope" + string(filepath.Separator))
		d.confirmChoice()
		if chosen != "" || d.listErr == "" {
			t.Errorf("save=%v: missing dir chosen=%q listErr=%q, want error and no choice", save, chosen, d.listErr)
		}

		d.loadDir(root)
		d.nameField.SetValue("main.go")
		d.confirmChoice()
		if chosen != filepath.Join(root, "main.go") {
			t.Errorf("save=%v: file chosen=%q", save, chosen)
		}
	}
}
