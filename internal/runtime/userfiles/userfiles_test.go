package userfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, passwd, group, shadow string) string {
	t.Helper()
	root := t.TempDir()
	etc := filepath.Join(root, "etc")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	if passwd != "" {
		if err := os.WriteFile(filepath.Join(etc, "passwd"), []byte(passwd), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if group != "" {
		if err := os.WriteFile(filepath.Join(etc, "group"), []byte(group), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if shadow != "" {
		if err := os.WriteFile(filepath.Join(etc, "shadow"), []byte(shadow), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMergePasswdDropsHostUID(t *testing.T) {
	root := setup(t,
		"root:x:0:0:root:/root:/bin/bash\nfoo:x:1000:1000:Foo:/home/foo:/bin/sh\nmysql:x:27:27:MySQL:/var/lib/mysql:/sbin/nologin\n",
		"root:x:0:\nmysql:x:27:\n",
		"",
	)
	out := t.TempDir()
	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/zsh", Gecos: "alice"}
	if err := h.MergeFromRoot(root, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "passwd"))
	if strings.Contains(string(got), "foo:x:1000") {
		t.Fatalf("expected foo (uid 1000) to be dropped, got:\n%s", got)
	}
	if !strings.Contains(string(got), "alice:x:1000:1000") {
		t.Fatalf("expected alice entry, got:\n%s", got)
	}
	if !strings.Contains(string(got), "mysql:x:27") {
		t.Fatalf("expected mysql entry preserved, got:\n%s", got)
	}
}

func TestMergeGroupAddsToWheel(t *testing.T) {
	root := setup(t,
		"root:x:0:0:root:/root:/bin/bash\n",
		"root:x:0:\nwheel:x:10:bob\nsudo:x:27:\n",
		"",
	)
	out := t.TempDir()
	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/bash", Gecos: "alice"}
	if err := h.MergeFromRoot(root, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "group"))
	body := string(got)
	if !strings.Contains(body, "wheel:x:10:bob,alice") {
		t.Fatalf("expected wheel members bob,alice, got:\n%s", body)
	}
	if !strings.Contains(body, "sudo:x:27:alice") {
		t.Fatalf("expected sudo with alice, got:\n%s", body)
	}
	if !strings.Contains(body, "alice:x:1000:alice") {
		t.Fatalf("expected primary group, got:\n%s", body)
	}
}

func TestMergeGroupSkipsHostGID(t *testing.T) {
	// Container already has a group with GID matching the host's primary GID.
	// We must drop it (filter by field3) so the host's group entry replaces it.
	root := setup(t,
		"root:x:0:0:root:/root:/bin/bash\n",
		"root:x:0:\nfoogroup:x:1000:\n",
		"",
	)
	out := t.TempDir()
	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/bash", Gecos: "alice"}
	if err := h.MergeFromRoot(root, out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "group"))
	if strings.Contains(string(got), "foogroup:x:1000") {
		t.Fatalf("expected foogroup dropped, got:\n%s", got)
	}
}

func TestMergeShadowMode0600(t *testing.T) {
	root := setup(t,
		"root:x:0:0:root:/root:/bin/bash\n",
		"root:x:0:\n",
		"root:!:18000:0:99999:7:::\nfoo:!:18000:0:99999:7:::\n",
	)
	out := t.TempDir()
	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/bash", Gecos: "alice"}
	if err := h.MergeFromRoot(root, out); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(out, "shadow"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("shadow mode = %o, want 0600", st.Mode().Perm())
	}
}

func TestEnsureOverlayUserFirstRun(t *testing.T) {
	root := setup(t,
		"root:x:0:0:root:/root:/bin/bash\nfoo:x:1000:1000:Foo:/home/foo:/bin/sh\n",
		"root:x:0:\nwheel:x:10:\n",
		"root:!:18000:0:99999:7:::\n",
	)
	overlay := t.TempDir()
	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/bash", Gecos: "alice"}
	if err := h.EnsureOverlayUser(root, overlay); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(overlay, "passwd"))
	if !strings.Contains(string(got), "alice:x:1000:1000") {
		t.Fatalf("missing alice in overlay: %s", got)
	}
}

func TestEnsureOverlayUserUpdatesUIDChange(t *testing.T) {
	overlay := t.TempDir()
	// Pre-existing overlay with alice at UID 1001 (stale).
	must := func(name, body string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(overlay, name), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	must("passwd", "root:x:0:0:root:/root:/bin/bash\nalice:x:1001:1001:alice:/home/alice:/bin/zsh\n", 0o644)
	must("group", "root:x:0:\nalice:x:1001:\nwheel:x:10:bob\n", 0o644)
	must("shadow", "alice:!:18000:0:99999:7:::\n", 0o600)

	h := &HostIdentity{User: "alice", UID: 1000, GID: 1000, Group: "alice", Home: "/home/alice", Shell: "/bin/bash", Gecos: "alice"}
	if err := h.EnsureOverlayUser("/nonexistent", overlay); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(overlay, "passwd"))
	if !strings.Contains(string(got), "alice:x:1000:1000") {
		t.Fatalf("expected alice updated to UID 1000, got:\n%s", got)
	}
	if strings.Contains(string(got), "alice:x:1001") {
		t.Fatalf("expected stale entry removed, got:\n%s", got)
	}
	gr, _ := os.ReadFile(filepath.Join(overlay, "group"))
	if !strings.Contains(string(gr), "wheel:x:10:bob,alice") {
		t.Fatalf("expected wheel with alice appended, got:\n%s", gr)
	}
}

func TestAppendUserToGroupNoDuplicate(t *testing.T) {
	in := []string{"wheel:x:10:alice,bob"}
	out := appendUserToGroup(in, "wheel", "alice", 9999)
	if out[0] != "wheel:x:10:alice,bob" {
		t.Fatalf("unexpected duplicate: %v", out)
	}
}

func TestAppendUserToGroupSkipsWhenGIDMatches(t *testing.T) {
	in := []string{"wheel:x:1000:"}
	out := appendUserToGroup(in, "wheel", "alice", 1000)
	if out[0] != "wheel:x:1000:" {
		t.Fatalf("expected unchanged when host GID matches, got %v", out)
	}
}
