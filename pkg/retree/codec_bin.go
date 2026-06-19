package retree

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// Binary format constants.
const (
	binMagic          = "RTND"
	binVersionV1      = uint8(1)
	binVersionCurrent = uint8(2)
	binHeaderSize     = 8 // 4 magic + 1 version + 3 reserved
	binMaxStrU16      = 65535
	binMaxBodyU32     = 1<<32 - 1
)

// --- enum encodings (deterministic, versioned) ---

var (
	statusToBin = map[NodeStatus]uint8{
		StatusActive: 0,
		StatusDone:   1,
		StatusPaused: 2,
	}
	binToStatus = map[uint8]NodeStatus{
		0: StatusActive,
		1: StatusDone,
		2: StatusPaused,
	}
	claimToBin = map[ClaimStatus]uint8{
		ClaimProvisional: 0,
		ClaimValidated:   1,
		ClaimInvalidated: 2,
		ClaimSuperseded:  3,
	}
	binToClaim = map[uint8]ClaimStatus{
		0: ClaimProvisional,
		1: ClaimValidated,
		2: ClaimInvalidated,
		3: ClaimSuperseded,
	}
	outcomeToBin = map[Outcome]uint8{
		OutcomeUnset:        0,
		OutcomeSuccess:      1,
		OutcomeFailure:      2,
		OutcomeInconclusive: 3,
	}
	binToOutcome = map[uint8]Outcome{
		0: OutcomeUnset,
		1: OutcomeSuccess,
		2: OutcomeFailure,
		3: OutcomeInconclusive,
	}
	artifactModeToBin = map[ArtifactMode]uint8{
		ArtifactPath:     0,
		ArtifactEmbedded: 1,
	}
	binToArtifactMode = map[uint8]ArtifactMode{
		0: ArtifactPath,
		1: ArtifactEmbedded,
	}
	evidenceStatusToBin = map[EvidenceStatus]uint8{
		"":                  0,
		EvidenceClean:       1,
		EvidenceSuspect:     2,
		EvidencePoisoned:    3,
		EvidenceRevalidated: 4,
	}
	binToEvidenceStatus = map[uint8]EvidenceStatus{
		0: "",
		1: EvidenceClean,
		2: EvidenceSuspect,
		3: EvidencePoisoned,
		4: EvidenceRevalidated,
	}
	evidenceCauseToBin = map[EvidenceCause]uint8{
		EvidenceCauseNone:          0,
		EvidenceCauseBaseSnapshot:  1,
		EvidenceCauseToolchain:     2,
		EvidenceCauseExporter:      3,
		EvidenceCauseDataset:       4,
		EvidenceCausePromptSurface: 5,
		EvidenceCauseRuntimeEnv:    6,
		EvidenceCauseUnknown:       7,
	}
	binToEvidenceCause = map[uint8]EvidenceCause{
		0: EvidenceCauseNone,
		1: EvidenceCauseBaseSnapshot,
		2: EvidenceCauseToolchain,
		3: EvidenceCauseExporter,
		4: EvidenceCauseDataset,
		5: EvidenceCausePromptSurface,
		6: EvidenceCauseRuntimeEnv,
		7: EvidenceCauseUnknown,
	}
)

// milestoneClassToBin encodes a MilestoneClass to its binary representation.
func milestoneClassToBin(v MilestoneClass) uint8 {
	switch v {
	case MilestoneGolden:
		return 1
	default:
		return 0
	}
}

// binToMilestoneClass decodes a byte back to a MilestoneClass and validity flag.
func binToMilestoneClass(v uint8) (MilestoneClass, bool) {
	switch v {
	case 0:
		return MilestoneNone, true
	case 1:
		return MilestoneGolden, true
	default:
		return "", false
	}
}

// milestoneKindToBin encodes a MilestoneKind to its binary representation.
func milestoneKindToBin(v MilestoneKind) uint8 {
	switch v {
	case MilestoneKindChampion:
		return 1
	case MilestoneKindBreakthrough:
		return 2
	case MilestoneKindPivot:
		return 3
	default:
		return 0
	}
}

// binToMilestoneKind decodes a byte back to a MilestoneKind and validity flag.
func binToMilestoneKind(v uint8) (MilestoneKind, bool) {
	switch v {
	case 0:
		return MilestoneKindNone, true
	case 1:
		return MilestoneKindChampion, true
	case 2:
		return MilestoneKindBreakthrough, true
	case 3:
		return MilestoneKindPivot, true
	default:
		return "", false
	}
}

// relation type binary encoding (deterministic, versioned)
var (
	relationTypeToBin = map[RelationType]uint8{
		"":                 0,
		RelDependsOn:       1,
		RelComparesAgainst: 2,
		RelInspiredBy:      3,
		RelAggregates:      4,
	}
	binToRelationType = map[uint8]RelationType{
		0: "",
		1: RelDependsOn,
		2: RelComparesAgainst,
		3: RelInspiredBy,
		4: RelAggregates,
	}
)

const relationExtensionMarker uint8 = 0xA1

// MarshalNodeBinary encodes a node into the binary wire format v1.
// Returns the raw bytes without the file header.
func MarshalNodeBinary(n *Node) ([]byte, error) {
	if n == nil {
		return nil, fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	var buf bytes.Buffer

	// schema_version
	binWriteU16(&buf, uint16(n.SchemaVersion))
	// id
	binWriteU64(&buf, uint64(n.ID))
	// title
	binWriteString(&buf, n.Title)
	// status
	bs, ok := statusToBin[n.Status]
	if !ok {
		return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidStatus, n.Status)
	}
	binWriteU8(&buf, bs)
	// claim_status
	bc, ok := claimToBin[n.ClaimStatus]
	if !ok {
		return nil, fmt.Errorf("%w: unknown claim status %q", ErrInvalidClaimStatus, n.ClaimStatus)
	}
	binWriteU8(&buf, bc)
	// outcome
	bo, ok := outcomeToBin[n.Outcome]
	if !ok {
		return nil, fmt.Errorf("%w: unknown outcome %q", ErrInvalidNode, n.Outcome)
	}
	binWriteU8(&buf, bo)
	// parents
	binWriteU64Slice(&buf, n.Parents)
	// agent
	binWriteString(&buf, n.Agent)
	// tags
	binWriteU16(&buf, uint16(len(n.Tags)))
	for _, t := range n.Tags {
		binWriteString(&buf, t)
	}
	// created / modified
	binWriteI64(&buf, timeToBin(n.Created))
	binWriteI64(&buf, timeToBin(n.Modified))
	// revision
	binWriteU64(&buf, n.Revision)
	// commits
	binWriteU16(&buf, uint16(len(n.Commits)))
	for _, c := range n.Commits {
		binWriteString(&buf, c.Hash)
		binWriteString32(&buf, c.Message)
	}
	// artifacts
	binWriteU16(&buf, uint16(len(n.Artifacts)))
	for _, a := range n.Artifacts {
		am, ok := artifactModeToBin[a.Mode]
		if !ok {
			return nil, fmt.Errorf("%w: unknown artifact mode %q", ErrInvalidArtifact, a.Mode)
		}
		binWriteU8(&buf, am)
		binWriteString(&buf, a.Host)
		binWriteString(&buf, a.Path)
		binWriteString(&buf, a.Description)
		binWriteI64(&buf, a.SizeBytes)
	}
	// invalidated_by
	binWriteU64Slice(&buf, n.InvalidatedBy)
	// invalidation_reason
	binWriteString(&buf, n.InvalidationReason)
	// body (32-bit length)
	binWriteString32(&buf, n.Body)
	// optional semantic extensions
	binWriteString32(&buf, n.Scope)
	binWriteString32(&buf, n.ExitCriteria)
	binWriteU64Slice(&buf, n.ContinuedBy)
	binWriteU64Slice(&buf, n.SupersededBy)
	// structured runs
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
	binWriteU16(&buf, uint16(len(n.Runs)))
	for _, r := range n.Runs {
		if r.Valid == nil {
			binWriteU8(&buf, 0)
		} else {
			binWriteU8(&buf, 1)
			if *r.Valid {
				binWriteU8(&buf, 1)
			} else {
				binWriteU8(&buf, 0)
			}
		}
		binWriteString32(&buf, r.InvalidReason)
	}
	binWriteU16(&buf, uint16(len(n.Runs)))
	for _, r := range n.Runs {
		binWriteString32(&buf, r.ResourceID)
		binWriteString32(&buf, r.Endpoint)
		binWriteString32(&buf, string(r.EndpointKind))
	}
	binWriteU8(&buf, milestoneClassToBin(n.MilestoneClass))
	binWriteU8(&buf, milestoneKindToBin(n.MilestoneKind))
	binWriteString32(&buf, n.MilestoneReason)
	binWriteU8(&buf, relationExtensionMarker)
	// relations
	binWriteU16(&buf, uint16(len(n.Relations)))
	for _, rel := range n.Relations {
		rt, ok := relationTypeToBin[rel.Type]
		if !ok {
			return nil, fmt.Errorf("%w: unknown relation type %q", ErrInvalidNode, rel.Type)
		}
		binWriteU8(&buf, rt)
		binWriteU64(&buf, uint64(rel.Target))
		binWriteString32(&buf, rel.Note)
	}
	// primary_parent (optional u64)
	if n.PrimaryParent != nil {
		binWriteU8(&buf, 1)
		binWriteU64(&buf, uint64(*n.PrimaryParent))
	} else {
		binWriteU8(&buf, 0)
	}
	es, ok := evidenceStatusToBin[n.EvidenceStatus]
	if !ok {
		return nil, fmt.Errorf("%w: unknown evidence status %q", ErrInvalidNode, n.EvidenceStatus)
	}
	ec, ok := evidenceCauseToBin[n.EvidenceCause]
	if !ok {
		return nil, fmt.Errorf("%w: unknown evidence cause %q", ErrInvalidNode, n.EvidenceCause)
	}
	binWriteU8(&buf, es)
	binWriteU8(&buf, ec)
	binWriteString32(&buf, n.EvidenceScope)
	binWriteU64Slice(&buf, n.PoisonedBy)
	binWriteU64Slice(&buf, n.RevalidatedBy)
	binWriteString32(&buf, n.PoisonReason)

	return buf.Bytes(), nil
}

// UnmarshalNodeBinary decodes a node from the binary wire format v1.
func UnmarshalNodeBinary(b []byte) (*Node, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrInvalidNode)
	}
	pos := 0
	n := &Node{}

	// schema_version
	sv, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	n.SchemaVersion = SchemaVersion(sv)
	// id
	id, err := binReadU64(b, &pos)
	if err != nil {
		return nil, err
	}
	n.ID = NodeID(id)
	// title
	title, err := binReadString(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Title = title
	// status
	sb, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	status, ok := binToStatus[sb]
	if !ok {
		return nil, fmt.Errorf("%w: unknown status byte %d", ErrInvalidStatus, sb)
	}
	n.Status = status
	// claim_status
	cb, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	claim, ok := binToClaim[cb]
	if !ok {
		return nil, fmt.Errorf("%w: unknown claim status byte %d", ErrInvalidClaimStatus, cb)
	}
	n.ClaimStatus = claim
	// outcome
	ob, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	outcome, ok := binToOutcome[ob]
	if !ok {
		return nil, fmt.Errorf("%w: unknown outcome byte %d", ErrInvalidNode, ob)
	}
	n.Outcome = outcome
	// parents
	parents, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Parents = parents
	// agent
	agent, err := binReadString(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Agent = agent
	// tags
	numTags, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	if numTags > 0 {
		tags := make([]string, 0, numTags)
		for i := uint16(0); i < numTags; i++ {
			tag, rerr := binReadString(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			tags = append(tags, tag)
		}
		n.Tags = tags
	}
	// created
	created, err := binReadI64(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Created = binToTime(created)
	// modified
	modified, err := binReadI64(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Modified = binToTime(modified)
	// revision
	revision, err := binReadU64(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Revision = revision
	// commits
	numCommits, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	if numCommits > 0 {
		commits := make([]GitCommit, 0, numCommits)
		for i := uint16(0); i < numCommits; i++ {
			hash, rerr := binReadString(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			msg, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			commits = append(commits, GitCommit{Hash: hash, Message: msg})
		}
		n.Commits = commits
	}
	// artifacts
	numArtifacts, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	if numArtifacts > 0 {
		artifacts := make([]Artifact, 0, numArtifacts)
		for i := uint16(0); i < numArtifacts; i++ {
			mb, rerr := binReadU8(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			mode, ok := binToArtifactMode[mb]
			if !ok {
				return nil, fmt.Errorf("%w: unknown artifact mode byte %d", ErrInvalidArtifact, mb)
			}
			host, rerr := binReadString(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			path, rerr := binReadString(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			desc, rerr := binReadString(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			size, rerr := binReadI64(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			artifacts = append(artifacts, Artifact{Mode: mode, Host: host, Path: path, Description: desc, SizeBytes: size})
		}
		n.Artifacts = artifacts
	}
	// invalidated_by
	invBy, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.InvalidatedBy = invBy
	// invalidation_reason
	reason, err := binReadString(b, &pos)
	if err != nil {
		return nil, err
	}
	n.InvalidationReason = reason
	// body
	body, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Body = body
	if pos == len(b) {
		return n, nil
	}
	// optional semantic extensions added after the legacy payload
	scope, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Scope = scope
	exitCriteria, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.ExitCriteria = exitCriteria
	continuedBy, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.ContinuedBy = continuedBy
	supersededBy, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.SupersededBy = supersededBy
	if pos == len(b) {
		return n, nil
	}
	runCount, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	if runCount > 0 {
		runs := make([]RunRecord, 0, runCount)
		for i := uint16(0); i < runCount; i++ {
			ts, rerr := binReadI64(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			host, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			command, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			outDir, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			seed, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			eta, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			cost, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			note, rerr := binReadString32(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			runs = append(runs, RunRecord{
				Timestamp: binToTime(ts),
				Host:      host,
				Command:   command,
				OutDir:    outDir,
				Seed:      seed,
				ETA:       eta,
				Cost:      cost,
				Note:      note,
			})
		}
		n.Runs = runs
	}
	if pos == len(b) {
		return n, nil
	}
	validityCount, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < validityCount && int(i) < len(n.Runs); i++ {
		present, rerr := binReadU8(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		if present == 1 {
			value, rerr := binReadU8(b, &pos)
			if rerr != nil {
				return nil, rerr
			}
			v := value == 1
			n.Runs[i].Valid = &v
		}
		invalidReason, rerr := binReadString32(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		n.Runs[i].InvalidReason = invalidReason
	}
	if pos == len(b) {
		return n, nil
	}
	runEndpointCount, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < runEndpointCount && int(i) < len(n.Runs); i++ {
		resourceID, rerr := binReadString32(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		endpoint, rerr := binReadString32(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		endpointKind, rerr := binReadString32(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		n.Runs[i].ResourceID = resourceID
		n.Runs[i].Endpoint = endpoint
		n.Runs[i].EndpointKind = EndpointKind(endpointKind)
	}
	if pos == len(b) {
		return n, nil
	}
	milestoneClassByte, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	milestoneClass, ok := binToMilestoneClass(milestoneClassByte)
	if !ok {
		return nil, fmt.Errorf("%w: unknown milestone class byte %d", ErrInvalidNode, milestoneClassByte)
	}
	n.MilestoneClass = milestoneClass
	milestoneKindByte, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	milestoneKind, ok := binToMilestoneKind(milestoneKindByte)
	if !ok {
		return nil, fmt.Errorf("%w: unknown milestone kind byte %d", ErrInvalidNode, milestoneKindByte)
	}
	n.MilestoneKind = milestoneKind
	milestoneReason, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.MilestoneReason = milestoneReason
	if pos == len(b) {
		return n, nil
	}
	marker, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	if marker != relationExtensionMarker {
		return nil, fmt.Errorf("%w: unknown extension marker byte %d", ErrInvalidNode, marker)
	}
	// relations
	relCount, err := binReadU16(b, &pos)
	if err != nil {
		return nil, err
	}
	n.Relations = make([]Relation, 0, relCount)
	for i := uint16(0); i < relCount; i++ {
		rtByte, rerr := binReadU8(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		rt, ok := binToRelationType[rtByte]
		if !ok {
			return nil, fmt.Errorf("%w: unknown relation type byte %d", ErrInvalidNode, rtByte)
		}
		target, rerr := binReadU64(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		note, rerr := binReadString32(b, &pos)
		if rerr != nil {
			return nil, rerr
		}
		n.Relations = append(n.Relations, Relation{Type: rt, Target: NodeID(target), Note: note})
	}
	if pos == len(b) {
		return n, nil
	}
	// primary_parent
	ppPresent, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	if ppPresent != 0 {
		ppVal, err := binReadU64(b, &pos)
		if err != nil {
			return nil, err
		}
		pp := NodeID(ppVal)
		n.PrimaryParent = &pp
	}
	if pos == len(b) {
		return n, nil
	}
	esByte, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	es, ok := binToEvidenceStatus[esByte]
	if !ok {
		return nil, fmt.Errorf("%w: unknown evidence status byte %d", ErrInvalidNode, esByte)
	}
	n.EvidenceStatus = es
	ecByte, err := binReadU8(b, &pos)
	if err != nil {
		return nil, err
	}
	ec, ok := binToEvidenceCause[ecByte]
	if !ok {
		return nil, fmt.Errorf("%w: unknown evidence cause byte %d", ErrInvalidNode, ecByte)
	}
	n.EvidenceCause = ec
	evidenceScope, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.EvidenceScope = evidenceScope
	poisonedBy, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.PoisonedBy = poisonedBy
	revalidatedBy, err := binReadU64Slice(b, &pos)
	if err != nil {
		return nil, err
	}
	n.RevalidatedBy = revalidatedBy
	poisonReason, err := binReadString32(b, &pos)
	if err != nil {
		return nil, err
	}
	n.PoisonReason = poisonReason

	// Strict: reject trailing bytes
	if pos != len(b) {
		return nil, fmt.Errorf("%w: %d trailing bytes after node payload", ErrInvalidNode, len(b)-pos)
	}

	return n, nil
}

// WriteBinHeader writes the nodes.bin file header.
func WriteBinHeader(buf *bytes.Buffer) {
	buf.WriteString(binMagic)
	buf.WriteByte(binVersionCurrent)
	buf.Write([]byte{0, 0, 0})
}

// ReadBinHeader reads and validates the nodes.bin file header.
func ReadBinHeader(data []byte) (uint8, error) {
	if len(data) < binHeaderSize {
		return 0, fmt.Errorf("%w: truncated binary header (%d bytes)", ErrInvalidNode, len(data))
	}
	if string(data[:4]) != binMagic {
		return 0, fmt.Errorf("%w: invalid binary magic", ErrUnsupportedSchema)
	}
	ver := data[4]
	if ver != binVersionV1 && ver != binVersionCurrent {
		return 0, fmt.Errorf("%w: binary format version %d (want %d or %d)", ErrUnsupportedSchema, ver, binVersionV1, binVersionCurrent)
	}
	return ver, nil
}

// --- write helpers ---

// binWriteU8 writes a uint8 to the buffer.
func binWriteU8(buf *bytes.Buffer, v uint8) {
	buf.WriteByte(v)
}

// binWriteU16 writes a uint16 in little-endian to the buffer.
func binWriteU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

// binWriteU32 writes a uint32 in little-endian to the buffer.
func binWriteU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// binWriteU64 writes a uint64 in little-endian to the buffer.
func binWriteU64(buf *bytes.Buffer, v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	buf.Write(b[:])
}

// binWriteI64 writes an int64 in little-endian to the buffer.
func binWriteI64(buf *bytes.Buffer, v int64) {
	binWriteU64(buf, uint64(v))
}

// binWriteString writes a uint16-prefixed UTF-8 string to the buffer.
func binWriteString(buf *bytes.Buffer, s string) {
	l := len(s)
	if l > binMaxStrU16 {
		l = binMaxStrU16
	}
	binWriteU16(buf, uint16(l))
	if l > 0 {
		buf.WriteString(s[:l])
	}
}

// binWriteString32 writes a uint32-prefixed UTF-8 string to the buffer.
func binWriteString32(buf *bytes.Buffer, s string) {
	l := len(s)
	if l > binMaxBodyU32 {
		l = binMaxBodyU32
	}
	binWriteU32(buf, uint32(l))
	if l > 0 {
		buf.WriteString(s[:l])
	}
}

// binWriteU64Slice writes a uint16-prefixed slice of uint64 IDs to the buffer.
func binWriteU64Slice(buf *bytes.Buffer, ids []NodeID) {
	binWriteU16(buf, uint16(len(ids)))
	for _, id := range ids {
		binWriteU64(buf, uint64(id))
	}
}

// --- read helpers ---

// binReadU8 reads a uint8 from b at position pos.
func binReadU8(b []byte, pos *int) (uint8, error) {
	if *pos >= len(b) {
		return 0, fmt.Errorf("%w: unexpected end of payload", ErrInvalidNode)
	}
	v := b[*pos]
	*pos++
	return v, nil
}

// binReadU16 reads a uint16 in little-endian from b at position pos.
func binReadU16(b []byte, pos *int) (uint16, error) {
	if *pos+2 > len(b) {
		return 0, fmt.Errorf("%w: unexpected end of payload", ErrInvalidNode)
	}
	v := binary.LittleEndian.Uint16(b[*pos:])
	*pos += 2
	return v, nil
}

// binReadU32 reads a uint32 in little-endian from b at position pos.
func binReadU32(b []byte, pos *int) (uint32, error) {
	if *pos+4 > len(b) {
		return 0, fmt.Errorf("%w: unexpected end of payload", ErrInvalidNode)
	}
	v := binary.LittleEndian.Uint32(b[*pos:])
	*pos += 4
	return v, nil
}

// binReadU64 reads a uint64 in little-endian from b at position pos.
func binReadU64(b []byte, pos *int) (uint64, error) {
	if *pos+8 > len(b) {
		return 0, fmt.Errorf("%w: unexpected end of payload", ErrInvalidNode)
	}
	v := binary.LittleEndian.Uint64(b[*pos:])
	*pos += 8
	return v, nil
}

// binReadI64 reads an int64 in little-endian from b at position pos.
func binReadI64(b []byte, pos *int) (int64, error) {
	u, err := binReadU64(b, pos)
	return int64(u), err
}

// binReadString reads a uint16-prefixed UTF-8 string from b at position pos.
func binReadString(b []byte, pos *int) (string, error) {
	l, err := binReadU16(b, pos)
	if err != nil {
		return "", err
	}
	if l == 0 {
		return "", nil
	}
	if *pos+int(l) > len(b) {
		return "", fmt.Errorf("%w: string length %d exceeds payload", ErrInvalidNode, l)
	}
	s := string(b[*pos : *pos+int(l)])
	*pos += int(l)
	return s, nil
}

// binReadString32 reads a uint32-prefixed UTF-8 string from b at position pos.
func binReadString32(b []byte, pos *int) (string, error) {
	l, err := binReadU32(b, pos)
	if err != nil {
		return "", err
	}
	if l == 0 {
		return "", nil
	}
	if *pos+int(l) > len(b) {
		return "", fmt.Errorf("%w: string32 length %d exceeds payload", ErrInvalidNode, l)
	}
	s := string(b[*pos : *pos+int(l)])
	*pos += int(l)
	return s, nil
}

// binReadU64Slice reads a uint16-prefixed slice of uint64 IDs from b at position pos.
func binReadU64Slice(b []byte, pos *int) ([]NodeID, error) {
	count, err := binReadU16(b, pos)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	out := make([]NodeID, 0, count)
	for i := uint16(0); i < count; i++ {
		id, rerr := binReadU64(b, pos)
		if rerr != nil {
			return nil, rerr
		}
		out = append(out, NodeID(id))
	}
	return out, nil
}

// timeToBin converts a time.Time to int64 unix nanoseconds. Zero time = 0.
func timeToBin(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixNano()
}

// binToTime converts int64 unix nanoseconds to time.Time. 0 = zero time.
func binToTime(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v).UTC()
}
