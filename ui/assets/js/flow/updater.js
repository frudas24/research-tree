// Graph translation and incremental update helpers for research-tree

// Convert research-tree graph JSON to Cytoscape element array
export function toElements(g) {
  const els = [];
  const seen = new Set();

  (g.nodes || []).forEach(n => {
    const id = String(n.id);
    if (!id || seen.has(id)) return;
    seen.add(id);
    els.push({
      data: {
        id,
        label: formatLabel(n),
        title: n.title || '',
        status: n.status || 'active',
        claim_status: n.claim_status || 'provisional',
        evidence_status: n.evidence_status || 'clean',
        outcome: n.outcome || 'unset',
        milestone_class: n.milestone_class || '',
        milestone_kind: n.milestone_kind || '',
        agent: n.agent || '',
        children: (n.children || []).length,
        pending_children: n.pending_children || 0,
        tags: (n.tags || []).join(', '),
        hotness: n.hotness || 0,
        scope: n.scope || '',
      },
    });
  });

  // Parent edges (DAG structure)
  (g.edges || []).forEach(e => {
    const sid = String(e.from);
    const tid = String(e.to);
    // Ensure endpoints exist
    if (sid && !seen.has(sid)) {
      seen.add(sid);
      els.push({ data: { id: sid, label: sid } });
    }
    if (tid && !seen.has(tid)) {
      seen.add(tid);
      els.push({ data: { id: tid, label: tid } });
    }
    const eid = `parent:${sid}->${tid}`;
    els.push({
      data: { id: eid, source: sid, target: tid, type: 'parent', weight: 1 },
    });
  });

  // Relation edges (typed cross-edges)
  (g.relations || []).forEach(r => {
    const sid = String(r.from);
    const tid = String(r.target);
    if (sid && !seen.has(sid)) {
      seen.add(sid);
      els.push({ data: { id: sid, label: sid } });
    }
    if (tid && !seen.has(tid)) {
      seen.add(tid);
      els.push({ data: { id: tid, label: tid } });
    }
    const relType = r.type || 'depends_on';
    const rid = `rel:${sid}->${tid}:${relType}`;
    els.push({
      data: { id: rid, source: sid, target: tid, type: relType },
    });
  });

  return els;
}

// Incremental diff: update existing, add new, never remove (keepHistory)
export function applyDiff(cy, els, keepHistory = true) {
  let changed = false;

  const nodeData = new Map();
  const edgeData = new Map();

  els.forEach(el => {
    const d = el.data;
    if (!d) return;
    if (d.source && d.target) {
      edgeData.set(d.id, d);
    } else if (d.id) {
      nodeData.set(d.id, d);
    }
  });

  // Only remove edges if not keeping history
  if (!keepHistory) {
    cy.edges().forEach(e => { if (!edgeData.has(e.id())) { e.remove(); changed = true; } });
    cy.nodes().forEach(n => { if (!nodeData.has(n.id())) { n.remove(); changed = true; } });
  }

  // Add/update nodes
  nodeData.forEach((d, id) => {
    const n = cy.getElementById(id);
    if (n && !n.empty()) {
      // Update data fields that drive styling
      n.data('label', d.label);
      n.data('status', d.status);
      n.data('claim_status', d.claim_status);
      n.data('evidence_status', d.evidence_status);
      n.data('outcome', d.outcome);
      n.data('milestone_class', d.milestone_class);
      n.data('milestone_kind', d.milestone_kind);
      n.data('children', d.children);
      n.data('pending_children', d.pending_children);
      n.data('hotness', d.hotness);
      n.data('tags', d.tags);
    } else {
      cy.add({ data: d });
      changed = true;
    }
  });

  // Add/update edges
  edgeData.forEach((d, id) => {
    const e = cy.getElementById(id);
    if (e && !e.empty()) {
      e.data('type', d.type);
      e.data('weight', d.weight);
    } else {
      // Ensure endpoints exist
      if (d.source) {
        const src = cy.getElementById(d.source);
        if (!src || src.empty()) cy.add({ data: { id: d.source, label: d.source } });
      }
      if (d.target) {
        const tgt = cy.getElementById(d.target);
        if (!tgt || tgt.empty()) cy.add({ data: { id: d.target, label: d.target } });
      }
      cy.add({ data: d });
      changed = true;
    }
  });

  return changed;
}

function formatLabel(n) {
  const id = String(n.id).padStart(4, '0');
  const prefix = getStatusIcon(n);
  const title = truncate(n.title || n.id, 40);
  return `${prefix} ${id} ${title}`;
}

function getStatusIcon(n) {
  if (n.milestone_class === 'golden') return '\u2605'; // ★
  switch (n.status) {
    case 'done':    return '\u2714'; // ✔
    case 'paused':  return '\u23F8'; // ⏸
    case 'active':  return '\u25B6'; // ▶
    default:        return '\u25CF'; // ●
  }
}

function truncate(s, max) {
  if (!s) return '';
  return s.length > max ? s.slice(0, max - 1) + '\u2026' : s;
}
