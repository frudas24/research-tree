package retree

import (
	"bufio"
	"bytes"
	"encoding/json"
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

// loadAllNodes loads all nodes from disk, dispatching by storage format.
func (s *Store) loadAllNodes() ([]*Node, error) {
	if s.format == StorageJSON {
		return s.loadAllNodesJSON()
	}
	return s.loadAllNodesBIN()
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

// ensureBinIndexPresent fails loudly when nodes.bin contains data but
// nodes.idx is missing. Treating a missing index as an empty graph silently
// destroys the binary data on the next write; regeneration must be explicit.
func (s *Store) ensureBinIndexPresent() error {
	if _, err := os.Stat(s.nodesIdxPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	bin, err := os.ReadFile(s.nodesBinPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no binary store yet; nothing to index
		}
		return err
	}
	if len(bin) > binHeaderSize {
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
	data, err := os.ReadFile(s.nodesBinPath())
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
	b, err := os.ReadFile(s.nodesIdxPath())
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

// writeBinIndex atomically writes the binary index to nodes.idx.
func (s *Store) writeBinIndex(idx map[NodeID]binIndexEntry) error {
	raw := make(map[string]binIndexEntry, len(idx))
	for id, v := range idx {
		raw[fmt.Sprintf("%d", id)] = v
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.nodesIdxPath() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.nodesIdxPath())
}

// loadAllNodesBIN loads nodes from the binary storage format with header validation.
func (s *Store) loadAllNodesBIN() ([]*Node, error) {
	if err := s.ensureBinIndexPresent(); err != nil {
		return nil, err
	}
	idx, err := s.readBinIndex()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(s.nodesBinPath())
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
	f, err := os.Open(s.nodesBinPath())
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
	return UnmarshalNodeBinary(buf)
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

// normalizeAndValidateLoadedNode applies deterministic defaults to a decoded
// node before validating it as if it had been created through the public API.
func normalizeAndValidateLoadedNode(n *Node) error {
	if n == nil {
		return fmt.Errorf("%w: nil", ErrInvalidNode)
	}
	ApplyNodeDefaults(n, n.Created)
	return ValidateNode(n)
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
	if s.format == StorageJSON {
		if err := s.writeAllNodesJSONDelta(nodes, dirty, removed); err != nil {
			return err
		}
	} else {
		if err := s.writeAllNodesBIN(nodes); err != nil {
			return err
		}
	}
	if err := s.regenerateEdgesFromGraph(g); err != nil {
		return err
	}
	return s.regenerateRelations(g)
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
	for id := range toDelete {
		name := filepath.Join(s.nodesDir(), fmt.Sprintf("%04d.json", id))
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
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

// writeAllNodesBIN writes all nodes using the binary codec with header.
func (s *Store) writeAllNodesBIN(nodes []*Node) error {
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
	tmpBin := s.nodesBinPath() + ".tmp"
	if err := os.WriteFile(tmpBin, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpBin, s.nodesBinPath()); err != nil {
		return err
	}
	return s.writeBinIndex(idx)
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

// RegenerateEdges reconstructs the edges.jsonl index from stored nodes.
func (s *Store) RegenerateEdges() error {
	g, err := s.loadGraph()
	if err != nil {
		return err
	}
	return s.regenerateEdgesFromGraph(g)
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
		if err := json.Unmarshal([]byte(line), &rl); err != nil {
			return nil, err
		}
		out = append(out, rl)
	}
	return out, sc.Err()
}

// regenerateRelationsFromNodes rebuilds relations.jsonl from all stored nodes.
func (s *Store) regenerateRelationsFromNodes() error {
	g, err := s.loadGraph()
	if err != nil {
		return err
	}
	return s.regenerateRelations(g)
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
		if jerr := json.Unmarshal(bytes.TrimSpace(line), &v); jerr != nil {
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

// saveNodeHistory writes the previous version of a node to the per-node
// history directory before it gets overwritten by an update.
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
	}
	if err != nil {
		return err
	}
	ts := n.Modified.UTC().Format("20060102_150405")
	path := filepath.Join(dir, fmt.Sprintf("rev%04d_%s%s", n.Revision, ts, s.ext()))
	if s.format == StorageBIN {
		return os.WriteFile(path, b, 0o644)
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
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
