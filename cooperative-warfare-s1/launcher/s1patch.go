package main

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Blacksmith (S1) co-op patch sets, bundled inside the launcher.
// v16 = Fusion in the Spec Ops menu, 2-player split screen (no weapons/models yet).
// v18 = v17 + script hook that gives Player 2 a gun.
// v17 = same, with Spec Ops mode off for that slot so the normal loadout runs (test build).
//
//go:embed s1patch
var s1patchFS embed.FS

var s1Sets = map[string][]string{
	"v16": {"ui.ff", "patch.ff", "patch_specialops.ff", "common_specialops.ff"},
	"v17": {"ui.ff", "patch.ff"},
	"v18": {"ui.ff", "patch.ff"},
	"v24a": {"ui.ff", "patch.ff"},
	"v24b": {"ui.ff", "patch.ff"},
	"v24c": {"ui.ff", "patch.ff"},
}

const s1Marker = "s1_coop_patch.txt"

func s1Bytes(set, name string) []byte {
	b, _ := s1patchFS.ReadFile("s1patch/" + set + "/" + name)
	return b
}

func s1Installed(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, s1Marker))
	if err != nil {
		return ""
	}
	for set := range s1Sets {
		if string(b) == set {
			return set
		}
	}
	return ""
}

func s1Install(dir, set string) error {
	files, ok := s1Sets[set]
	if !ok {
		return fmt.Errorf("unknown patch set %q", set)
	}
	if !exists(filepath.Join(dir, "common.ff")) {
		return fmt.Errorf("common.ff not found in %s - set the game folder first", dir)
	}
	for _, f := range files {
		dst := filepath.Join(dir, f)
		bak := dst + ".bak"
		if exists(dst) && !exists(bak) && (f == "ui.ff" || f == "patch.ff") && !s1IsOurs(dst, f) {
			if err := copyFile(dst, bak); err != nil {
				return fmt.Errorf("backup of %s failed: %v", f, err)
			}
			logf("backed up %s -> %s.bak", f, f)
		}
		data := s1Bytes(set, f)
		if len(data) == 0 {
			return fmt.Errorf("bundled file %s/%s missing", set, f)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %v", f, err)
		}
		logf("installed %s (%s)", f, set)
	}
	return os.WriteFile(filepath.Join(dir, s1Marker), []byte(set), 0644)
}

func s1Restore(dir string) int {
	n := 0
	for _, f := range []string{"ui.ff", "patch.ff"} {
		dst := filepath.Join(dir, f)
		if exists(dst + ".bak") {
			if err := copyFile(dst+".bak", dst); err == nil {
				n++
				logf("restored %s from backup", f)
			}
		}
	}
	for _, f := range []string{"patch_specialops.ff", "common_specialops.ff", s1Marker} {
		if os.Remove(filepath.Join(dir, f)) == nil {
			logf("removed %s", f)
		}
	}
	return n
}

var _ = io.EOF

// s1IsOurs reports whether the file on disk is one of the bundled patch files (so it must not be saved as the "original").
func s1IsOurs(path, f string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	for set := range s1Sets {
		if b := s1Bytes(set, f); len(b) > 0 && int64(len(b)) == st.Size() {
			return true
		}
	}
	return false
}
