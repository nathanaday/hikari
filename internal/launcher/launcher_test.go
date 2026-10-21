package launcher

import (
	"slices"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/store"
)

func TestSystemMounts(t *testing.T) {
	got := SystemMounts("/code/shop", "services/orders", []string{"proto"}, []string{"node_modules"}, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop/services/orders", Target: "/workspace/services/orders"},
		{Source: "/code/shop/proto", Target: "/workspace/proto", ReadOnly: true},
		{Source: "/tmp/empty", Target: "/workspace/services/orders/.hikari", ReadOnly: true},
		{Source: "/code/shop/services/orders/.hikari/private", Target: "/hikari/private"},
		{Source: VolumeName("/code/shop", "services/orders", "node_modules"), Target: "/workspace/services/orders/node_modules", Volume: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("M1 to M4:\n got %+v\nwant %+v", got, want)
	}
	for _, m := range got {
		if strings.HasSuffix(m.Target, "/.git") {
			t.Error("M5: a scoped system must not mount anything at .git")
		}
	}
}

func TestWholeRepoScopeHidesGit(t *testing.T) {
	got := SystemMounts("/code/shop", "", nil, nil, "/tmp/empty")
	want := []Mount{
		{Source: "/code/shop", Target: "/workspace"},
		{Source: "/tmp/empty", Target: "/workspace/.hikari", ReadOnly: true},
		{Source: "/code/shop/.hikari/private", Target: "/hikari/private"},
		{Source: "/tmp/empty", Target: "/workspace/.git", ReadOnly: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("M5:\n got %+v\nwant %+v", got, want)
	}
}

func TestVolumeNames(t *testing.T) {
	a := VolumeName("/code/shop", "web", "node_modules")
	if a != VolumeName("/code/shop", "web", "node_modules") {
		t.Error("M4: volume names must be stable between runs")
	}
	if a == VolumeName("/code/shop", "api", "node_modules") || a == VolumeName("/other", "web", "node_modules") {
		t.Error("M4: different systems must get different volumes")
	}
	if !strings.HasPrefix(a, "hikari-cache-") {
		t.Errorf("volume name %q", a)
	}
}

func TestRunArgs(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "")
	l := &Launcher{RunID: "r1"}
	args := strings.Join(l.runArgs(Spec{
		Name:    "orders",
		Image:   "hikari-node-abc",
		WorkDir: "/workspace/services/orders",
		Mounts: []Mount{
			{Source: "/code/my,shop/orders", Target: "/workspace/orders"},
			{Source: "/code/shop/proto", Target: "/workspace/proto", ReadOnly: true},
			{Source: "hikari-cache-1", Target: "/workspace/orders/node_modules", Volume: true},
		},
	}), " ")
	for _, want := range []string{
		"--name hikari-r1-orders",
		"--network hikari-r1-orders",
		"-e CLAUDE_CODE_OAUTH_TOKEN",
		"--workdir /workspace/services/orders",
		`--mount type=bind,"source=/code/my,shop/orders",target=/workspace/orders`,
		"--mount type=bind,source=/code/shop/proto,target=/workspace/proto,readonly",
		"--mount type=volume,source=hikari-cache-1,target=/workspace/orders/node_modules",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("run args lack %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "secret") || strings.Contains(args, "ANTHROPIC_API_KEY") {
		t.Errorf("secrets or unset variables in args:\n%s", args)
	}
	if !strings.HasSuffix(args, " hikari-node-abc") {
		t.Errorf("the image must be the last argument:\n%s", args)
	}
}

func TestOrchestratorGetsNoMounts(t *testing.T) {
	args := strings.Join((&Launcher{RunID: "r1"}).runArgs(Spec{Name: "lead", Image: "hikari-node"}), " ")
	if strings.Contains(args, "--workdir") {
		t.Errorf("M6: an agent without a system got a working directory:\n%s", args)
	}
}

func TestImageTag(t *testing.T) {
	spec := ImageSpec{Packages: []string{"python3"}, Setup: "echo hi", Path: []string{"/opt/bin"}}
	tag := imageTag("hikari-node", "sha256:base1", spec)
	if !strings.HasPrefix(tag, "hikari-node-") || tag != imageTag("hikari-node", "sha256:base1", spec) {
		t.Errorf("I2 tag %q is not stable", tag)
	}
	changes := []ImageSpec{
		{Packages: []string{"python3", "git"}, Setup: "echo hi", Path: []string{"/opt/bin"}},
		{Packages: []string{"python3"}, Setup: "echo bye", Path: []string{"/opt/bin"}},
		{Packages: []string{"python3"}, Setup: "echo hi", Path: []string{"/usr/bin"}},
	}
	for _, c := range changes {
		if imageTag("hikari-node", "sha256:base1", c) == tag {
			t.Errorf("I2: %+v did not change the tag", c)
		}
	}
	if imageTag("hikari-node", "sha256:base2", spec) == tag {
		t.Error("I2: a new base image did not change the tag")
	}
}

func TestDockerfile(t *testing.T) {
	got := dockerfile("hikari-node", ImageSpec{Packages: []string{"python3", "make"}, Setup: "echo hi", Path: []string{"/usr/local/go/bin", "/opt/bin"}})
	want := `FROM hikari-node
USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 make \
 && rm -rf /var/lib/apt/lists/*
COPY setup.sh /tmp/hikari-setup.sh
RUN bash -euo pipefail /tmp/hikari-setup.sh && rm /tmp/hikari-setup.sh
ENV PATH=/usr/local/go/bin:/opt/bin:$PATH
USER agent
`
	if got != want {
		t.Errorf("I1 Dockerfile:\n%s\nwant:\n%s", got, want)
	}
	if (ImageSpec{}).empty() != true || (ImageSpec{Setup: "  "}).empty() != true {
		t.Error("an image spec that adds nothing must count as empty")
	}
}

func TestStoreMounts(t *testing.T) {
	st := store.Store{Root: "/s"}
	owner := StoreMounts(st, "orders", true, nil, []string{"inventory"}, "/tmp/empty")
	wantOwner := []Mount{
		{Source: "/s/shared", Target: "/hikari/public/readonly/shared", ReadOnly: true},
		{Source: "/s/systems/orders", Target: "/hikari/public/export", ReadOnly: true},
		{Source: "/s/systems/orders/staging", Target: "/hikari/public/export/staging"},
		{Source: "/s/systems/inventory/releases", Target: "/hikari/public/readonly/systems/inventory/releases", ReadOnly: true},
	}
	if !slices.Equal(owner, wantOwner) {
		t.Errorf("owner mounts:\n got %+v\nwant %+v", owner, wantOwner)
	}

	lead := StoreMounts(st, "lead", false, []string{"orders"}, []string{"orders", "billing"}, "/tmp/empty")
	wantLead := []Mount{
		{Source: "/s/shared", Target: "/hikari/public/readonly/shared", ReadOnly: true},
		{Source: "/s/systems/orders", Target: "/hikari/public/readonly/systems/orders", ReadOnly: true},
		{Source: "/tmp/empty", Target: "/hikari/public/readonly/systems/orders/staging", ReadOnly: true},
		{Source: "/s/systems/billing/releases", Target: "/hikari/public/readonly/systems/billing/releases", ReadOnly: true},
	}
	if !slices.Equal(lead, wantLead) {
		t.Errorf("orchestrator mounts:\n got %+v\nwant %+v", lead, wantLead)
	}

	member := StoreMounts(st, "writer", false, nil, nil, "/tmp/empty")
	if len(member) != 1 || member[0].Target != "/hikari/public/readonly/shared" {
		t.Errorf("a member without grants sees only shared: %+v", member)
	}
}
