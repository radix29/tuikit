package dialogs

import (
	"math"
	"strconv"
	"strings"

	"github.com/radix29/tuikit/widgets"
)

// ---------------------------------------------------------------------------
// Tab completion
// ---------------------------------------------------------------------------

// completeField extends f's value with the longest prefix shared by every
// directory entry (dirsOnly, for the path field) or every entry (the name field)
// matching what's typed after the last separator: shell-style Tab completion.
// Returns false, leaving f untouched, when there's nothing to complete, so the
// caller falls through to Tab focus-cycling.
func (d *FileDialog) completeField(f *widgets.InputField, dirsOnly bool) bool {
	fs := d.FileSystem()
	val := f.Value()
	dirPart, base := fs.Split(val)
	searchDir := d.dir
	if dirPart != "" {
		if fs.IsAbs(dirPart) {
			searchDir = fs.Clean(dirPart)
		} else {
			searchDir = fs.Join(d.dir, dirPart)
		}
	}
	// Complete against the directory already on screen, not a second listing: this
	// runs on every Tab, a network round trip on a remote filesystem. d.entries
	// carries the dialog's own ".." row, which fs.List never reports and which would
	// poison the common prefix.
	infos := d.entries
	if searchDir != d.dir {
		var err error
		if infos, err = fs.List(searchDir); err != nil {
			return false
		}
	}
	var candidates []string
	for _, e := range infos {
		if e.Name == ".." {
			continue
		}
		if dirsOnly && !e.IsDir {
			continue
		}
		if base == "" || strings.HasPrefix(e.Name, base) {
			candidates = append(candidates, e.Name)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	common := commonPrefix(candidates)
	if common == "" || common == base {
		return false
	}
	completed := dirPart + common
	if len(candidates) == 1 {
		// A failed probe just leaves the trailing separator off.
		if _, isDir, _ := fs.Exists(fs.Join(searchDir, common)); isDir {
			completed += fs.Separator()
		}
	}
	f.SetValue(completed)
	return true
}

// commonPrefix returns the longest string every element of strs starts with
// (strs is never empty). It compares rune by rune: a byte prefix could end
// mid-rune on multi-byte filenames, and InputField.SetValue's []rune(v) turns
// that invalid tail into a stray replacement character.
func commonPrefix(strs []string) string {
	prefix := []rune(strs[0])
	for _, s := range strs[1:] {
		r := []rune(s)
		n := min(len(r), len(prefix))
		i := 0
		for i < n && prefix[i] == r[i] {
			i++
		}
		prefix = prefix[:i]
	}
	return string(prefix)
}

// formatFileSize renders n bytes as a short human-readable size (e.g.
// "1.8 KB"), matching the mockup's column style.
//
// The units run to EB because an int64 tops out just below 8 EiB, and a file
// that large is real: a sparse file takes no disk space, so `truncate -s 2P`
// makes one in an instant. With "KMGT" alone, 1 PiB indexed past the end and
// panicked inside Draw. The unit is chosen from the rounded value, so 1 MiB − 1
// reads "1.0 MB", not "1024.0 KB". A negative size (a FileSystem reporting
// garbage) is left as it is and prints e.g. "-1 B".
func formatFileSize(n int64) string {
	const unit = 1024
	const units = "KMGTPE"
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	v, exp := float64(n)/unit, 0
	for math.Round(v*10)/10 >= unit && exp < len(units)-1 {
		v /= unit
		exp++
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + " " + units[exp:exp+1] + "B"
}
