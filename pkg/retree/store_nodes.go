package retree

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxBinNodePayloadSize = 64 << 20

type binIndexEntry struct {
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
	Checksum uint32 `json:"checksum"`
}

// loadGraph loads all nodes from disk into an in-memory graph.
func (s *Store) loadGraph() (*Graph, error) {
	nodes, err := s.loadAllNodes()
	if err != nil {
		return nil, err
	}
	g := NewGraph()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, n := range nodes {
		if err := g.addNode(n, false); err != nil {
			return nil, err
		}
	}
	if err := validateGraphReferentialIntegrity(g); err != nil {
		return nil, err
	}
	return g, nil
}

// loadGraphAllowLegacyDoneUnset loads the full graph while tolerating the
// historical done+unset pattern so repair and restore flows can validate
// legacy stores without weakening any other invariant.
func (s *Store) loadGraphAllowLegacyDoneUnset() (*Graph, error) {
	nodes, err := s.loadAllNodesAllowLegacyDoneUnset()
	if err != nil {
		return nil, err
	}
	g := NewGraph()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	for _, n := range nodes {
		if err := g.addNodeAssumeValid(n, false); err != nil {
			return nil, err
		}
	}
	if err := validateGraphReferentialIntegrity(g); err != nil {
		return nil, err
	}
	return g, nil
}

// loadAllNodes loads all nodes from disk, dispatching by storage format.
func (s *Store) loadAllNodes() ([]*Node, error) {
	if s.format == StorageJSON {
		return s.loadAllNodesJSON()
	}
	return s.loadAllNodesBIN()
}

// loadAllNodesAllowLegacyDoneUnset loads nodes while tolerating the historical
// done+unset pattern so repair tooling can inspect and upgrade old stores.
func (s *Store) loadAllNodesAllowLegacyDoneUnset() ([]*Node, error) {
	if s.format == StorageJSON {
		return s.loadAllNodesJSONWithLegacyOutcomeTolerance()
	}
	return s.loadAllNodesBINWithLegacyOutcomeTolerance()
}

// loadAllNodesJSON loads nodes from individual JSON files.
func (s *Store) loadAllNodesJSON() ([]*Node, error) {
	entries, err := os.ReadDir(s.nodesDir())
	if err != nil {
		return nil, err
	}
	out := make([]*Node, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n, err := loadAndValidateJSONNode(filepath.Join(s.nodesDir(), e.Name()), true)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// loadAllNodesJSONWithLegacyOutcomeTolerance loads JSON nodes while allowing
// historical done+unset payloads so migration tooling can inspect old stores.
func (s *Store) loadAllNodesJSONWithLegacyOutcomeTolerance() ([]*Node, error) {
	entries, err := os.ReadDir(s.nodesDir())
	if err != nil {
		return nil, err
	}
	out := make([]*Node, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n, err := loadAndValidateJSONNodeWithLegacyOutcomeTolerance(filepath.Join(s.nodesDir(), e.Name()), true)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// ensureBinIndexPresent fails loudly when nodes.bin contains data but
// nodes.idx is missing. Treating a missing index as an empty graph silently
// destroys the binary data on the next write; regeneration must be explicit.
func (s *Store) ensureBinIndexPresent() error {
	if _, err := os.Stat(s.nodesIdxPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	binSize, err := binaryFileSize(s.nodesBinPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no binary store yet; nothing to index
		}
		return err
	}
	if binSize > binHeaderSize {
		return fmt.Errorf("bin index %s is missing but %s holds data; run RegenerateBinIndex (rt storage reindex) before writing", s.nodesIdxPath(), s.nodesBinPath())
	}
	return nil
}

// regenerateBinIndex rebuilds nodes.idx by scanning nodes.bin sequentially.
// It is the recovery path for a lost or corrupted binary index.
func (s *Store) regenerateBinIndex() error {
	if s.format != StorageBIN {
		return fmt.Errorf("%w: binary index only exists in bin mode", ErrInvalidNode)
	}
	return s.withLock("reindex_bin", func() error {
		idx, err := s.scanBinIndex()
		if err != nil {
			return err
		}
		return s.writeBinIndex(idx)
	})
}

// scanBinIndex sequentially decodes every node payload in nodes.bin,
// recomputing offsets, lengths, and CRC32 checksums for the index.
func (s *Store) scanBinIndex() (map[NodeID]binIndexEntry, error) {
	data, err := readBinaryFile(s.nodesBinPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[NodeID]binIndexEntry{}, nil
		}
		return nil, err
	}
	if len(data) < binHeaderSize {
		return nil, fmt.Errorf("%w: truncated binary file (%d bytes)", ErrInvalidNode, len(data))
	}
	ver, err := ReadBinHeader(data[:binHeaderSize])
	if err != nil {
		return nil, err
	}
	if ver == binVersionV1 && len(data) > binHeaderSize {
		return nil, fmt.Errorf("%w: cannot safely reindex legacy v1 nodes.bin without a trusted nodes.idx", ErrUnsupportedSchema)
	}
	pos := binHeaderSize
	idx := make(map[NodeID]binIndexEntry)
	for pos < len(data) {
		start := pos
		n, consumed, err := decodeNodeBinary(data[start:])
		if err != nil {
			return nil, fmt.Errorf("regenerate bin index: node at offset %d: %w", start, err)
		}
		if _, exists := idx[n.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate node id %d in nodes.bin during reindex", ErrDuplicateID, n.ID)
		}
		idx[n.ID] = binIndexEntry{
			Offset:   int64(start),
			Length:   int64(consumed),
			Checksum: crc32.ChecksumIEEE(data[start : start+consumed]),
		}
		pos += consumed
	}
	return idx, nil
}

// readBinIndex reads the binary index from nodes.idx.
func (s *Store) readBinIndex() (map[NodeID]binIndexEntry, error) {
	b, err := readBinaryFile(s.nodesIdxPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[NodeID]binIndexEntry{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return map[NodeID]binIndexEntry{}, nil
	}
	var raw map[string]binIndexEntry
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[NodeID]binIndexEntry, len(raw))
	for k, v := range raw {
		n, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return nil, err
		}
		out[NodeID(n)] = v
	}
	return out, nil
}

// marshalBinIndex serializes the direct-access index deterministically enough
// for atomic pair staging. JSON object key order is handled by encoding/json.
func marshalBinIndex(idx map[NodeID]binIndexEntry) ([]byte, error) {
	raw := make(map[string]binIndexEntry, len(idx))
	for id, v := range idx {
		raw[fmt.Sprintf("%d", id)] = v
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// writeBinIndex atomically writes the binary index to nodes.idx.
func (s *Store) writeBinIndex(idx map[NodeID]binIndexEntry) error {
	b, err := marshalBinIndex(idx)
	if err != nil {
		return err
	}
	tmp := s.nodesIdxPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return replaceBinaryFile(tmp, s.nodesIdxPath())
}

// recoverBinaryPublicationLocked repairs a crash interrupted between the
// nodes.bin and nodes.idx renames. The caller must hold the store lock.
func (s *Store) recoverBinaryPublicationLocked() error {
	if s.format != StorageBIN {
		return nil
	}
	if _, err := os.Stat(s.binaryDirtyPath()); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	tokenBytes, err := os.ReadFile(s.binaryDirtyPath())
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		token, err = newLockToken()
		if err != nil {
			return err
		}
	}
	idx, err := s.scanBinIndex()
	if err != nil {
		return err
	}
	if err := s.writeBinIndex(idx); err != nil {
		return err
	}
	if err := s.writeBinaryGeneration(token); err != nil {
		return err
	}
	return os.Remove(s.binaryDirtyPath())
}

// readBinaryGeneration returns the last completely published generation.
// Missing generation files identify legacy stores and are represented by "".
func (s *Store) readBinaryGeneration() (string, error) {
	b, err := readBinaryFile(s.binaryGenerationPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// writeBinaryGeneration atomically publishes the completed generation token.
func (s *Store) writeBinaryGeneration(token string) error {
	tmp := s.binaryGenerationPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(token+"\n"), 0o644); err != nil {
		return err
	}
	return replaceBinaryFile(tmp, s.binaryGenerationPath())
}

// beginStableBinaryRead waits for an idle publication window and captures its
// generation. The marker is checked again after reading the generation to
// close the writer-started-between-checks race.
func (s *Store) beginStableBinaryRead(deadline time.Time) (string, error) {
	for {
		if _, err := os.Stat(s.binaryDirtyPath()); err == nil {
			if time.Now().After(deadline) {
				return "", fmt.Errorf("binary node store has an incomplete publication marker; reopen the store to recover")
			}
			time.Sleep(20 * time.Millisecond)
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		generation, err := s.readBinaryGeneration()
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(s.binaryDirtyPath()); os.IsNotExist(err) {
			return generation, nil
		} else if err != nil {
			return "", err
		}
	}
}

// binaryReadStillStable validates the seqlock after an IDX/BIN read.
func (s *Store) binaryReadStillStable(generation string) (bool, error) {
	if _, err := os.Stat(s.binaryDirtyPath()); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	current, err := s.readBinaryGeneration()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(s.binaryDirtyPath()); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return current == generation, nil
}

// withStableBinaryRead retries a lock-free read if a complete writer
// publication overlaps it. A stable decoding error is returned as corruption;
// an error from a mixed generation is discarded and retried.
func withStableBinaryRead[T any](s *Store, read func() (T, error)) (T, error) {
	var zero T
	deadline := time.Now().Add(lockTimeout)
	for {
		generation, err := s.beginStableBinaryRead(deadline)
		if err != nil {
			return zero, err
		}
		value, readErr := read()
		stable, err := s.binaryReadStillStable(generation)
		if err != nil {
			return zero, err
		}
		if stable {
			return value, readErr
		}
		if time.Now().After(deadline) {
			return zero, fmt.Errorf("binary node store did not reach a stable generation")
		}
	}
}

// loadAllNodesBIN loads nodes from the binary storage format with header validation.
func (s *Store) loadAllNodesBIN() ([]*Node, error) {
	return withStableBinaryRead(s, s.loadAllNodesBINOnce)
}

// loadAllNodesBINOnce reads one candidate generation; the caller verifies its
// generation before returning it.
func (s *Store) loadAllNodesBINOnce() ([]*Node, error) {
	if err := s.ensureBinIndexPresent(); err != nil {
		return nil, err
	}
	idx, err := s.readBinIndex()
	if err != nil {
		return nil, err
	}
	f, err := openBinaryRead(s.nodesBinPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, binHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("bin: read header: %w", err)
	}
	if _, err := ReadBinHeader(header); err != nil {
		return nil, err
	}
	ids := make([]NodeID, 0, len(idx))
	for id := range idx {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	out := make([]*Node, 0, len(ids))
	for _, id := range ids {
		entry := idx[id]
		n, err := s.readBinNodeAt(f, fi.Size(), id, entry)
		if err != nil {
			return nil, err
		}
		if n.ID != id {
			return nil, fmt.Errorf("%w: index entry %d decodes to node %d", ErrInvalidNode, id, n.ID)
		}
		out = append(out, n)
	}
	return out, nil
}

// loadAllNodesBINWithLegacyOutcomeTolerance loads BIN nodes while allowing
// historical done+unset payloads so migration tooling can inspect old stores.
func (s *Store) loadAllNodesBINWithLegacyOutcomeTolerance() ([]*Node, error) {
	return withStableBinaryRead(s, s.loadAllNodesBINWithLegacyOutcomeToleranceOnce)
}

// loadAllNodesBINWithLegacyOutcomeToleranceOnce reads one candidate legacy
// generation; the caller verifies its generation before returning it.
func (s *Store) loadAllNodesBINWithLegacyOutcomeToleranceOnce() ([]*Node, error) {
	if err := s.ensureBinIndexPresent(); err != nil {
		return nil, err
	}
	idx, err := s.readBinIndex()
	if err != nil {
		return nil, err
	}
	f, err := openBinaryRead(s.nodesBinPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, binHeaderSize)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, fmt.Errorf("bin: read header: %w", err)
	}
	if _, err := ReadBinHeader(header); err != nil {
		return nil, err
	}
	ids := make([]NodeID, 0, len(idx))
	for id := range idx {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	out := make([]*Node, 0, len(ids))
	for _, id := range ids {
		entry := idx[id]
		n, err := s.readBinNodeAtAllowLegacyDoneUnset(f, fi.Size(), id, entry)
		if err != nil {
			return nil, err
		}
		if n.ID != id {
			return nil, fmt.Errorf("%w: index entry %d decodes to node %d", ErrInvalidNode, id, n.ID)
		}
		out = append(out, n)
	}
	return out, nil
}

// getNode returns a single node by ID without scanning the full store.
// JSON mode reads the node's own file; binary mode uses the index + CRC.
func (s *Store) getNode(id NodeID) (*Node, error) {
	if s.format == StorageJSON {
		return s.getNodeJSON(id)
	}
	return s.getNodeBIN(id)
}

// getNodeJSON reads one node file directly.
func (s *Store) getNodeJSON(id NodeID) (*Node, error) {
	b, err := os.ReadFile(filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", id)))
	if err != nil {
		if os.IsNotExist(err) {
			// A missing file with a healthy nodes dir means "no such node";
			// a missing nodes dir is a broken store, not a not-found node.
			if _, dirErr := os.Stat(s.nodesDir()); os.IsNotExist(dirErr) {
				return nil, fmt.Errorf("nodes directory missing: %w", err)
			}
			return nil, ErrNotFound
		}
		return nil, err
	}
	n, err := UnmarshalNodeJSON(b)
	if err != nil {
		return nil, err
	}
	if err := normalizeAndValidateLoadedNode(n); err != nil {
		return nil, err
	}
	if n.ID != id {
		return nil, fmt.Errorf("%w: file %04d.json contains node %d", ErrInvalidNode, id, n.ID)
	}
	return n, nil
}

// getNodeBIN reads one node payload through the index, verifying its CRC.
func (s *Store) getNodeBIN(id NodeID) (*Node, error) {
	return withStableBinaryRead(s, func() (*Node, error) {
		return s.getNodeBINOnce(id)
	})
}

// getNodeBINOnce reads one candidate index/data generation.
func (s *Store) getNodeBINOnce(id NodeID) (*Node, error) {
	if err := s.ensureBinIndexPresent(); err != nil {
		return nil, err
	}
	idx, err := s.readBinIndex()
	if err != nil {
		return nil, err
	}
	entry, ok := idx[id]
	if !ok {
		return nil, ErrNotFound
	}
	f, err := openBinaryRead(s.nodesBinPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	n, err := s.readBinNodeAt(f, fi.Size(), id, entry)
	if err != nil {
		return nil, err
	}
	if n.ID != id {
		return nil, fmt.Errorf("%w: index entry %d decodes to node %d", ErrInvalidNode, id, n.ID)
	}
	return n, nil
}

// readBinNodeAt validates one index entry, reads its payload, verifies its CRC,
// and decodes the node.
func (s *Store) readBinNodeAt(f *os.File, fileSize int64, id NodeID, entry binIndexEntry) (*Node, error) {
	if err := validateBinIndexEntry(fileSize, entry); err != nil {
		return nil, fmt.Errorf("node %d: %w", id, err)
	}
	buf := make([]byte, int(entry.Length))
	if _, err := f.ReadAt(buf, entry.Offset); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(buf) != entry.Checksum {
		return nil, fmt.Errorf("checksum mismatch for node %d", id)
	}
	n, err := UnmarshalNodeBinary(buf)
	if err != nil {
		return nil, err
	}
	if err := normalizeAndValidateLoadedNode(n); err != nil {
		return nil, err
	}
	return n, nil
}

// readBinNodeAtAllowLegacyDoneUnset reads and validates one binary node entry
// while tolerating the historical done+unset combination for repair workflows.
func (s *Store) readBinNodeAtAllowLegacyDoneUnset(f *os.File, fileSize int64, id NodeID, entry binIndexEntry) (*Node, error) {
	if err := validateBinIndexEntry(fileSize, entry); err != nil {
		return nil, fmt.Errorf("node %d: %w", id, err)
	}
	buf := make([]byte, int(entry.Length))
	if _, err := f.ReadAt(buf, entry.Offset); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(buf) != entry.Checksum {
		return nil, fmt.Errorf("checksum mismatch for node %d", id)
	}
	n, err := UnmarshalNodeBinary(buf)
	if err != nil {
		return nil, err
	}
	if err := normalizeAndValidateLoadedNodeAllowLegacyDoneUnset(n); err != nil {
		return nil, err
	}
	return n, nil
}

// validateBinIndexEntry rejects malformed or unsafe nodes.idx entries before
// they can panic or reserve absurd amounts of memory.
func validateBinIndexEntry(fileSize int64, entry binIndexEntry) error {
	if entry.Offset < binHeaderSize {
		return fmt.Errorf("%w: invalid node offset %d", ErrInvalidNode, entry.Offset)
	}
	if entry.Length <= 0 {
		return fmt.Errorf("%w: invalid node length %d", ErrInvalidNode, entry.Length)
	}
	if entry.Length > maxBinNodePayloadSize {
		return fmt.Errorf("%w: node length %d exceeds defensive payload limit %d", ErrInvalidNode, entry.Length, maxBinNodePayloadSize)
	}
	if entry.Length > int64(math.MaxInt) {
		return fmt.Errorf("%w: node length %d exceeds process limit", ErrInvalidNode, entry.Length)
	}
	if entry.Offset > fileSize {
		return fmt.Errorf("%w: node offset %d exceeds file size %d", ErrInvalidNode, entry.Offset, fileSize)
	}
	if entry.Length > fileSize-entry.Offset {
		return fmt.Errorf("%w: node range offset=%d length=%d exceeds file size %d", ErrInvalidNode, entry.Offset, entry.Length, fileSize)
	}
	return nil
}

// loadAndValidateJSONNode reads one JSON node file, applies defaults, validates
// the payload, and optionally enforces that the filename matches the node ID.
func loadAndValidateJSONNode(path string, checkFilenameID bool) (*Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := UnmarshalNodeJSON(b)
	if err != nil {
		return nil, err
	}
	if err := normalizeAndValidateLoadedNode(n); err != nil {
		return nil, err
	}
	if checkFilenameID {
		id, err := parseJSONNodeID(filepath.Base(path))
		if err != nil {
			return nil, err
		}
		if n.ID != id {
			return nil, fmt.Errorf("%w: file %s contains node %d", ErrInvalidNode, filepath.Base(path), n.ID)
		}
	}
	return n, nil
}

// loadAndValidateJSONNodeWithLegacyOutcomeTolerance loads one JSON node file
// while tolerating the historical done+unset combination for repair workflows.
func loadAndValidateJSONNodeWithLegacyOutcomeTolerance(path string, checkFilenameID bool) (*Node, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := UnmarshalNodeJSON(b)
	if err != nil {
		return nil, err
	}
	if err := normalizeAndValidateLoadedNodeAllowLegacyDoneUnset(n); err != nil {
		return nil, err
	}
	if checkFilenameID {
		id, err := parseJSONNodeID(filepath.Base(path))
		if err != nil {
			return nil, err
		}
		if n.ID != id {
			return nil, fmt.Errorf("%w: file %s contains node %d", ErrInvalidNode, filepath.Base(path), n.ID)
		}
	}
	return n, nil
}

// normalizeAndValidateLoadedNode applies deterministic defaults to a decoded
// node before validating it as if it had been created through the public API.
func normalizeAndValidateLoadedNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	ApplyNodeDefaults(n, n.Created)
	return ValidateNode(n)
}

// normalizeAndValidateLoadedNodeAllowLegacyDoneUnset applies defaults and
// validates a decoded node while tolerating the historical done+unset pattern.
func normalizeAndValidateLoadedNodeAllowLegacyDoneUnset(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	ApplyNodeDefaults(n, n.Created)
	return validateNode(n, true)
}

// validateGraphReferentialIntegrity rejects in-memory graphs that still contain
// parent edges pointing at nodes absent from the loaded node set.
func validateGraphReferentialIntegrity(g *Graph) error {
	for childID, parents := range g.Parents {
		for _, parentID := range parents {
			if _, ok := g.Nodes[parentID]; !ok {
				return fmt.Errorf("%w: node %d references missing parent %d", ErrInvalidNode, childID, parentID)
			}
		}
	}
	return nil
}

// markDerivedDirty durably announces that edges.jsonl/relations.jsonl must not
// be trusted until they are regenerated from authoritative node payloads.
func (s *Store) markDerivedDirty() error {
	tmp := s.derivedDirtyPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.derivedDirtyPath())
}

// clearDerivedDirty marks both derived indexes as synchronized.
func (s *Store) clearDerivedDirty() { _ = os.Remove(s.derivedDirtyPath()) }

// derivedIndexesNeedRepair reports whether a previous publication was
// interrupted or either rebuildable sidecar is absent.
func (s *Store) derivedIndexesNeedRepair() (bool, error) {
	if _, err := os.Stat(s.derivedDirtyPath()); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	for _, path := range []string{s.edgesPath(), s.relationsPath()} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return true, nil
		} else if err != nil {
			return false, err
		}
	}
	return false, nil
}

// repairDerivedIndexesLocked rebuilds both projections from one graph while
// the caller holds the store lock.
func (s *Store) repairDerivedIndexesLocked() error {
	g, err := s.loadGraphAllowLegacyDoneUnset()
	if err != nil {
		return err
	}
	if err := s.regenerateEdgesFromGraph(g); err != nil {
		return err
	}
	if err := s.regenerateRelations(g); err != nil {
		return err
	}
	s.clearDerivedDirty()
	return nil
}

// ensureDerivedIndexesReady repairs an interrupted derived publication. It is
// used only from read paths that are not already inside a store mutation.
func (s *Store) ensureDerivedIndexesReady() error {
	need, err := s.derivedIndexesNeedRepair()
	if err != nil || !need {
		return err
	}
	return s.withLock("repair_derived_indexes", func() error {
		need, err := s.derivedIndexesNeedRepair()
		if err != nil || !need {
			return err
		}
		return s.repairDerivedIndexesLocked()
	})
}

// authoritativeCommitSucceeded reports whether a persistence error occurred
// only after authoritative node state was already committed. Public node CRUD
// treats this as success and leaves .derived.dirty for deterministic repair.
func authoritativeCommitSucceeded(err error) bool {
	return err == nil || errors.Is(err, ErrDerivedState)
}

// persistGraph writes the in-memory graph to disk in the configured format.
func (s *Store) persistGraph(g *Graph) error {
	return s.persistGraphDelta(g, nil, nil)
}

// persistGraphDelta persists the graph. In JSON mode only dirty nodes are
// written (and files for removed/orphaned nodes deleted), so a crash never
// leaves the store in a state where every node file was already deleted.
// dirty == nil means every node is written (full rewrite). removed lists
// node IDs that no longer exist. Binary mode always rewrites nodes.bin
// atomically as a single file.
func (s *Store) persistGraphDelta(g *Graph, dirty map[NodeID]struct{}, removed []NodeID) error {
	ids := make([]NodeID, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	nodes := make([]*Node, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, g.Nodes[id])
	}
	// Announce the derived-index transition before touching authoritative node
	// state. A crash anywhere below leaves a deterministic repair signal.
	if err := s.markDerivedDirty(); err != nil {
		return err
	}
	if s.format == StorageJSON {
		if err := s.writeAllNodesJSONDelta(nodes, dirty, removed); err != nil {
			return err
		}
	} else {
		if err := s.writeAllNodesBIN(nodes); err != nil {
			return err
		}
	}
	// From this point the authoritative node mutation is committed. Derived
	// projections are rebuildable; a late sidecar error must not masquerade as
	// a failed node mutation. Leave .derived.dirty for deterministic repair.
	if err := s.regenerateEdgesFromGraph(g); err != nil {
		return fmt.Errorf("%w: regenerate edges: %v", ErrDerivedState, err)
	}
	if err := s.regenerateRelations(g); err != nil {
		return fmt.Errorf("%w: regenerate relations: %v", ErrDerivedState, err)
	}
	s.clearDerivedDirty()
	return nil
}

// writeAllNodesJSONDelta writes only dirty node files in JSON mode. Files for
// node IDs in removed, or absent from the graph, are deleted. Unrelated node
// files are left untouched, which removes the delete-all-then-rewrite crash
// window and keeps single-node edits O(1) in file operations.
func (s *Store) writeAllNodesJSONDelta(nodes []*Node, dirty map[NodeID]struct{}, removed []NodeID) error {
	if err := os.MkdirAll(s.nodesDir(), 0o755); err != nil {
		return err
	}
	inGraph := make(map[NodeID]struct{}, len(nodes))
	for _, n := range nodes {
		inGraph[n.ID] = struct{}{}
	}
	toDelete := make(map[NodeID]struct{}, len(removed))
	for _, id := range removed {
		toDelete[id] = struct{}{}
	}
	existing, err := os.ReadDir(s.nodesDir())
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id, perr := parseJSONNodeID(e.Name())
		if perr != nil {
			continue
		}
		if _, ok := inGraph[id]; !ok {
			toDelete[id] = struct{}{}
		}
	}

	// Publish every rewritten survivor before deleting removed nodes. If the
	// process dies between these phases, the old parent may remain but no child
	// can reference a parent that has already vanished.
	writeAll := dirty == nil
	for _, n := range nodes {
		if !writeAll {
			if _, ok := dirty[n.ID]; !ok {
				continue
			}
		}
		b, err := MarshalNodeJSON(n)
		if err != nil {
			return err
		}
		name := filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", n.ID))
		tmp := name + ".tmp"
		if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, name); err != nil {
			return err
		}
	}
	for id := range toDelete {
		name := filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", id))
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// parseJSONNodeID extracts a NodeID from a node file name like "0042.json".
func parseJSONNodeID(name string) (NodeID, error) {
	base := strings.TrimSuffix(name, ".json")
	if base == name || base == "" {
		return 0, fmt.Errorf("%w: not a node file %q", ErrInvalidNode, name)
	}
	id, err := strconv.ParseUint(base, 10, 64)
	if err != nil {
		return 0, err
	}
	return NodeID(id), nil
}

// writeAllNodesBIN stages nodes.bin and nodes.idx completely before publishing
// either one. A durable marker brackets the two renames so lock-free readers
// wait instead of mixing generations; Open can recover a crash by rebuilding
// nodes.idx from whichever complete nodes.bin was published.
func (s *Store) writeAllNodesBIN(nodes []*Node) error {
	return s.writeAllNodesBINWithGenerationWriter(nodes, s.writeBinaryGeneration)
}

// writeAllNodesBINWithGenerationWriter publishes a binary node generation and
// permits deterministic fault injection at the final generation write.
func (s *Store) writeAllNodesBINWithGenerationWriter(nodes []*Node, writeGeneration func(string) error) error {
	return s.writeAllNodesBINWithPublishers(nodes, replaceBinaryFile, writeGeneration)
}

// writeAllNodesBINWithPublishers publishes a binary generation while
// permitting deterministic fault injection at both publication boundaries.
func (s *Store) writeAllNodesBINWithPublishers(nodes []*Node, renameBinary func(string, string) error, writeGeneration func(string) error) error {
	var buf bytes.Buffer
	WriteBinHeader(&buf)
	idx := make(map[NodeID]binIndexEntry, len(nodes))
	for _, n := range nodes {
		b, err := MarshalNodeBinary(n)
		if err != nil {
			return err
		}
		off := int64(buf.Len())
		if _, err := buf.Write(b); err != nil {
			return err
		}
		idx[n.ID] = binIndexEntry{Offset: off, Length: int64(len(b)), Checksum: crc32.ChecksumIEEE(b)}
	}
	idxBytes, err := marshalBinIndex(idx)
	if err != nil {
		return err
	}
	tmpBin := s.nodesBinPath() + ".tmp"
	tmpIdx := s.nodesIdxPath() + ".pair.tmp"
	if err := os.WriteFile(tmpBin, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(tmpIdx, idxBytes, 0o644); err != nil {
		_ = os.Remove(tmpBin)
		return err
	}
	token, err := newLockToken()
	if err != nil {
		_ = os.Remove(tmpBin)
		_ = os.Remove(tmpIdx)
		return err
	}
	markerTmp := s.binaryDirtyPath() + ".tmp"
	if err := os.WriteFile(markerTmp, []byte(token+"\n"), 0o644); err != nil {
		_ = os.Remove(tmpBin)
		_ = os.Remove(tmpIdx)
		return err
	}
	if err := os.Rename(markerTmp, s.binaryDirtyPath()); err != nil {
		_ = os.Remove(tmpBin)
		_ = os.Remove(tmpIdx)
		return err
	}
	if err := renameBinary(tmpBin, s.nodesBinPath()); err != nil {
		cleanupErr := s.abortBinaryPublication(tmpBin, tmpIdx)
		if cleanupErr != nil {
			return fmt.Errorf("publish binary data: %w; abort publication: %v", err, cleanupErr)
		}
		return fmt.Errorf("publish binary data: %w", err)
	}
	if err := replaceBinaryFile(tmpIdx, s.nodesIdxPath()); err != nil {
		// nodes.bin is authoritative once renamed. Attempt immediate recovery so
		// callers do not receive a false pre-commit failure or self-block later.
		if recoverErr := s.recoverBinaryPublicationLocked(); recoverErr != nil {
			return fmt.Errorf("%w: publish binary index: %v; immediate recovery: %v", ErrDerivedState, err, recoverErr)
		}
		_ = os.Remove(tmpIdx)
		return nil
	}
	if err := writeGeneration(token); err != nil {
		if recoverErr := s.recoverBinaryPublicationLocked(); recoverErr != nil {
			return fmt.Errorf("%w: publish binary generation: %v; immediate recovery: %v", ErrDerivedState, err, recoverErr)
		}
		return nil
	}
	// The pair is now complete. Failure to remove the marker does not make the
	// pair invalid; it only forces the next Open through deterministic reindex.
	_ = os.Remove(s.binaryDirtyPath())
	return nil
}

// abortBinaryPublication removes staging and dirty state after a failure that
// occurred before nodes.bin became authoritative.
func (s *Store) abortBinaryPublication(tmpBin, tmpIdx string) error {
	var errs []error
	for _, path := range []string{tmpBin, tmpIdx, s.binaryDirtyPath()} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", filepath.Base(path), err))
		}
	}
	return errors.Join(errs...)
}

// regenerateEdgesFromGraph reconstructs edges.jsonl from the graph.
func (s *Store) regenerateEdgesFromGraph(g *Graph) error {
	var b strings.Builder
	ids := make([]NodeID, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, child := range ids {
		for _, parent := range g.GetParents(child) {
			line := fmt.Sprintf("{\"from\":%d,\"to\":%d}\n", parent, child)
			b.WriteString(line)
		}
	}
	tmp := s.edgesPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.edgesPath())
}

// regenerateRelations reconstructs relations.jsonl from the graph's node data.
func (s *Store) regenerateRelations(g *Graph) error {
	var b strings.Builder
	ids := make([]NodeID, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		n := g.Nodes[id]
		for _, rel := range n.Relations {
			lineBytes, err := json.Marshal(relationsLine{
				From: id,
				To:   rel.Target,
				Type: rel.Type,
				Note: rel.Note,
			})
			if err != nil {
				return err
			}
			b.Write(lineBytes)
			b.WriteByte('\n')
		}
	}
	tmp := s.relationsPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.relationsPath())
}

// RegenerateEdges reconstructs the edges.jsonl index from stored nodes under
// the store lock so a stale pre-update graph cannot overwrite a newer index.
func (s *Store) RegenerateEdges() error {
	return s.withLock("regenerate_edges", func() error {
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		return s.regenerateEdgesFromGraph(g)
	})
}

// edgeLine is the on-disk format for one parent edge in edges.jsonl.
type edgeLine struct {
	From NodeID `json:"from"`
	To   NodeID `json:"to"`
}

// readEdgesLines reads and schema-validates every parent edge.
func (s *Store) readEdgesLines() ([]edgeLine, error) {
	f, err := os.Open(s.edgesPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []edgeLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var edge edgeLine
		if err := decodeJSONStrict(line, &edge); err != nil {
			return nil, err
		}
		if edge.From == 0 || edge.To == 0 {
			return nil, fmt.Errorf("%w: edge endpoints must be non-zero", ErrInvalidNode)
		}
		out = append(out, edge)
	}
	return out, sc.Err()
}

// relationsLine is the on-disk format for one relation edge in relations.jsonl.
type relationsLine struct {
	From NodeID       `json:"from"`
	To   NodeID       `json:"to"`
	Type RelationType `json:"type"`
	Note string       `json:"note,omitempty"`
}

// listRelations returns all relations for a specific node from relations.jsonl.
func (s *Store) listRelations(id NodeID) ([]Relation, error) {
	all, err := s.readRelationsLines()
	if err != nil {
		return nil, err
	}
	var out []Relation
	for _, rl := range all {
		if rl.From == id {
			out = append(out, Relation{Type: rl.Type, Target: rl.To, Note: rl.Note})
		}
	}
	return out, nil
}

// listAllRelations returns all relation edges with full context.
func (s *Store) listAllRelations() ([]struct {
	From     NodeID
	Relation Relation
}, error) {
	all, err := s.readRelationsLines()
	if err != nil {
		return nil, err
	}
	out := make([]struct {
		From     NodeID
		Relation Relation
	}, 0, len(all))
	for _, rl := range all {
		out = append(out, struct {
			From     NodeID
			Relation Relation
		}{From: rl.From, Relation: Relation{Type: rl.Type, Target: rl.To, Note: rl.Note}})
	}
	return out, nil
}

// readRelationsLines reads all lines from relations.jsonl.
func (s *Store) readRelationsLines() ([]relationsLine, error) {
	f, err := os.Open(s.relationsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []relationsLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rl relationsLine
		if err := decodeJSONStrict([]byte(line), &rl); err != nil {
			return nil, err
		}
		out = append(out, rl)
	}
	return out, sc.Err()
}

// regenerateRelationsFromNodes rebuilds relations.jsonl from all stored nodes
// while holding the store lock to prevent stale repair output from winning a
// race with a concurrent node update.
func (s *Store) regenerateRelationsFromNodes() error {
	return s.withLock("regenerate_relations", func() error {
		g, err := s.loadGraph()
		if err != nil {
			return err
		}
		return s.regenerateRelations(g)
	})
}

// appendJSONLine appends a JSON-encoded value as a single line to the file.
func appendJSONLine(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	return enc.Encode(v)
}

// readJSONLines reads JSONL entries from a file into a slice of T.
func readJSONLines[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := make([]T, 0)
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) == 0 {
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		var v T
		if jerr := decodeJSONStrict(bytes.TrimSpace(line), &v); jerr != nil {
			return nil, jerr
		}
		out = append(out, v)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// nowUTC returns the current time in UTC.
func nowUTC() time.Time { return time.Now().UTC() }

// ext returns the file extension for the current storage format.
func (s *Store) ext() string {
	if s.format == StorageBIN {
		return ".bin"
	}
	return ".json"
}

// saveNodeHistory writes the previous version of a node to an immutable,
// collision-resistant history entry. Callers validate the candidate first so a
// rejected mutation cannot create history.
func (s *Store) saveNodeHistory(n *Node) error {
	dir := filepath.Join(s.nodeHistoryDir(), fmt.Sprintf("%04d", n.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b []byte
	var err error
	if s.format == StorageBIN {
		b, err = MarshalNodeBinary(n)
	} else {
		b, err = MarshalNodeJSON(n)
		if err == nil {
			b = append(b, '\n')
		}
	}
	if err != nil {
		return err
	}
	ts := time.Now().UTC().Format("20060102_150405.000000000")
	path := filepath.Join(dir, fmt.Sprintf("rev%04d_%s%s", n.Revision, ts, s.ext()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// GetNodeHistory returns all historical versions of a node, sorted oldest-first.
// Each history entry is decoded according to its own file extension so that
// entries written before a storage-format migration remain readable.
func (s *Store) getNodeHistory(id NodeID) ([]*Node, error) {
	dir := filepath.Join(s.nodeHistoryDir(), fmt.Sprintf("%04d", id))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Node
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		var n *Node
		switch {
		case strings.HasSuffix(e.Name(), ".json"):
			var b []byte
			b, err = os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			n, err = UnmarshalNodeJSON(b)
		case strings.HasSuffix(e.Name(), ".bin"):
			var b []byte
			b, err = os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			n, err = UnmarshalNodeBinary(b)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.Before(out[j].Modified) })
	return out, nil
}
