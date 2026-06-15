package retree

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// --- helpers ---

// fullTestNode creates a fully populated test node.
func fullTestNode() *Node {
	n := &Node{
		Frontmatter: Frontmatter{
			ID:              42,
			Title:           "KD sparse/hybrid experiment",
			Status:          StatusDone,
			ClaimStatus:     ClaimValidated,
			Scope:           "mistral-q4km ctx=2048 greedy",
			ExitCriteria:    "close after 3 seeds",
			Parents:         []NodeID{1, 2},
			ContinuedBy:     []NodeID{77},
			SupersededBy:    []NodeID{88},
			Agent:           "researcher",
			Tags:            []string{"kd", "sparse", "experimento"},
			Created:         time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC),
			Modified:        time.Date(2026, 5, 31, 15, 30, 0, 0, time.UTC),
			MilestoneClass:  MilestoneGolden,
			MilestoneKind:   MilestoneKindBreakthrough,
			MilestoneReason: "compressed teacher substrate by orders of magnitude",
			Relations: []Relation{
				{Type: RelComparesAgainst, Target: 7, Note: "baseline"},
				{Type: RelInspiredBy, Target: 8},
			},
		},
		Commits: []GitCommit{
			{Hash: "abc123def", Message: "flag --top-k"},
			{Hash: "def456abc", Message: "fix reshape bug"},
		},
		Runs: []RunRecord{
			{Timestamp: time.Date(2026, 5, 31, 14, 5, 0, 0, time.UTC), Host: "gpu-node-0", Command: "python train.py --seed 7", OutDir: "/tmp/run-7", Seed: "7", ETA: "2h", Cost: "$3", Note: "baseline", Valid: boolPtr(true)},
		},
		Artifacts: []Artifact{
			{Mode: ArtifactPath, Host: "gpu-node-0", Path: "/tmp/kd_sparse_t9k_k128", Description: "model weights", SizeBytes: 6920601},
			{Mode: ArtifactEmbedded, Path: "artifacts/0042/metrics.json", Description: "metrics", SizeBytes: 2048},
		},
		InvalidatedBy:      []NodeID{99},
		InvalidationReason: "bad assumption about k=128",
		Body:               "## Hypothesis\nk=128 gives better coverage than k=64.\n",
	}
	ApplyNodeDefaults(n, n.Created)
	pp := NodeID(1)
	n.PrimaryParent = &pp
	return n
}

// roundtripJSON roundtrips a node through JSON marshal/unmarshal for comparison.
func roundtripJSON(t *testing.T, n *Node) *Node {
	t.Helper()
	b, err := MarshalNodeJSON(n)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	out, err := UnmarshalNodeJSON(b)
	if err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	return out
}

// --- tests ---

// TestBinaryRoundtripFull verifies binary marshal/unmarshal preserves all fields.
func TestBinaryRoundtripFull(t *testing.T) {
	n := fullTestNode()
	b, err := MarshalNodeBinary(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	jsonRef := roundtripJSON(t, n)

	if got.ID != jsonRef.ID {
		t.Fatalf("id mismatch: %d vs %d", got.ID, jsonRef.ID)
	}
	if got.Title != jsonRef.Title {
		t.Fatalf("title mismatch: %q vs %q", got.Title, jsonRef.Title)
	}
	if got.Status != jsonRef.Status {
		t.Fatalf("status mismatch: %q vs %q", got.Status, jsonRef.Status)
	}
	if got.ClaimStatus != jsonRef.ClaimStatus {
		t.Fatalf("claim status mismatch: %q vs %q", got.ClaimStatus, jsonRef.ClaimStatus)
	}
	if !reflect.DeepEqual(got.Parents, jsonRef.Parents) {
		t.Fatalf("parents mismatch: %v vs %v", got.Parents, jsonRef.Parents)
	}
	if got.Scope != jsonRef.Scope || got.ExitCriteria != jsonRef.ExitCriteria {
		t.Fatalf("semantic strings mismatch: scope=%q/%q exit=%q/%q", got.Scope, jsonRef.Scope, got.ExitCriteria, jsonRef.ExitCriteria)
	}
	if got.MilestoneClass != jsonRef.MilestoneClass || got.MilestoneKind != jsonRef.MilestoneKind || got.MilestoneReason != jsonRef.MilestoneReason {
		t.Fatalf("milestone mismatch: class=%q/%q kind=%q/%q reason=%q/%q", got.MilestoneClass, jsonRef.MilestoneClass, got.MilestoneKind, jsonRef.MilestoneKind, got.MilestoneReason, jsonRef.MilestoneReason)
	}
	if !reflect.DeepEqual(got.Relations, jsonRef.Relations) {
		t.Fatalf("relations mismatch: %+v vs %+v", got.Relations, jsonRef.Relations)
	}
	if (got.PrimaryParent == nil) != (jsonRef.PrimaryParent == nil) || (got.PrimaryParent != nil && *got.PrimaryParent != *jsonRef.PrimaryParent) {
		t.Fatalf("primary parent mismatch: %v vs %v", got.PrimaryParent, jsonRef.PrimaryParent)
	}
	if !reflect.DeepEqual(got.ContinuedBy, jsonRef.ContinuedBy) || !reflect.DeepEqual(got.SupersededBy, jsonRef.SupersededBy) {
		t.Fatalf("semantic links mismatch: continued=%v/%v superseded=%v/%v", got.ContinuedBy, jsonRef.ContinuedBy, got.SupersededBy, jsonRef.SupersededBy)
	}
	if got.Agent != jsonRef.Agent {
		t.Fatalf("agent mismatch: %q vs %q", got.Agent, jsonRef.Agent)
	}
	if !reflect.DeepEqual(got.Tags, jsonRef.Tags) {
		t.Fatalf("tags mismatch: %v vs %v", got.Tags, jsonRef.Tags)
	}
	if !got.Created.Equal(jsonRef.Created) {
		t.Fatalf("created mismatch: %v vs %v", got.Created, jsonRef.Created)
	}
	if !got.Modified.Equal(jsonRef.Modified) {
		t.Fatalf("modified mismatch: %v vs %v", got.Modified, jsonRef.Modified)
	}
	if !reflect.DeepEqual(got.Commits, jsonRef.Commits) {
		t.Fatalf("commits mismatch: %+v vs %+v", got.Commits, jsonRef.Commits)
	}
	if !reflect.DeepEqual(got.Runs, jsonRef.Runs) {
		t.Fatalf("runs mismatch: %+v vs %+v", got.Runs, jsonRef.Runs)
	}
	if !reflect.DeepEqual(got.Artifacts, jsonRef.Artifacts) {
		t.Fatalf("artifacts mismatch: %+v vs %+v", got.Artifacts, jsonRef.Artifacts)
	}
	if !reflect.DeepEqual(got.InvalidatedBy, jsonRef.InvalidatedBy) {
		t.Fatalf("invalidated_by mismatch: %v vs %v", got.InvalidatedBy, jsonRef.InvalidatedBy)
	}
	if got.InvalidationReason != jsonRef.InvalidationReason {
		t.Fatalf("invalidation_reason mismatch: %q vs %q", got.InvalidationReason, jsonRef.InvalidationReason)
	}
	if got.Body != jsonRef.Body {
		t.Fatalf("body mismatch: %q vs %q", got.Body, jsonRef.Body)
	}

	// Size sanity: binary should be smaller than JSON for non-trivial nodes
	jb, _ := MarshalNodeJSON(n)
	t.Logf("json=%d bytes  bin=%d bytes", len(jb), len(b))
	if len(b) >= len(jb) {
		t.Logf("warning: binary not smaller than json for this node (may be normal for small payloads)")
	}
}

// TestBinaryRoundtripMinimum verifies a minimal node roundtrips correctly.
func TestBinaryRoundtripMinimum(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "only title"}}
	ApplyNodeDefaults(n, time.Now())
	b, err := MarshalNodeBinary(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "only title" {
		t.Fatalf("title mismatch: %q", got.Title)
	}
	if got.Status != StatusActive {
		t.Fatalf("status default mismatch: %q", got.Status)
	}
	if got.ClaimStatus != ClaimProvisional {
		t.Fatalf("claim default mismatch: %q", got.ClaimStatus)
	}
	if len(got.Parents) != 0 || len(got.Tags) != 0 || len(got.Commits) != 0 || len(got.Artifacts) != 0 {
		t.Fatalf("expected empty slices, got parents=%v tags=%v commits=%v artifacts=%v", got.Parents, got.Tags, got.Commits, got.Artifacts)
	}
	if got.Body != "" {
		t.Fatalf("body should be empty: %q", got.Body)
	}
}

// TestBinaryRoundtripEmptyBody verifies empty body is preserved.
func TestBinaryRoundtripEmptyBody(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "no body"}}
	ApplyNodeDefaults(n, time.Now())
	n.Body = ""
	b, err := MarshalNodeBinary(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Body != "" {
		t.Fatalf("expected empty body: %q", got.Body)
	}
}

// marshalNodeBinaryLegacyV1 encodes the pre-semantic-extension binary payload.
func marshalNodeBinaryLegacyV1(n *Node) ([]byte, error) {
	if n == nil {
		return nil, errors.New("nil node")
	}
	var buf bytes.Buffer
	binWriteU16(&buf, uint16(n.SchemaVersion))
	binWriteU64(&buf, uint64(n.ID))
	binWriteString(&buf, n.Title)
	binWriteU8(&buf, statusToBin[n.Status])
	binWriteU8(&buf, claimToBin[n.ClaimStatus])
	binWriteU8(&buf, outcomeToBin[n.Outcome])
	binWriteU64Slice(&buf, n.Parents)
	binWriteString(&buf, n.Agent)
	binWriteU16(&buf, uint16(len(n.Tags)))
	for _, t := range n.Tags {
		binWriteString(&buf, t)
	}
	binWriteI64(&buf, timeToBin(n.Created))
	binWriteI64(&buf, timeToBin(n.Modified))
	binWriteU64(&buf, n.Revision)
	binWriteU16(&buf, uint16(len(n.Commits)))
	for _, c := range n.Commits {
		binWriteString(&buf, c.Hash)
		binWriteString32(&buf, c.Message)
	}
	binWriteU16(&buf, uint16(len(n.Artifacts)))
	for _, a := range n.Artifacts {
		binWriteU8(&buf, artifactModeToBin[a.Mode])
		binWriteString(&buf, a.Host)
		binWriteString(&buf, a.Path)
		binWriteString(&buf, a.Description)
		binWriteI64(&buf, a.SizeBytes)
	}
	binWriteU64Slice(&buf, n.InvalidatedBy)
	binWriteString(&buf, n.InvalidationReason)
	binWriteString32(&buf, n.Body)
	return buf.Bytes(), nil
}

// marshalNodeBinaryPhase3 encodes the structured-run payload before run validity fields.
func marshalNodeBinaryPhase3(n *Node) ([]byte, error) {
	if n == nil {
		return nil, errors.New("nil node")
	}
	var buf bytes.Buffer
	binWriteU16(&buf, uint16(n.SchemaVersion))
	binWriteU64(&buf, uint64(n.ID))
	binWriteString(&buf, n.Title)
	binWriteU8(&buf, statusToBin[n.Status])
	binWriteU8(&buf, claimToBin[n.ClaimStatus])
	binWriteU8(&buf, outcomeToBin[n.Outcome])
	binWriteU64Slice(&buf, n.Parents)
	binWriteString(&buf, n.Agent)
	binWriteU16(&buf, uint16(len(n.Tags)))
	for _, t := range n.Tags {
		binWriteString(&buf, t)
	}
	binWriteI64(&buf, timeToBin(n.Created))
	binWriteI64(&buf, timeToBin(n.Modified))
	binWriteU64(&buf, n.Revision)
	binWriteU16(&buf, uint16(len(n.Commits)))
	for _, c := range n.Commits {
		binWriteString(&buf, c.Hash)
		binWriteString32(&buf, c.Message)
	}
	binWriteU16(&buf, uint16(len(n.Artifacts)))
	for _, a := range n.Artifacts {
		binWriteU8(&buf, artifactModeToBin[a.Mode])
		binWriteString(&buf, a.Host)
		binWriteString(&buf, a.Path)
		binWriteString(&buf, a.Description)
		binWriteI64(&buf, a.SizeBytes)
	}
	binWriteU64Slice(&buf, n.InvalidatedBy)
	binWriteString(&buf, n.InvalidationReason)
	binWriteString32(&buf, n.Body)
	binWriteString32(&buf, n.Scope)
	binWriteString32(&buf, n.ExitCriteria)
	binWriteU64Slice(&buf, n.ContinuedBy)
	binWriteU64Slice(&buf, n.SupersededBy)
	binWriteU16(&buf, uint16(len(n.Runs)))
	for _, r := range n.Runs {
		binWriteI64(&buf, timeToBin(r.Timestamp))
		binWriteString32(&buf, r.Host)
		binWriteString32(&buf, r.Command)
		binWriteString32(&buf, r.OutDir)
		binWriteString32(&buf, r.Seed)
		binWriteString32(&buf, r.ETA)
		binWriteString32(&buf, r.Cost)
		binWriteString32(&buf, r.Note)
	}
	return buf.Bytes(), nil
}

// TestBinaryUnmarshalLegacyV1 verifies new code can still load older binary payloads.
func TestBinaryUnmarshalLegacyV1(t *testing.T) {
	n := fullTestNode()
	legacy, err := marshalNodeBinaryLegacyV1(n)
	if err != nil {
		t.Fatalf("legacy marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(legacy)
	if err != nil {
		t.Fatalf("legacy unmarshal: %v", err)
	}
	if got.Scope != "" || got.ExitCriteria != "" || len(got.ContinuedBy) != 0 || len(got.SupersededBy) != 0 {
		t.Fatalf("legacy payload should default new fields to zero values: %+v", got)
	}
	if got.MilestoneClass != MilestoneNone || got.MilestoneKind != MilestoneKindNone || got.MilestoneReason != "" {
		t.Fatalf("legacy payload should default milestone fields to zero values: %+v", got)
	}
}

// TestBinaryUnmarshalPhase3 verifies compatibility with structured-run payloads
// written before run validity fields existed.
func TestBinaryUnmarshalPhase3(t *testing.T) {
	n := fullTestNode()
	phase3, err := marshalNodeBinaryPhase3(n)
	if err != nil {
		t.Fatalf("phase3 marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(phase3)
	if err != nil {
		t.Fatalf("phase3 unmarshal: %v", err)
	}
	if len(got.Runs) != 1 || got.Runs[0].Host != "gpu-node-0" {
		t.Fatalf("unexpected phase3 runs: %+v", got.Runs)
	}
	if got.Runs[0].Valid != nil || got.Runs[0].InvalidReason != "" {
		t.Fatalf("phase3 payload should default run validity fields: %+v", got.Runs[0])
	}
}

// boolPtr returns a pointer to v for test fixtures.
func boolPtr(v bool) *bool { return &v }

// TestBinaryRoundtripAllEnums verifies all status/claim status enum values roundtrip.
func TestBinaryRoundtripAllEnums(t *testing.T) {
	allStatuses := []NodeStatus{StatusActive, StatusDone, StatusDone, StatusDone, StatusPaused, StatusDone}
	allClaims := []ClaimStatus{ClaimProvisional, ClaimValidated, ClaimInvalidated, ClaimSuperseded}

	for _, st := range allStatuses {
		for _, cl := range allClaims {
			n := &Node{Frontmatter: Frontmatter{Title: "enum test", Status: st, ClaimStatus: cl}}
			ApplyNodeDefaults(n, time.Now())
			b, err := MarshalNodeBinary(n)
			if err != nil {
				t.Fatalf("marshal status=%s claim=%s: %v", st, cl, err)
			}
			got, err := UnmarshalNodeBinary(b)
			if err != nil {
				t.Fatalf("unmarshal status=%s claim=%s: %v", st, cl, err)
			}
			if got.Status != st || got.ClaimStatus != cl {
				t.Fatalf("roundtrip mismatch: got status=%s claim=%s", got.Status, got.ClaimStatus)
			}
		}
	}
}

// TestBinaryRoundtripArtifactModes verifies both artifact modes roundtrip.
func TestBinaryRoundtripArtifactModes(t *testing.T) {
	modes := []ArtifactMode{ArtifactPath, ArtifactEmbedded}
	for _, mode := range modes {
		n := &Node{Frontmatter: Frontmatter{Title: "artifact mode test"}}
		ApplyNodeDefaults(n, time.Now())
		n.Artifacts = []Artifact{
			{Mode: mode, Host: "host1", Path: "/tmp/x", Description: "test", SizeBytes: 100},
		}
		if mode == ArtifactEmbedded {
			n.Artifacts[0].Host = ""
		}
		b, err := MarshalNodeBinary(n)
		if err != nil {
			t.Fatalf("marshal mode=%s: %v", mode, err)
		}
		got, err := UnmarshalNodeBinary(b)
		if err != nil {
			t.Fatalf("unmarshal mode=%s: %v", mode, err)
		}
		if len(got.Artifacts) != 1 || got.Artifacts[0].Mode != mode {
			t.Fatalf("artifact mode mismatch: %+v", got.Artifacts)
		}
	}
}

// TestBinaryRoundtripZeroTimes verifies zero-value times are preserved.
func TestBinaryRoundtripZeroTimes(t *testing.T) {
	n := &Node{Frontmatter: Frontmatter{Title: "zero time"}}
	ApplyNodeDefaults(n, time.Time{})
	n.Created = time.Time{}
	n.Modified = time.Time{}
	b, err := MarshalNodeBinary(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalNodeBinary(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Created.IsZero() || !got.Modified.IsZero() {
		t.Fatalf("expected zero times: created=%v modified=%v", got.Created, got.Modified)
	}
}

// TestBinaryRoundtripLargeBody verifies large bodies (>50KB) survive roundtrip.
func TestBinaryRoundtripLargeBody(t *testing.T) {
	large := make([]byte, 50000)
	for i := range large {
		large[i] = byte('a' + (i % 26))
	}
	n := &Node{Frontmatter: Frontmatter{Title: "large body"}}
	ApplyNodeDefaults(n, time.Now())
	n.Body = string(large)
	b, err := MarshalNodeBinary(n)
	if err != nil {
		t.Fatalf("marshal large body: %v", err)
	}
	got, err := UnmarshalNodeBinary(b)
	if err != nil {
		t.Fatalf("unmarshal large body: %v", err)
	}
	if got.Body != string(large) {
		t.Fatalf("large body mismatch: len=%d vs len=%d", len(got.Body), len(large))
	}
}

// TestBinaryRejectsTruncated verifies truncated payloads produce errors.
func TestBinaryRejectsTruncated(t *testing.T) {
	n := fullTestNode()
	b, _ := MarshalNodeBinary(n)
	// cut at various boundary points
	for _, cut := range []int{1, 3, 10, len(b) / 2, len(b) - 1} {
		_, err := UnmarshalNodeBinary(b[:cut])
		if err == nil {
			t.Fatalf("expected error for truncated payload at %d/%d bytes", cut, len(b))
		}
	}
}

// TestBinaryRejectsTrailingBytes verifies trailing bytes are rejected (strict mode).
func TestBinaryRejectsTrailingBytes(t *testing.T) {
	n := fullTestNode()
	b, _ := MarshalNodeBinary(n)
	extra := append(append([]byte(nil), b...), 0xFF, 0xEE)
	_, err := UnmarshalNodeBinary(extra)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for trailing bytes, got %v", err)
	}
}

// TestBinaryHeaderRoundtrip verifies the file header roundtrip.
func TestBinaryHeaderRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	WriteBinHeader(&buf)
	if buf.Len() != binHeaderSize {
		t.Fatalf("expected header size %d, got %d", binHeaderSize, buf.Len())
	}
	if _, err := ReadBinHeader(buf.Bytes()); err != nil {
		t.Fatalf("read header: %v", err)
	}
}

// TestBinaryHeaderAcceptsLegacyV1 verifies older nodes.bin headers remain readable.
func TestBinaryHeaderAcceptsLegacyV1(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(binMagic)
	buf.WriteByte(binVersionV1)
	buf.Write([]byte{0, 0, 0})
	ver, err := ReadBinHeader(buf.Bytes())
	if err != nil {
		t.Fatalf("read legacy header: %v", err)
	}
	if ver != binVersionV1 {
		t.Fatalf("expected legacy version %d, got %d", binVersionV1, ver)
	}
}

// TestBinaryHeaderRejectsWrongMagic verifies wrong magic bytes are rejected.
func TestBinaryHeaderRejectsWrongMagic(t *testing.T) {
	bad := []byte("XXXX\x01\x00\x00\x00")
	_, err := ReadBinHeader(bad)
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("expected ErrUnsupportedSchema, got %v", err)
	}
}

// TestBinaryHeaderRejectsFutureVersion verifies future format versions are rejected.
func TestBinaryHeaderRejectsFutureVersion(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(binMagic)
	buf.WriteByte(99)
	buf.Write([]byte{0, 0, 0})
	_, err := ReadBinHeader(buf.Bytes())
	if !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("expected ErrUnsupportedSchema for future version, got %v", err)
	}
}

// TestBinaryHeaderRejectsTruncated verifies truncated headers are rejected.
func TestBinaryHeaderRejectsTruncated(t *testing.T) {
	_, err := ReadBinHeader([]byte("RT"))
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

// TestBinaryNilNode verifies marshal of nil node fails.
func TestBinaryNilNode(t *testing.T) {
	_, err := MarshalNodeBinary(nil)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode, got %v", err)
	}
}

// TestBinaryEmptyBuffer verifies unmarshal of empty buffer fails.
func TestBinaryEmptyBuffer(t *testing.T) {
	_, err := UnmarshalNodeBinary(nil)
	if !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode, got %v", err)
	}
}

// TestBinaryDualRoundtripJSONandBIN verifies JSON and binary roundtrips produce identical results.
func TestBinaryDualRoundtripJSONandBIN(t *testing.T) {
	// Node roundtripped through JSON should match binary roundtrip.
	n := fullTestNode()
	jsonB, _ := MarshalNodeJSON(n)
	binB, _ := MarshalNodeBinary(n)

	jsonN, _ := UnmarshalNodeJSON(jsonB)
	binN, _ := UnmarshalNodeBinary(binB)

	// Compare via JSON serialization of both (field-level semantic equality).
	jsonOut, _ := json.Marshal(jsonN)
	binOut, _ := json.Marshal(binN)
	if !bytes.Equal(jsonOut, binOut) {
		t.Fatalf("json and binary roundtrips differ:\njson: %s\nbin:  %s", jsonOut, binOut)
	}
}

// TestBinaryUnknownEnumBytes verifies unknown enum byte values are rejected.
func TestBinaryUnknownEnumBytes(t *testing.T) {
	n := fullTestNode()
	b, _ := MarshalNodeBinary(n)

	// Corrupt status byte (offset after schema=2 + id=8 + title_len=2 + title=N)
	// We need to find the status byte position. Easiest: marshal, find the byte after title.
	// Schema(2) + ID(8) = 10. Then title_len(2) + len(title).
	titleLen := len(n.Title)
	statusPos := 10 + 2 + titleLen
	if statusPos >= len(b) {
		t.Fatal("payload too short")
	}
	corrupted := make([]byte, len(b))
	copy(corrupted, b)
	corrupted[statusPos] = 99 // invalid status

	_, err := UnmarshalNodeBinary(corrupted)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

// TestBinaryRoundtripAllOutcomes verifies all outcome enum values roundtrip.
func TestBinaryRoundtripAllOutcomes(t *testing.T) {
	allStatuses := []NodeStatus{StatusActive, StatusDone, StatusPaused}
	allOutcomes := []Outcome{OutcomeUnset, OutcomeSuccess, OutcomeFailure, OutcomeInconclusive}

	for _, st := range allStatuses {
		for _, oc := range allOutcomes {
			n := &Node{Frontmatter: Frontmatter{Title: "outcome test", Status: st, Outcome: oc}}
			ApplyNodeDefaults(n, time.Now())
			b, err := MarshalNodeBinary(n)
			if err != nil {
				t.Fatalf("marshal status=%s outcome=%s: %v", st, oc, err)
			}
			got, err := UnmarshalNodeBinary(b)
			if err != nil {
				t.Fatalf("unmarshal status=%s outcome=%s: %v", st, oc, err)
			}
			if got.Status != st || got.Outcome != oc {
				t.Fatalf("roundtrip mismatch: got status=%s outcome=%s", got.Status, got.Outcome)
			}
		}
	}
}
