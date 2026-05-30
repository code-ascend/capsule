package export

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"capsule/internal/format/binconfig"
)

func TestFindAndCopyIconsHiColorXPM(t *testing.T) {
	root := t.TempDir()
	dataHome := t.TempDir()
	writeFile(t, root, "usr/share/icons/hicolor/48x48/apps/7colors.xpm", "icon")

	got, err := findAndCopyIcons(root, "7colors", dataHome)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dataHome, "icons/hicolor/48x48/apps/7colors.xpm")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := os.Stat(want[0]); err != nil {
		t.Fatalf("icon not copied: %v", err)
	}
}

func TestFindAndCopyIconsAllSizes(t *testing.T) {
	root := t.TempDir()
	dataHome := t.TempDir()
	writeFile(t, root, "usr/share/icons/hicolor/48x48/apps/app.png", "icon")
	writeFile(t, root, "usr/share/icons/hicolor/256x256/apps/app.png", "icon")
	writeFile(t, root, "usr/share/icons/hicolor/scalable/apps/app.svg", "icon")

	got, err := findAndCopyIcons(root, "app", dataHome)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dataHome, "icons/hicolor/256x256/apps/app.png"),
		filepath.Join(dataHome, "icons/hicolor/48x48/apps/app.png"),
		filepath.Join(dataHome, "icons/hicolor/scalable/apps/app.svg"),
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exported icons mismatch:\n got  %v\n want %v", got, want)
	}
	for _, p := range got {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("icon not copied: %s: %v", p, err)
		}
	}
}

func TestFindAndCopyIconsPrefersPNGOverXPMPerSize(t *testing.T) {
	root := t.TempDir()
	dataHome := t.TempDir()
	writeFile(t, root, "usr/share/icons/hicolor/48x48/apps/app.png", "icon")
	writeFile(t, root, "usr/share/icons/hicolor/48x48/apps/app.xpm", "icon")

	got, err := findAndCopyIcons(root, "app", dataHome)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dataHome, "icons/hicolor/48x48/apps/app.png")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (one file per size, png preferred)", got, want)
	}
}

func TestFindAndCopyIconsPixmapsFallback(t *testing.T) {
	root := t.TempDir()
	dataHome := t.TempDir()
	writeFile(t, root, "usr/share/pixmaps/app.png", "icon")

	got, err := findAndCopyIcons(root, "app", dataHome)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dataHome, "pixmaps/app.png")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIconBaseName(t *testing.T) {
	cases := map[string]string{
		"/usr/share/icons/hicolor/32x32/apps/7colors.xpm": "7colors",
		"/usr/share/pixmaps/foo.png":                      "foo",
		"firefox":                                         "firefox",
		"":                                                "",
	}
	for in, want := range cases {
		if got := iconBaseName(in); got != want {
			t.Errorf("iconBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppsAbsolutePathIcon(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "usr/share/applications/7colors.desktop",
		"[Desktop Entry]\nName=7colors\nExec=7colors\nIcon=/usr/share/icons/hicolor/32x32/apps/7colors.xpm\n")
	writeFile(t, root, "usr/share/icons/hicolor/32x32/apps/7colors.xpm", "icon")
	writeFile(t, root, "usr/share/icons/hicolor/48x48/apps/7colors.xpm", "icon")

	cfg := &binconfig.Config{Apps: []binconfig.AppExport{
		{Desktop: "/usr/share/applications/7colors.desktop"},
	}}
	ex := newTestExporter(t, "/cap", cfg, root)
	if err := ex.Apps(); err != nil {
		t.Fatal(err)
	}

	desktop := filepath.Join(ex.paths.XDGDataHome, "applications/7colors.desktop")
	body, err := os.ReadFile(desktop)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Icon=7colors\n") {
		t.Fatalf("Icon= not normalized to bare name:\n%s", body)
	}
	for _, size := range []string{"32x32", "48x48"} {
		p := filepath.Join(ex.paths.XDGDataHome, "icons/hicolor", size, "apps/7colors.xpm")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("icon %s not exported: %v", size, err)
		}
	}
}
