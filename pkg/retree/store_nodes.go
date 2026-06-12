package retree

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
		b, err := os.ReadFile(filepath.Join(s.nodesDir(), e.Name()))
		if err != nil {
			return nil, err
		}
		n, err := UnmarshalNodeJSON(b)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
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
	defer f.Close()

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
	out := make([]*Node, 0, len(ids))
	for _, id := range ids {
		entry := idx[id]
		buf := make([]byte, entry.Length)
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
		out = append(out, n)
	}
	return out, nil
}

// getNode returns a deep copy of a single node by ID.
func (s *Store) getNode(id NodeID) (*Node, error) {
	all, err := s.loadAllNodes()
	if err != nil {
		return nil, err
	}
	for _, n := range all {
		if n.ID == id {
			return CloneNode(n), nil
		}
	}
	return nil, ErrNotFound
}

// persistGraph writes the in-memory graph to disk in the configured format.
func (s *Store) persistGraph(g *Graph) error {
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
		if err := s.writeAllNodesJSON(nodes); err != nil {
			return err
		}
	} else {
		if err := s.writeAllNodesBIN(nodes); err != nil {
			return err
		}
	}
	return s.regenerateEdgesFromGraph(g)
}

// writeAllNodesJSON writes all nodes as individual JSON files.
func (s *Store) writeAllNodesJSON(nodes []*Node) error {
	if err := os.MkdirAll(s.nodesDir(), 0o755); err != nil {
		return err
	}
	existing, err := os.ReadDir(s.nodesDir())
	if err != nil {
		return err
	}
	for _, e := range existing {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if err := os.Remove(filepath.Join(s.nodesDir(), e.Name())); err != nil {
				return err
			}
		}
	}
	for _, n := range nodes {
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

// RegenerateEdges reconstructs the edges.jsonl index from stored nodes.
func (s *Store) RegenerateEdges() error {
	g, err := s.loadGraph()
	if err != nil {
		return err
	}
	return s.regenerateEdgesFromGraph(g)
}

// appendJSONLine appends a JSON-encoded value as a single line to the file.
func appendJSONLine(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
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
	defer f.Close()
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
func (s *Store) getNodeHistory(id NodeID) ([]*Node, error) {
	dir := filepath.Join(s.nodeHistoryDir(), fmt.Sprintf("%04d", id))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ext := s.ext()
	var out []*Node
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var n *Node
		if s.format == StorageBIN {
			n, err = UnmarshalNodeBinary(b)
		} else {
			n, err = UnmarshalNodeJSON(b)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.Before(out[j].Modified) })
	return out, nil
}
