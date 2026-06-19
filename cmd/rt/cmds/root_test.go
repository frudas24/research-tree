package cmds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCLIInitAndInitFailsIfExists verifies that init creates a root and a second init fails.
func TestCLIInitAndInitFailsIfExists(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	if _, err := runCLI(t, "--research-root", root, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "init"); err == nil {
		t.Fatalf("expected second init to fail")
	}
}

// TestCLIInitDefaultStorageFormatIsBin verifies that init defaults to binary mode.
func TestCLIInitDefaultStorageFormatIsBin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	if _, err := runCLI(t, "--research-root", root, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes.bin")); err != nil {
		t.Fatalf("nodes.bin missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes.idx")); err != nil {
		t.Fatalf("nodes.idx missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes")); !os.IsNotExist(err) {
		t.Fatalf("nodes/ should not exist in binary mode, got err=%v", err)
	}
}

// TestCLIInitDefaultStorageFormatFromEnv verifies RESEARCH_TREE_FORMAT defaulting.
func TestCLIInitDefaultStorageFormatFromEnv(t *testing.T) {
	t.Setenv("RESEARCH_TREE_FORMAT", "json")
	root := filepath.Join(t.TempDir(), "research")
	if _, err := runCLI(t, "--research-root", root, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes")); err != nil {
		t.Fatalf("nodes/ missing for json mode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes.bin")); !os.IsNotExist(err) {
		t.Fatalf("nodes.bin should not exist in json mode, got err=%v", err)
	}
}

// TestCLIInitDefaultResearchRootFromEnv verifies RESEARCH_ROOT and RESEARCH_TREE_ROOT.
func TestCLIInitDefaultResearchRootFromEnv(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "explicit-root")
	t.Setenv("RESEARCH_ROOT", explicit)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init with RESEARCH_ROOT failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(explicit, "meta.json")); err != nil {
		t.Fatalf("meta.json missing at RESEARCH_ROOT: %v", err)
	}

	base := t.TempDir()
	t.Setenv("RESEARCH_ROOT", "")
	t.Setenv("RESEARCH_TREE_ROOT", base)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init with RESEARCH_TREE_ROOT failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, ".research", "meta.json")); err != nil {
		t.Fatalf("meta.json missing at RESEARCH_TREE_ROOT/.research: %v", err)
	}
}

// TestCLINodeCreateShowEditDeleteTreeStatus exercises create, show, edit, delete, tree, and status commands.
func TestCLINodeCreateShowEditDeleteTreeStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")

	out, err := runCLI(t, "--research-root", root, "node", "create", "--title", "test")
	if err != nil || !strings.Contains(out, "created node 0001") {
		t.Fatalf("create output=%q err=%v", out, err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "create"); err == nil {
		t.Fatalf("expected create without title to fail")
	}

	jsonOut, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("show json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	_, err = runCLI(t, "--research-root", root, "node", "edit", "1", "--status", "done", "--outcome", "success")
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	_, err = runCLI(t, "--research-root", root, "node", "create", "--title", "child", "--parents", "1")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "delete", "1"); err == nil {
		t.Fatalf("expected delete without force to fail")
	}
	if _, err := runCLI(t, "--research-root", root, "node", "delete", "1", "--force"); err != nil {
		t.Fatalf("force delete: %v", err)
	}

	if out, err := runCLI(t, "--research-root", root, "tree"); err != nil || strings.TrimSpace(out) == "" {
		t.Fatalf("tree output=%q err=%v", out, err)
	}
	if out, err := runCLI(t, "--research-root", root, "status"); err != nil || !strings.Contains(out, "Nodes:") {
		t.Fatalf("status output=%q err=%v", out, err)
	}
}

// TestCLITreeRenderingModes verifies all tree rendering modes (multiple roots, subtree, depth, flat, json).
func TestCLITreeRenderingModes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root2")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child", "--parents", "1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "grand", "--parents", "3")

	tree, err := runCLI(t, "--research-root", root, "tree")
	if err != nil || !strings.Contains(tree, "0001") || !strings.Contains(tree, "0002") {
		t.Fatalf("tree multiple roots output=%q err=%v", tree, err)
	}
	sub, err := runCLI(t, "--research-root", root, "tree", "3")
	if err != nil || strings.Contains(sub, "0002") {
		t.Fatalf("subtree output=%q err=%v", sub, err)
	}
	depth, err := runCLI(t, "--research-root", root, "tree", "--depth", "1")
	if err != nil || strings.Contains(depth, "0004") {
		t.Fatalf("depth output=%q err=%v", depth, err)
	}
	flat, err := runCLI(t, "--research-root", root, "tree", "--flat")
	if err != nil || strings.Contains(flat, "  0003") {
		t.Fatalf("flat output=%q err=%v", flat, err)
	}
	js, err := runCLI(t, "--research-root", root, "--json", "tree")
	if err != nil {
		t.Fatalf("tree json err=%v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(js), &payload); err != nil {
		t.Fatalf("invalid tree json: %v", err)
	}
}

// TestCLIGoldenMilestonesRender verifies golden milestone metadata is visible in human CLI views.
func TestCLIGoldenMilestonesRender(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, err := runCLI(t,
		"--research-root", root,
		"node", "create",
		"--title", "breakthrough node",
		"--status", "done",
		"--outcome", "success",
		"--claim-status", "validated",
		"--milestone-class", "golden",
		"--milestone-kind", "breakthrough",
		"--milestone-reason", "broke a frontier",
	)
	if err != nil {
		t.Fatalf("create golden node: %v", err)
	}

	show, err := runCLI(t, "--research-root", root, "node", "show", "1", "--view", "summary")
	if err != nil {
		t.Fatalf("node show: %v", err)
	}
	if !strings.Contains(show, "★ breakthrough node") || !strings.Contains(show, "milestone: golden / breakthrough — broke a frontier") {
		t.Fatalf("golden show output missing expected markers: %q", show)
	}

	tree, err := runCLI(t, "--research-root", root, "tree")
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if !strings.Contains(tree, "★ breakthrough node") {
		t.Fatalf("golden tree output missing expected marker: %q", tree)
	}

	status, err := runCLI(t, "--research-root", root, "status", "--verbose", "--limit", "5")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(status, "★ breakthrough node") {
		t.Fatalf("golden status output missing expected marker: %q", status)
	}

	feed, err := runCLI(t, "--research-root", root, "feed", "--days", "365", "--limit", "5")
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	if !strings.Contains(feed, "★ breakthrough node") {
		t.Fatalf("golden feed output missing expected marker: %q", feed)
	}

	list, err := runCLI(t, "--research-root", root, "node", "list", "--milestone-class", "golden")
	if err != nil {
		t.Fatalf("node list: %v", err)
	}
	if !strings.Contains(list, "★ breakthrough node") {
		t.Fatalf("golden list output missing expected marker: %q", list)
	}
}

// TestCLIGoldenCommand verifies the dedicated golden command is a thin human wrapper
// over canonical milestone queries.
func TestCLIGoldenCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, err := runCLI(t,
		"--research-root", root,
		"node", "create",
		"--title", "gold champion",
		"--status", "done",
		"--outcome", "success",
		"--claim-status", "validated",
		"--milestone-class", "golden",
		"--milestone-kind", "champion",
		"--milestone-reason", "best current lineage artifact",
	)
	if err != nil {
		t.Fatalf("create champion golden node: %v", err)
	}
	_, err = runCLI(t,
		"--research-root", root,
		"node", "create",
		"--title", "gold breakthrough",
		"--status", "done",
		"--outcome", "success",
		"--claim-status", "validated",
		"--milestone-class", "golden",
		"--milestone-kind", "breakthrough",
		"--milestone-reason", "moved the frontier",
	)
	if err != nil {
		t.Fatalf("create breakthrough golden node: %v", err)
	}

	out, err := runCLI(t, "--research-root", root, "golden", "--kind", "champion", "--verbose")
	if err != nil {
		t.Fatalf("golden command: %v", err)
	}
	if !strings.Contains(out, "golden/champion") || !strings.Contains(out, "★ gold champion") || !strings.Contains(out, "best current lineage artifact") {
		t.Fatalf("unexpected golden command output: %q", out)
	}
	if strings.Contains(out, "gold breakthrough") {
		t.Fatalf("golden --kind filter leaked other kinds: %q", out)
	}

	js, err := runCLI(t, "--research-root", root, "--json", "golden", "--kind", "breakthrough")
	if err != nil {
		t.Fatalf("golden --json: %v", err)
	}
	var payload []map[string]any
	if err := json.Unmarshal([]byte(js), &payload); err != nil {
		t.Fatalf("invalid golden json: %v", err)
	}
	if len(payload) != 1 || payload[0]["title"] != "gold breakthrough" {
		t.Fatalf("unexpected golden json payload: %+v", payload)
	}
}

// TestCLIArtifactTagRecoveryStorageAlert exercises artifact, tag, recovery, storage, and alert commands.
func TestCLIArtifactTagRecoveryStorageAlert(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "ancestor", "--status", "done", "--outcome", "success", "--agent", "opus")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "refuter", "--status", "done", "--outcome", "success")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "active", "--parents", "1", "--agent", "opus")

	metrics := filepath.Join(t.TempDir(), "metrics.json")
	if err := os.WriteFile(metrics, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "--research-root", root, "artifact", "embed", "1", "--file", metrics); err != nil {
		t.Fatalf("artifact embed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "artifact", "remove", "1", "--path", "/tmp/does-not-exist"); err != nil {
		t.Fatalf("artifact remove noop: %v", err)
	}

	if _, err := runCLI(t, "--research-root", root, "tag", "add", "1", "a,b"); err != nil {
		t.Fatalf("tag add: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "tag", "rm", "1", "a"); err != nil {
		t.Fatalf("tag rm: %v", err)
	}

	if _, err := runCLI(t, "--research-root", root, "node", "invalidate", "1", "--by", "2", "--reason", "test"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	alerts, err := runCLI(t, "--research-root", root, "alert", "list", "--agent", "opus", "--only-unacked")
	if err != nil || !strings.Contains(alerts, "warn_") {
		t.Fatalf("alert list output=%q err=%v", alerts, err)
	}
	parts := strings.Split(strings.TrimSpace(alerts), " ")
	warnID := parts[0]
	if _, err := runCLI(t, "--research-root", root, "alert", "ack", warnID); err != nil {
		t.Fatalf("alert ack: %v", err)
	}

	if _, err := runCLI(t, "--research-root", root, "recovery", "list"); err != nil {
		t.Fatalf("recovery list: %v", err)
	}
	snaps, err := runCLI(t, "--research-root", root, "recovery", "list")
	if err != nil {
		t.Fatalf("recovery list (2): %v", err)
	}
	first := strings.TrimSpace(strings.Split(snaps, "\n")[0])
	snapshotID := strings.TrimSpace(strings.Split(first, " | ")[0])
	if _, err := runCLI(t, "--research-root", root, "recovery", "restore", snapshotID); err != nil {
		t.Fatalf("recovery restore: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "storage", "migrate", "--to", "bin"); err != nil {
		t.Fatalf("storage migrate: %v", err)
	}
}

// TestCLIResourceInventoryAndAutoRelease verifies resource inventory, claims, and automatic release on close.
func TestCLIResourceInventoryAndAutoRelease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "gpu experiment")

	if _, err := runCLI(t, "--research-root", root, "resource", "add",
		"--id", "gpu-node-0",
		"--label", "gpu-node-0 gpu0",
		"--kind", "gpu",
		"--endpoint", "10.0.0.14",
		"--endpoint-kind", "ip",
		"--tags", "cuda,24gb",
	); err != nil {
		t.Fatalf("resource add: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "resource", "claim", "1", "gpu-node-0", "--by", "codex"); err != nil {
		t.Fatalf("resource claim: %v", err)
	}
	show, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("node show: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(show), &payload); err != nil {
		t.Fatalf("invalid node json: %v", err)
	}
	activeResources, _ := payload["active_resources"].([]any)
	if len(activeResources) != 1 {
		t.Fatalf("expected one active resource, got %d", len(activeResources))
	}
	if _, err := runCLI(t, "--research-root", root, "node", "close", "1", "--outcome", "success"); err != nil {
		t.Fatalf("node close: %v", err)
	}
	report, err := runCLI(t, "--research-root", root, "resource", "report")
	if err != nil {
		t.Fatalf("resource report: %v", err)
	}
	if strings.Contains(report, "node: #0001") {
		t.Fatalf("expected leases released after close, got report=%q", report)
	}
	history, err := runCLI(t, "--research-root", root, "resource", "history", "gpu-node-0")
	if err != nil {
		t.Fatalf("resource history: %v", err)
	}
	if !strings.Contains(history, "claim") || !strings.Contains(history, "auto_release_done") {
		t.Fatalf("expected claim and auto release events, got %q", history)
	}
}

// TestCLILogrunResourceFields verifies logrun uses resource_id and endpoint semantics.
func TestCLILogrunResourceFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "gpu experiment")
	_, _ = runCLI(t, "--research-root", root, "resource", "add",
		"--id", "gpu-node-0",
		"--label", "gpu-node-0 gpu0",
		"--kind", "gpu",
		"--endpoint", "10.0.0.14",
		"--endpoint-kind", "ip",
	)
	_, _ = runCLI(t, "--research-root", root, "resource", "claim", "1", "gpu-node-0", "--by", "codex")
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1",
		"--resource-id", "gpu-node-0",
		"--cmd", "python bench.py",
		"--outdir", "/tmp/run",
	); err != nil {
		t.Fatalf("logrun: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	runs, _ := payload["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %d", len(runs))
	}
	firstRun, _ := runs[0].(map[string]any)
	if firstRun["resource_id"] != "gpu-node-0" {
		t.Fatalf("expected resource_id persisted, got %+v", firstRun)
	}
	if firstRun["endpoint"] != "10.0.0.14" {
		t.Fatalf("expected endpoint derived from resource, got %+v", firstRun)
	}
	if firstRun["endpoint_kind"] != "ip" {
		t.Fatalf("expected endpoint_kind persisted, got %+v", firstRun)
	}
}

// TestCLIResourceBusyAndMaintenanceErrors verifies clear occupancy and maintenance errors.
func TestCLIResourceBusyAndMaintenanceErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "owner")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "waiter")
	_, _ = runCLI(t, "--research-root", root, "resource", "add",
		"--id", "busy-gpu0",
		"--label", "busy gpu0",
		"--kind", "gpu",
		"--enabled", "true",
	)
	_, _ = runCLI(t, "--research-root", root, "resource", "claim", "1", "busy-gpu0", "--by", "codex")
	if _, err := runCLI(t, "--research-root", root, "resource", "claim", "2", "busy-gpu0", "--by", "other"); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected busy error with blocker details, got %v", err)
	}
	_, _ = runCLI(t, "--research-root", root, "resource", "add",
		"--id", "maint-gpu0",
		"--label", "maint gpu0",
		"--kind", "gpu",
		"--enabled", "true",
		"--maintenance", "true",
	)
	if _, err := runCLI(t, "--research-root", root, "resource", "claim", "2", "maint-gpu0"); err == nil || !strings.Contains(err.Error(), "maintenance") {
		t.Fatalf("expected maintenance error, got %v", err)
	}
}

// TestCLIStatusJSONHasExpectedFields verifies status --json output contains expected keys.
func TestCLIStatusJSONHasExpectedFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "a")
	out, err := runCLI(t, "--research-root", root, "--json", "status")
	if err != nil {
		t.Fatalf("status --json err=%v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid status json: %v", err)
	}
	for _, k := range []string{
		"total",
		"active",
		"done",
		"paused",
		"warnings",
		"agent",
		"status_counts",
		"claim_status_counts",
		"outcome_counts",
		"run_validity_counts",
		"matrix",
		"hotspot_formula",
		"hotspots",
	} {
		if _, ok := payload[k]; !ok {
			t.Fatalf("missing key %s in status payload", k)
		}
	}
}

// TestCLIStatusHotspotsExplainFormula verifies hotspot formula transparency in text and JSON outputs.
func TestCLIStatusHotspotsExplainFormula(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child-a", "--parents", "1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child-b", "--parents", "1")

	textOut, err := runCLI(t, "--research-root", root, "status", "--section", "hotspots")
	if err != nil {
		t.Fatalf("status hotspots text: %v", err)
	}
	for _, want := range []string{"formula:", "pending=", "bonus="} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("missing %q in hotspot text output: %q", want, textOut)
		}
	}

	jsonOut, err := runCLI(t, "--research-root", root, "--json", "status")
	if err != nil {
		t.Fatalf("status hotspots json: %v", err)
	}
	var payload struct {
		HotspotFormula string `json:"hotspot_formula"`
		Hotspots       []struct {
			PendingWeight     int `json:"pending_weight"`
			InconclusiveBonus int `json:"inconclusive_bonus"`
		} `json:"hotspots"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("invalid hotspot json: %v", err)
	}
	if !strings.Contains(payload.HotspotFormula, "pending_children*5") {
		t.Fatalf("unexpected hotspot formula: %q", payload.HotspotFormula)
	}
	if len(payload.Hotspots) == 0 {
		t.Fatalf("expected at least one hotspot")
	}
	if payload.Hotspots[0].PendingWeight != 5 {
		t.Fatalf("unexpected hotspot payload: %+v", payload.Hotspots[0])
	}
}

// TestCLIStatusFilterByTag verifies status filtering by tag in json mode.
func TestCLIStatusFilterByTag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "with-tag", "--tags", "abc")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "without-tag")
	out, err := runCLI(t, "--research-root", root, "--json", "status", "--tag", "abc")
	if err != nil {
		t.Fatalf("status --tag --json err=%v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid status json: %v", err)
	}
	if int(payload["total"].(float64)) != 1 {
		t.Fatalf("expected total=1 for tag filter, got %v", payload["total"])
	}
}

// TestCLIStatusSectionFiltersTextOutput verifies --section scopes detailed output.
func TestCLIStatusSectionFiltersTextOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "active-a")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "done-b", "--status", "done", "--outcome", "success")
	out, err := runCLI(t, "--research-root", root, "status", "--section", "done", "--limit", "1")
	if err != nil {
		t.Fatalf("status --section done err=%v", err)
	}
	if strings.Contains(out, "Active:") {
		t.Fatalf("unexpected Active section in done-only output: %q", out)
	}
	if !strings.Contains(out, "Done (completed):") {
		t.Fatalf("expected done section, got: %q", out)
	}
}

// TestCLIStatusMatrixAndLimit verifies matrix rendering and row limits.
func TestCLIStatusMatrixAndLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "a1", "--agent", "x")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "a2", "--agent", "x")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "d1", "--status", "done", "--outcome", "failure", "--agent", "x")
	out, err := runCLI(t, "--research-root", root, "status", "--verbose", "--limit", "1", "--matrix")
	if err != nil {
		t.Fatalf("status --verbose --limit --matrix err=%v", err)
	}
	if !strings.Contains(out, "Status x Outcome Matrix:") {
		t.Fatalf("matrix section missing: %q", out)
	}
	if !strings.Contains(out, "... 1 more") {
		t.Fatalf("expected limit truncation marker, got: %q", out)
	}
}

// TestCLIStatusJSONBidirectionalGraph verifies that status --json summaries
// include both parents and children for bidirectional graph navigation.
func TestCLIStatusJSONBidirectionalGraph(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child1", "--parents", "1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child2", "--parents", "1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "grandchild", "--parents", "2")

	out, err := runCLI(t, "--research-root", root, "--json", "status")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	active, ok := payload["active"].([]any)
	if !ok {
		t.Fatal("active is not an array")
	}

	// Find summaries by title
	find := func(title string) map[string]any {
		for _, a := range active {
			m := a.(map[string]any)
			if m["title"] == title {
				return m
			}
		}
		return nil
	}

	rootNode := find("root")
	if rootNode == nil {
		t.Fatal("root not found in active")
	}
	parents, _ := rootNode["parents"].([]any)
	children, _ := rootNode["children"].([]any)
	if len(parents) != 0 {
		t.Fatalf("root should have 0 parents, got %v", parents)
	}
	if len(children) != 2 {
		t.Fatalf("root should have 2 children, got %v", children)
	}

	child1 := find("child1")
	if child1 == nil {
		t.Fatal("child1 not found")
	}
	c1Parents, _ := child1["parents"].([]any)
	c1Children, _ := child1["children"].([]any)
	if len(c1Parents) != 1 || int(c1Parents[0].(float64)) != 1 {
		t.Fatalf("child1 parents mismatch: %v", c1Parents)
	}
	if len(c1Children) != 1 || int(c1Children[0].(float64)) != 4 {
		t.Fatalf("child1 children mismatch: %v", c1Children)
	}

	grand := find("grandchild")
	if grand == nil {
		t.Fatal("grandchild not found")
	}
	gcParents, _ := grand["parents"].([]any)
	gcChildren, _ := grand["children"].([]any)
	if len(gcParents) != 1 || int(gcParents[0].(float64)) != 2 {
		t.Fatalf("grandchild parents mismatch: %v", gcParents)
	}
	if len(gcChildren) != 0 {
		t.Fatalf("grandchild should be leaf, got children=%v", gcChildren)
	}
}

// TestCLINodeCreateConcurrentNoCollision verifies concurrent node creation produces no ID collisions.
func TestCLINodeCreateConcurrentNoCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := runCLI(t, "--research-root", root, "node", "create", "--title", "t")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create err: %v", err)
		}
	}
	out, err := runCLI(t, "--research-root", root, "--json", "node", "list")
	if err != nil {
		t.Fatalf("node list json: %v", err)
	}
	var ids []int
	if err := json.Unmarshal([]byte(out), &ids); err != nil {
		t.Fatalf("bad list json: %v", err)
	}
	if len(ids) != n {
		t.Fatalf("expected %d nodes, got %d", n, len(ids))
	}
}

// TestCLITreeCycleCutGuard verifies that tree rendering handles cycles gracefully.
func TestCLITreeCycleCutGuard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init", "--storage-format", "json")
	_ = os.WriteFile(filepath.Join(root, "nodes", "0001.json"), []byte(`{"schema_version":1,"id":1,"title":"a","status":"active","claim_status":"provisional","parents":[2]}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "nodes", "0002.json"), []byte(`{"schema_version":1,"id":2,"title":"b","status":"active","claim_status":"provisional","parents":[1]}`), 0o644)
	done := make(chan struct{})
	var out string
	var err error
	go func() {
		out, err = runCLI(t, "--research-root", root, "tree", "1")
		close(done)
	}()
	select {
	case <-done:
		if err != nil {
			t.Fatalf("tree err: %v", err)
		}
		if !strings.Contains(out, "cycle-cut") {
			t.Fatalf("expected cycle-cut guard, got %q", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tree command hung on cycle")
	}
}

// TestCLINewFlagsBodyParentsEdit verifies the human-friendly flags: --body inline,
// --parents with title substring, --edit body replace, and node show formatting.
func TestCLINewFlagsBodyParentsEdit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")

	// --body inline
	_, err := runCLI(t, "--research-root", root, "node", "create", "--title", "inline test", "--body", "# Hello\nworld")
	if err != nil {
		t.Fatalf("create with --body: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil || !strings.Contains(out, "# Hello") || !strings.Contains(out, "─── body ───") {
		t.Fatalf("show with body: output=%q err=%v", out, err)
	}

	// --body from file
	tmpBody := filepath.Join(t.TempDir(), "body.md")
	_ = os.WriteFile(tmpBody, []byte("file content"), 0o644)
	_, err = runCLI(t, "--research-root", root, "node", "create", "--title", "file test", "--body-file", tmpBody)
	if err != nil {
		t.Fatalf("create with --body-file: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "2")
	if !strings.Contains(out, "file content") {
		t.Fatalf("body-file content missing: %q", out)
	}

	// --parents with title substring (fuzzy match)
	_, err = runCLI(t, "--research-root", root, "node", "create", "--title", "child", "--parents", "inline")
	if err != nil {
		t.Fatalf("create with fuzzy parent: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "3")
	if !strings.Contains(out, "parents:  0001") {
		t.Fatalf("fuzzy parent not resolved: %q", out)
	}

	// --parents ambiguous should fail
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "another inline", "--body", "x")
	_, err = runCLI(t, "--research-root", root, "node", "create", "--title", "bad", "--parents", "test")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %v", err)
	}

	// node edit --body replaces
	_, err = runCLI(t, "--research-root", root, "node", "edit", "1", "--body", "replaced")
	if err != nil {
		t.Fatalf("edit --body: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "1")
	if !strings.Contains(out, "replaced") || strings.Contains(out, "# Hello") {
		t.Fatalf("body not replaced: %q", out)
	}

	// node edit --body-file replaces
	_ = os.WriteFile(tmpBody, []byte("from file replace"), 0o644)
	_, err = runCLI(t, "--research-root", root, "node", "edit", "2", "--body-file", tmpBody)
	if err != nil {
		t.Fatalf("edit --body-file: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "2")
	if !strings.Contains(out, "from file replace") {
		t.Fatalf("body-file replace missing: %q", out)
	}

	// node edit --claim-status updates epistemic state
	_, err = runCLI(t, "--research-root", root, "node", "edit", "2", "--claim-status", "validated")
	if err != nil {
		t.Fatalf("edit --claim-status: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "2")
	if !strings.Contains(out, "claim: validated") {
		t.Fatalf("claim-status not updated: %q", out)
	}

	// node show format includes metadata fields
	out, _ = runCLI(t, "--research-root", root, "node", "show", "1")
	for _, field := range []string{"status:", "claim:", "agent:", "created:", "modified:"} {
		if !strings.Contains(out, field) {
			t.Fatalf("show missing field %q: %q", field, out)
		}
	}

	out, _ = runCLI(t, "--research-root", root, "node", "show", "1", "--view", "summary")
	if strings.Contains(out, "─── body ───") || !strings.Contains(out, "body: present") {
		t.Fatalf("summary view should hide body, got %q", out)
	}
}

// TestCLINodeEditReparent verifies reparenting via node edit --parents and cycle rejection.
func TestCLINodeEditReparent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "A")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "B", "--parents", "1")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "C")

	// Reparent B from A to C using title reference.
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "2", "--parents", "C"); err != nil {
		t.Fatalf("reparent edit failed: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "2")
	if err != nil {
		t.Fatalf("show json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	parents, _ := payload["parents"].([]any)
	if len(parents) != 1 || int(parents[0].(float64)) != 3 {
		t.Fatalf("unexpected parents after reparent: %v", parents)
	}

	// Reject cycle: C -> B while B -> C.
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "3", "--parents", "2"); err == nil {
		t.Fatal("expected cycle rejection, got nil")
	}
}

// TestCLINodeImportAndBatch verifies import and batch update workflows.
func TestCLINodeImportAndBatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")

	importFile := filepath.Join(t.TempDir(), "nodes.json")
	content := `{"nodes":[{"title":"n1","status":"active","claim_status":"provisional"},{"title":"n2","status":"active","claim_status":"provisional"}]}`
	if err := os.WriteFile(importFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "import", "--file", importFile); err != nil {
		t.Fatalf("node import failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "batch", "--filter-status", "active", "--set-status", "paused"); err != nil {
		t.Fatalf("node batch failed: %v", err)
	}
	statusJSON, err := runCLI(t, "--research-root", root, "--json", "status")
	if err != nil {
		t.Fatalf("status json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(statusJSON), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	active, _ := payload["active"].([]any)
	paused, _ := payload["paused"].([]any)
	if len(active) != 0 || len(paused) != 2 {
		t.Fatalf("unexpected status split active=%d paused=%d", len(active), len(paused))
	}
}

// TestCLINodeListAdvancedFiltersAndAtomicParentEdits verifies advanced
// filtering plus additive/removal parent edits.
func TestCLINodeListAdvancedFiltersAndAtomicParentEdits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "gamma", "--body", "one", "--tags", "a,b", "--status", "done", "--outcome", "success")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "alpha", "--body", "needle", "--tags", "a,c", "--status", "done", "--outcome", "success")
	_, _ = runCLI(t, "--research-root", root, "artifact", "add", "2", "--host", "gpu-node-0", "--path", "/tmp/a", "--desc", "probe")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "beta", "--tags", "c", "--status", "done", "--outcome", "failure")

	out, err := runCLI(t, "--research-root", root, "node", "list", "--tags-all", "a,c", "--has-artifact", "true", "--sort-by", "title")
	if err != nil || !strings.Contains(out, "0002 | done | provisional | clean | alpha") {
		t.Fatalf("advanced list output=%q err=%v", out, err)
	}

	out, err = runCLI(t, "--research-root", root, "node", "list", "--sort-by", "title", "--offset", "1", "--limit", "1")
	if err != nil || !strings.Contains(out, "0003 | done | provisional | clean | beta") {
		t.Fatalf("offset list output=%q err=%v", out, err)
	}

	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "child", "--parents", "4")
	_, err = runCLI(t, "--research-root", root, "node", "edit", "5", "--add-parents", "2")
	if err != nil {
		t.Fatalf("add-parents failed: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "5")
	if !strings.Contains(out, "parents:  0002, 0004") {
		t.Fatalf("expected dual parents after add-parents: %q", out)
	}
	_, err = runCLI(t, "--research-root", root, "node", "edit", "5", "--rm-parents", "4")
	if err != nil {
		t.Fatalf("rm-parents failed: %v", err)
	}
	out, _ = runCLI(t, "--research-root", root, "node", "show", "5")
	if !strings.Contains(out, "parents:  0002") || strings.Contains(out, "0004") {
		t.Fatalf("expected parent removed: %q", out)
	}
}

func TestCLINodePoisonAndRevalidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "contaminated")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "clean rerun")

	if _, err := runCLI(t, "--research-root", root, "node", "poison", "1",
		"--cause", "base_snapshot",
		"--scope", "qwen@host",
		"--reason", "base corrupted",
		"--by", "2"); err != nil {
		t.Fatalf("node poison failed: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil {
		t.Fatalf("node show after poison failed: %v", err)
	}
	for _, want := range []string{"evidence: poisoned / base_snapshot", "poisoned by: [2] — base corrupted"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output: %q", want, out)
		}
	}

	if _, err := runCLI(t, "--research-root", root, "node", "revalidate", "1", "--by", "2"); err != nil {
		t.Fatalf("node revalidate failed: %v", err)
	}
	out, err = runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil {
		t.Fatalf("node show after revalidate failed: %v", err)
	}
	if !strings.Contains(out, "revalidated by: [2]") {
		t.Fatalf("expected revalidated_by in output: %q", out)
	}
}

func TestTreePrimaryParentAvoidsCycleCutForMultiParentReference(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root-a")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root-b")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "shared", "--parents", "1,2", "--primary-parent", "1")

	out, err := runCLI(t, "--research-root", root, "tree")
	if err != nil {
		t.Fatalf("tree failed: %v", err)
	}
	if strings.Contains(out, "...cycle-cut...") {
		t.Fatalf("unexpected cycle-cut for primary-parent traversal: %q", out)
	}
}

// TestCLINewViews verifies mermaid/changes/timeline commands produce output.
func TestCLINewViews(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "R")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "C", "--parents", "1")

	mermaid, err := runCLI(t, "--research-root", root, "mermaid")
	if err != nil {
		t.Fatalf("mermaid failed: %v", err)
	}
	if !strings.Contains(mermaid, "flowchart") || !strings.Contains(mermaid, "N1 --> N2") {
		t.Fatalf("unexpected mermaid output: %q", mermaid)
	}
	subtree, err := runCLI(t, "--research-root", root, "mermaid", "2")
	if err != nil {
		t.Fatalf("mermaid subtree failed: %v", err)
	}
	if strings.Contains(subtree, "N1[") || !strings.Contains(subtree, "N2[") {
		t.Fatalf("unexpected subtree mermaid output: %q", subtree)
	}

	changes, err := runCLI(t, "--research-root", root, "changes", "--limit", "2")
	if err != nil {
		t.Fatalf("changes failed: %v", err)
	}
	if !strings.Contains(changes, "0001") && !strings.Contains(changes, "0002") {
		t.Fatalf("unexpected changes output: %q", changes)
	}

	timeline, err := runCLI(t, "--research-root", root, "timeline", "--days", "30", "--limit", "5")
	if err != nil {
		t.Fatalf("timeline failed: %v", err)
	}
	if !strings.Contains(timeline, "───") {
		t.Fatalf("unexpected timeline output: %q", timeline)
	}

	feed, err := runCLI(t, "--research-root", root, "feed", "--by", "created", "--limit", "2")
	if err != nil {
		t.Fatalf("feed failed: %v", err)
	}
	if !strings.Contains(feed, "{created}") || !strings.Contains(feed, "0002") {
		t.Fatalf("unexpected feed output: %q", feed)
	}
}

// TestCLIWorkflowCommands verifies close/logrun/link end-to-end behavior.
func TestCLIWorkflowCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "workflow target")

	if _, err := runCLI(t, "--research-root", root, "node", "close", "1"); err == nil {
		t.Fatal("expected close without outcome to fail")
	}
	if _, err := runCLI(t, "--research-root", root, "node", "close", "1", "--outcome", "success", "--append-body", "close note"); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1",
		"--endpoint", "gpu-node-0.int.lab",
		"--endpoint-kind", "dns",
		"--cmd", "python train.py --seed 7",
		"--outdir", "/tmp/run-7",
		"--seed", "7",
		"--eta", "2h",
		"--cost", "$3",
		"--note", "baseline",
		"--artifact-desc", "baseline-run",
	); err != nil {
		t.Fatalf("logrun failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "link", "1",
		"--commit", "none",
		"--artifact", "/tmp/report.json",
		"--host", "gpu-node-0",
		"--artifact-desc", "report",
	); err != nil {
		t.Fatalf("link failed: %v", err)
	}

	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["status"] != "done" || payload["outcome"] != "success" {
		t.Fatalf("unexpected workflow status payload: %v", payload)
	}
	body, _ := payload["body"].(string)
	if !strings.Contains(body, "close note") || strings.Contains(body, "### run-meta") {
		t.Fatalf("workflow body missing expected data: %q", body)
	}
	artifacts, _ := payload["artifacts"].([]any)
	if len(artifacts) != 2 {
		t.Fatalf("expected two artifacts after logrun+link, got %d", len(artifacts))
	}
	runs, _ := payload["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("expected one structured run after logrun, got %d", len(runs))
	}
}

// TestCLILogrunProjectBodyKeepsSingleLatestBlock verifies optional body projection stays deduplicated.
func TestCLILogrunProjectBodyKeepsSingleLatestBlock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "projection target", "--body", "narrative")
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1", "--cmd", "python first.py", "--project-body"); err != nil {
		t.Fatalf("first logrun failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1", "--cmd", "python second.py", "--project-body"); err != nil {
		t.Fatalf("second logrun failed: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	body, _ := payload["body"].(string)
	if strings.Count(body, "### run-meta") != 1 {
		t.Fatalf("expected exactly one run-meta block, got body=%q", body)
	}
	if !strings.Contains(body, "python second.py") || strings.Contains(body, "python first.py") {
		t.Fatalf("expected only latest projected run, got body=%q", body)
	}
}

// TestCLINodeDiffRevisions verifies semantic revision diffs in text and JSON modes.
func TestCLINodeDiffRevisions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	if _, err := runCLI(t, "--research-root", root, "node", "create", "--title", "alpha", "--body", "body one"); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1", "--status", "done", "--outcome", "success", "--append-body", "body two"); err != nil {
		t.Fatalf("edit failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1", "--claim-status", "validated", "--add-tags", "x,y"); err != nil {
		t.Fatalf("second edit failed: %v", err)
	}

	textOut, err := runCLI(t, "--research-root", root, "node", "diff", "1", "--rev-a", "1", "--rev-b", "3")
	if err != nil {
		t.Fatalf("node diff text: %v", err)
	}
	for _, want := range []string{"Diff for 0001 rev 1 -> rev 3", "status:", "body:", "claim_status:", "tags:"} {
		if !strings.Contains(textOut, want) {
			t.Fatalf("missing %q in diff text output: %q", want, textOut)
		}
	}
	if !strings.Contains(textOut, `"body one\nbody two"`) {
		t.Fatalf("expected newline-normalized append in diff output: %q", textOut)
	}

	jsonOut, err := runCLI(t, "--research-root", root, "--json", "node", "diff", "1", "--rev-a", "1", "--rev-b", "3")
	if err != nil {
		t.Fatalf("node diff json: %v", err)
	}
	var payload struct {
		RevisionA     float64  `json:"revision_a"`
		RevisionB     float64  `json:"revision_b"`
		ChangedFields []string `json:"changed_fields"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &payload); err != nil {
		t.Fatalf("invalid diff json: %v", err)
	}
	if payload.RevisionA != 1 || payload.RevisionB != 3 {
		t.Fatalf("unexpected revisions: %+v", payload)
	}
	for _, want := range []string{"status", "claim_status", "body", "tags"} {
		if !containsString(payload.ChangedFields, want) {
			t.Fatalf("missing changed field %q in %+v", want, payload.ChangedFields)
		}
	}
}

// TestCLIAppendBodyNormalizesNewline verifies append-body inserts one separator newline when needed.
func TestCLIAppendBodyNormalizesNewline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "append target", "--body", "line one")
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1", "--append-body", "line two"); err != nil {
		t.Fatalf("edit append failed: %v", err)
	}
	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1")
	if err != nil {
		t.Fatalf("show json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload["body"] != "line one\nline two" {
		t.Fatalf("unexpected normalized body: %q", payload["body"])
	}
}

// TestCLISemanticScalingPhase1 verifies visible verdict badges, latest run
// rendering, and compact agent handoff view.
func TestCLISemanticScalingPhase1(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "base hypothesis", "--body", "Focus: verify sparse decoder boundary")
	if _, err := runCLI(t, "--research-root", root, "node", "close", "1", "--outcome", "failure"); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1", "--claim-status", "superseded"); err != nil {
		t.Fatalf("claim-status update failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1",
		"--endpoint", "gpu-node-0.int.lab",
		"--endpoint-kind", "dns",
		"--cmd", "python eval.py --seed 17",
		"--outdir", "/tmp/eval-17",
		"--seed", "17",
		"--note", "handoff baseline",
	); err != nil {
		t.Fatalf("logrun failed: %v", err)
	}

	show, err := runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(show, "base hypothesis [superseded]") {
		t.Fatalf("expected verdict badge in show: %q", show)
	}
	if !strings.Contains(show, "latest run:") || !strings.Contains(show, "python eval.py --seed 17") || !strings.Contains(show, "/tmp/eval-17") {
		t.Fatalf("expected latest run section in show: %q", show)
	}

	list, err := runCLI(t, "--research-root", root, "node", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(list, "base hypothesis [superseded]") {
		t.Fatalf("expected verdict badge in list: %q", list)
	}

	tree, err := runCLI(t, "--research-root", root, "tree")
	if err != nil {
		t.Fatalf("tree failed: %v", err)
	}
	if !strings.Contains(tree, "base hypothesis [superseded]") {
		t.Fatalf("expected verdict badge in tree: %q", tree)
	}

	agentView, err := runCLI(t, "--research-root", root, "node", "show", "1", "--agent")
	if err != nil {
		t.Fatalf("show --agent failed: %v", err)
	}
	if !strings.Contains(agentView, "latest_run:") || !strings.Contains(agentView, "endpoint: gpu-node-0.int.lab") || !strings.Contains(agentView, "summary: Focus: verify sparse decoder boundary") {
		t.Fatalf("unexpected agent view: %q", agentView)
	}

	agentJSON, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1", "--agent")
	if err != nil {
		t.Fatalf("show --agent --json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(agentJSON), &payload); err != nil {
		t.Fatalf("invalid agent json: %v", err)
	}
	if payload["display_title"] != "base hypothesis [superseded]" {
		t.Fatalf("unexpected display_title: %v", payload["display_title"])
	}
	if payload["handoff_version"] != "v1" {
		t.Fatalf("unexpected handoff_version: %v", payload["handoff_version"])
	}
	latestRun, ok := payload["latest_run"].(map[string]any)
	if !ok || latestRun["endpoint"] != "gpu-node-0.int.lab" || latestRun["endpoint_kind"] != "dns" || latestRun["seed"] != "17" {
		t.Fatalf("unexpected latest_run payload: %v", payload["latest_run"])
	}
	if _, ok := payload["lineage"].(map[string]any); !ok {
		t.Fatalf("expected lineage object, got %T", payload["lineage"])
	}
	if _, ok := payload["evidence"].(map[string]any); !ok {
		t.Fatalf("expected evidence object, got %T", payload["evidence"])
	}
}

// TestCLISemanticScalingPhase2 verifies additive semantic fields are persisted and shown.
func TestCLISemanticScalingPhase2(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "phase2 root")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "phase2 continuation")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "phase2 superseder")

	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1",
		"--scope", "mistral-q4km ctx=2048 greedy",
		"--exit-criteria", "close after three reproducible seeds",
		"--continued-by", "2",
		"--superseded-by", "3",
	); err != nil {
		t.Fatalf("semantic edit failed: %v", err)
	}

	show, err := runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(show, "scope: mistral-q4km ctx=2048 greedy") ||
		!strings.Contains(show, "exit criteria: close after three reproducible seeds") ||
		!strings.Contains(show, "continued by: 0002") ||
		!strings.Contains(show, "superseded by: 0003") {
		t.Fatalf("semantic show output missing fields: %q", show)
	}

	agentJSON, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1", "--agent")
	if err != nil {
		t.Fatalf("show --agent --json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(agentJSON), &payload); err != nil {
		t.Fatalf("invalid semantic agent json: %v", err)
	}
	if payload["scope"] != "mistral-q4km ctx=2048 greedy" || payload["exit_criteria"] != "close after three reproducible seeds" {
		t.Fatalf("unexpected semantic json fields: %v", payload)
	}
	continuedBy, _ := payload["continued_by"].([]any)
	supersededBy, _ := payload["superseded_by"].([]any)
	if len(continuedBy) != 1 || len(supersededBy) != 1 || int(continuedBy[0].(float64)) != 2 || int(supersededBy[0].(float64)) != 3 {
		t.Fatalf("unexpected semantic link payloads: continued=%v superseded=%v", continuedBy, supersededBy)
	}
}

// TestCLISemanticScalingPhase3 verifies structured runs are persisted and surfaced.
func TestCLISemanticScalingPhase3(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "phase3 run node", "--body", "Benchmark sparse decoder")

	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1",
		"--endpoint", "10.0.0.64",
		"--endpoint-kind", "ip",
		"--cmd", "python bench.py --seed 11",
		"--outdir", "/tmp/bench-11",
		"--seed", "11",
		"--eta", "45m",
		"--cost", "$1.20",
		"--note", "phase3 structured",
	); err != nil {
		t.Fatalf("logrun failed: %v", err)
	}

	out, err := runCLI(t, "--research-root", root, "--json", "node", "show", "1", "--agent")
	if err != nil {
		t.Fatalf("show --agent --json failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid phase3 json: %v", err)
	}
	runs, _ := payload["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("expected one structured run, got %d", len(runs))
	}
	latestRun, _ := payload["latest_run"].(map[string]any)
	if latestRun["endpoint"] != "10.0.0.64" || latestRun["endpoint_kind"] != "ip" || latestRun["outdir"] != "/tmp/bench-11" || latestRun["seed"] != "11" {
		t.Fatalf("unexpected latest run payload: %v", latestRun)
	}
	if latestRun["valid"] != true {
		t.Fatalf("expected valid structured run, got %v", latestRun)
	}
	show, err := runCLI(t, "--research-root", root, "node", "show", "1")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(show, "latest run:") || !strings.Contains(show, "endpoint:  10.0.0.64 (ip)") || !strings.Contains(show, "cmd:       python bench.py --seed 11") {
		t.Fatalf("expected structured latest run in show: %q", show)
	}
}

// TestCLISemanticScalingPhase4 verifies query filters and run-validity summaries.
func TestCLISemanticScalingPhase4(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "scope node", "--scope", "mistral ctx=2048")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "next node")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "superseding node")
	if _, err := runCLI(t, "--research-root", root, "node", "edit", "1", "--continued-by", "2", "--superseded-by", "3"); err != nil {
		t.Fatalf("semantic links edit failed: %v", err)
	}
	if _, err := runCLI(t, "--research-root", root, "node", "logrun", "1", "--cmd", "python bench.py", "--valid=false", "--invalid-reason", "wrong target"); err != nil {
		t.Fatalf("invalid logrun failed: %v", err)
	}
	list, err := runCLI(t, "--research-root", root, "node", "list", "--scope-contains", "ctx=2048")
	if err != nil || !strings.Contains(list, "scope node") {
		t.Fatalf("scope list failed output=%q err=%v", list, err)
	}
	list, err = runCLI(t, "--research-root", root, "node", "list", "--continued-by", "2")
	if err != nil || !strings.Contains(list, "scope node") {
		t.Fatalf("continued-by list failed output=%q err=%v", list, err)
	}
	list, err = runCLI(t, "--research-root", root, "node", "list", "--superseded-by", "3")
	if err != nil || !strings.Contains(list, "scope node") {
		t.Fatalf("superseded-by list failed output=%q err=%v", list, err)
	}
	statusJSON, err := runCLI(t, "--research-root", root, "--json", "status", "--scope-contains", "ctx=2048")
	if err != nil {
		t.Fatalf("status scope filter failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(statusJSON), &payload); err != nil {
		t.Fatalf("invalid status json: %v", err)
	}
	runValidity, ok := payload["run_validity_counts"].(map[string]any)
	if !ok || runValidity["invalid"] != float64(1) {
		t.Fatalf("unexpected run_validity_counts: %v", payload["run_validity_counts"])
	}
	statusText, err := runCLI(t, "--research-root", root, "status", "--section", "runs")
	if err != nil || !strings.Contains(statusText, "Run Validity:") || !strings.Contains(statusText, "invalid: 1") {
		t.Fatalf("run validity section missing output=%q err=%v", statusText, err)
	}
}

// TestCLIRelationsAndLinks verifies typed relations, primary parent rendering, and the links command.
func TestCLIRelationsAndLinks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "root")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "baseline")
	if _, err := runCLI(t,
		"--research-root", root,
		"node", "create",
		"--title", "candidate",
		"--parents", "1,2",
		"--primary-parent", "1",
		"--relation", "compares_against:2,inspired_by:1",
	); err != nil {
		t.Fatalf("create relation node failed: %v", err)
	}

	show, err := runCLI(t, "--research-root", root, "node", "show", "3")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(show, "primary parent: 0001") || !strings.Contains(show, "relations: compares_against:0002") {
		t.Fatalf("show missing primary parent/relations: %q", show)
	}

	links, err := runCLI(t, "--research-root", root, "--json", "links")
	if err != nil {
		t.Fatalf("links json failed: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(links), &entries); err != nil {
		t.Fatalf("invalid links json: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 links (2 parents + 2 relations), got %d", len(entries))
	}

	filtered, err := runCLI(t, "--research-root", root, "links", "--type", "compares_against")
	if err != nil {
		t.Fatalf("links filter failed: %v", err)
	}
	if !strings.Contains(filtered, "compares_against") || strings.Contains(filtered, "parent") {
		t.Fatalf("unexpected filtered links output: %q", filtered)
	}
}

// TestCLILintAuditsRelationHygiene verifies lint surfaces relation and graph hygiene issues.
func TestCLILintAuditsRelationHygiene(t *testing.T) {
	root := filepath.Join(t.TempDir(), "research")
	_, _ = runCLI(t, "--research-root", root, "init")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "lonely")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "parent-a")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "parent-b")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "parent-c")
	_, _ = runCLI(t, "--research-root", root, "node", "create", "--title", "parent-d")
	if _, err := runCLI(t,
		"--research-root", root,
		"node", "create",
		"--title", "crowded",
		"--parents", "2,3,4,5",
		"--relation", "compares_against:999,compares_against:999",
	); err != nil {
		t.Fatalf("create lint fixture failed: %v", err)
	}

	out, err := runCLI(t, "--research-root", root, "lint", "--max-parents", "4")
	if err != nil {
		t.Fatalf("lint failed: %v", err)
	}
	for _, needle := range []string{
		"has 4 parents",
		"relation compares_against:999 targets non-existent node",
		"duplicate relation compares_against:999",
		"isolated node: no parents and no children",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("lint output missing %q: %q", needle, out)
		}
	}
}

// containsString reports whether a string slice contains the requested value.
func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// runCLI executes the CLI root command with the given arguments and returns its output.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	buf := new(strings.Builder)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}
