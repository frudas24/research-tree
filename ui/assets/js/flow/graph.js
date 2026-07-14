// Boot and orchestrate Cytoscape graph for research-tree DAG
/* global cytoscape */

import { style } from './styles.js';
import { toElements, applyDiff } from './updater.js';

async function fetchGraph() {
  const r = await fetch('/graph', { cache: 'no-store' });
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

export async function bootCy(container) {
  const cy = cytoscape({ container, elements: [], style: style(), pixelRatio: 1 });

  // ELK layered layout (top-down DAG)
  const lay = () => cy.layout({
    name: 'elk',
    fit: false,
    animate: false,
    nodeDimensionsIncludeLabels: true,
    elk: {
      algorithm: 'layered',
      'elk.layered.spacing.nodeNodeBetweenLayers': 60,
      'elk.direction': 'DOWN',
    },
  });

  // ── In-memory state (synapp2 pattern) ──
  const state = {
    nodes: new Map(),   // id → server node payload
    edges: new Map(),   // "parent:id->id" → edge
    relations: new Map(), // "rel:id->id:type" → relation edge
    lastRefresh: 0,
    paused: false,
    filterText: '',
    filterStatus: '',
    filterClaim: '',
  };

  function upsertNode(id, n) {
    state.nodes.set(String(id), n);
  }
  function upsertEdge(from, to, type, weight) {
    const key = `${type}:${from}->${to}`;
    state.edges.set(key, { from, to, type, weight: weight || 1 });
  }

  // Build snapshot from state
  function snapshotFromState() {
    const nodes = Array.from(state.nodes.values());
    const edges = Array.from(state.edges.values());
    const relations = Array.from(state.relations.values());
    return { nodes, edges, relations };
  }

  // Apply filters
  function filterNodes(nodes) {
    const f = state.filterText.toLowerCase().trim();
    const fs = state.filterStatus;
    const fc = state.filterClaim;
    return nodes.filter(n => {
      if (fs && n.status !== fs) return false;
      if (fc && n.claim_status !== fc) return false;
      if (f) {
        const title = (n.title || '').toLowerCase();
        const sid = String(n.id);
        const tags = (n.tags || []).join(' ').toLowerCase();
        const scope = (n.scope || '').toLowerCase();
        return title.includes(f) || sid.includes(f) || tags.includes(f) || scope.includes(f);
      }
      return true;
    });
  }

  let firstLayoutDone = false;
  let lastLayoutAt = 0;
  const LAYOUT_MIN_MS = 3000;

  async function refresh() {
    try {
      const raw = snapshotFromState();
      const filtered = filterNodes(raw.nodes);
      // Rebuild edges for filtered nodes only
      const filteredIds = new Set(filtered.map(n => String(n.id)));
      const filteredEdges = raw.edges.filter(e => filteredIds.has(String(e.from)) && filteredIds.has(String(e.to)));
      const filteredRels = raw.relations.filter(r => filteredIds.has(String(r.from)) && filteredIds.has(String(r.target)));

      const g = { nodes: filtered, edges: filteredEdges, relations: filteredRels };
      const els = toElements(g);

      const zoom = cy.zoom();
      const pan = cy.pan();

      cy.startBatch();
      const changed = applyDiff(cy, els, true);
      cy.endBatch();

      const now = Date.now();
      const hasNodes = cy.nodes().length > 0;

      if (hasNodes && !firstLayoutDone) {
        firstLayoutDone = true;
        lastLayoutAt = now;
        const layout = lay();
        layout.one('layoutstop', () => {
          try { cy.fit(cy.elements(), 50); } catch {}
        });
        layout.run();
      } else if (changed && (now - lastLayoutAt) >= LAYOUT_MIN_MS) {
        lastLayoutAt = now;
        const layout = lay();
        layout.one('layoutstop', () => {
          cy.zoom(zoom);
          cy.pan(pan);
        });
        layout.run();
      } else {
        cy.zoom(zoom);
        cy.pan(pan);
      }

      // Update stats
      const hint = document.getElementById('cyHint');
      if (hint) {
        hint.textContent = `nodes=${filtered.length} edges=${filteredEdges.length + filteredRels.length}`;
      }
    } catch (e) {
      console.warn('refresh failed', e);
    }
  }

  // ── Data ingestion: full reload from /graph ──
  async function ingestGraph(data) {
    state.nodes.clear();
    state.edges.clear();
    state.relations.clear();

    (data.nodes || []).forEach(n => upsertNode(n.id, n));
    (data.edges || []).forEach(e => upsertEdge(e.from, e.to, 'parent', 1));
    (data.relations || []).forEach(r => {
      const key = `rel:${r.from}->${r.target}:${r.type}`;
      state.relations.set(key, { from: r.from, target: r.target, type: r.type, note: r.note || '' });
    });

    await refresh();
    renderSidebar(data.nodes || []);
  }

  // ── Polling loop (primary transport for RT) ──
  let pollTimer = null;
  let pollInterval = 5000;

  async function poll() {
    try {
      const data = await fetchGraph();
      await ingestGraph(data);
    } catch (e) {
      console.warn('poll failed', e);
    }
    if (!state.paused) {
      pollTimer = setTimeout(poll, pollInterval);
    }
  }

  // ── Sidebar: top hotspots ──
  function renderSidebar(nodes) {
    const list = document.getElementById('hotspotList');
    if (!list) return;
    list.innerHTML = '';

    const sorted = nodes
      .filter(n => n.hotness > 0)
      .sort((a, b) => b.hotness - a.hotness)
      .slice(0, 15);

    sorted.forEach(n => {
      const li = document.createElement('li');
      li.className = 'node-item';
      li.onclick = () => {
        cy.animate({ center: { eles: `#${n.id}` }, duration: 300 });
      };

      const name = document.createElement('span');
      name.className = 'name';
      name.textContent = `[${String(n.id).padStart(4, '0')}] ${n.title || ''}`;

      const m = document.createElement('span');
      m.className = 'metric';
      m.textContent = `h=${n.hotness || 0}`;

      li.appendChild(name);
      li.appendChild(m);
      list.appendChild(li);
    });
  }

  // ── Controls ──
  function wireControls() {
    const pauseBtn = document.getElementById('btnPause');
    const txtFilter = document.getElementById('txtFilter');
    const selStatus = document.getElementById('selStatus');
    const selClaim = document.getElementById('selClaim');
    const pollSel = document.getElementById('selPollInterval');

    if (pauseBtn) {
      pauseBtn.onclick = () => {
        state.paused = !state.paused;
        pauseBtn.textContent = state.paused ? 'Resume' : 'Pause';
        if (!state.paused) {
          pollTimer = setTimeout(poll, 500);
        } else {
          clearTimeout(pollTimer);
        }
      };
    }

    if (txtFilter) {
      txtFilter.addEventListener('input', () => {
        state.filterText = txtFilter.value;
        refresh();
      });
    }

    if (selStatus) {
      selStatus.addEventListener('change', () => {
        state.filterStatus = selStatus.value;
        refresh();
      });
    }

    if (selClaim) {
      selClaim.addEventListener('change', () => {
        state.filterClaim = selClaim.value;
        refresh();
      });
    }

    if (pollSel) {
      pollSel.addEventListener('change', () => {
        pollInterval = parseInt(pollSel.value, 10) || 5000;
      });
    }

    // Node click → show detail
    cy.on('tap', 'node', (evt) => {
      const node = evt.target;
      showNodeDetail(node);
    });

    cy.on('tap', (evt) => {
      if (evt.target === cy) {
        hideNodeDetail();
      }
    });

    // Right-click context menu
    cy.on('cxttap', 'node', (evt) => {
      const node = evt.target;
      showContextMenu(evt.originalEvent.clientX, evt.originalEvent.clientY, node);
    });
  }

  function showNodeDetail(node) {
    const d = node.data();
    const panel = document.getElementById('nodeDetail');
    if (!panel) return;
    panel.innerHTML = `
      <strong>${d.label}</strong>
      <div style="font-size:11px; margin-top:4px; line-height:1.5">
        <div>Status: <span class="pill ${d.status === 'done' ? 'pill-ok' : d.status === 'active' ? 'pill-warn' : ''}">${d.status}</span></div>
        <div>Claim: ${d.claim_status}</div>
        <div>Evidence: ${d.evidence_status}</div>
        <div>Outcome: ${d.outcome}</div>
        <div>Children: ${d.children} (${d.pending_children} pending)</div>
        <div>Hotness: ${d.hotness}</div>
        ${d.milestone_class ? `<div>Milestone: ${d.milestone_class}/${d.milestone_kind}</div>` : ''}
        ${d.agent ? `<div>Agent: ${d.agent}</div>` : ''}
        ${d.tags ? `<div>Tags: ${d.tags}</div>` : ''}
      </div>
    `;
    panel.style.display = 'block';
  }

  function hideNodeDetail() {
    const panel = document.getElementById('nodeDetail');
    if (panel) panel.style.display = 'none';
  }

  function showContextMenu(x, y, node) {
    // Remove existing
    const old = document.querySelector('.cy-menu');
    if (old) old.remove();

    const menu = document.createElement('div');
    menu.className = 'cy-menu';
    menu.style.left = x + 'px';
    menu.style.top = y + 'px';

    const id = node.id();
    const items = [
      { label: `Focus on ${id}`, action: () => cy.fit(node, 80) },
      { label: 'Show neighbors (k=1)', action: () => {
        const nhood = node.closedNeighborhood();
        cy.elements().addClass('dimmed');
        nhood.removeClass('dimmed');
      }},
      { label: 'Reset view', action: () => {
        cy.elements().removeClass('dimmed');
        cy.fit(cy.elements(), 50);
      }},
    ];

    items.forEach(it => {
      const btn = document.createElement('button');
      btn.textContent = it.label;
      btn.onclick = () => { it.action(); menu.remove(); };
      menu.appendChild(btn);
    });

    document.body.appendChild(menu);
    const close = (e) => {
      if (!menu.contains(e.target)) { menu.remove(); document.removeEventListener('click', close); }
    };
    setTimeout(() => document.addEventListener('click', close), 0);
  }

  // ── Legend ──
  function renderLegend(container) {
    if (!container) return;
    container.innerHTML = `
      <div class="legend-item"><span class="legend-swatch" style="background:#eab308"></span> Active</div>
      <div class="legend-item"><span class="legend-swatch" style="background:#3fb950"></span> Done</div>
      <div class="legend-item"><span class="legend-swatch" style="background:#6b7280"></span> Paused</div>
      <div class="legend-item"><span class="legend-swatch" style="border:3px solid #3fb950; background:transparent"></span> Validated</div>
      <div class="legend-item"><span class="legend-swatch" style="border:3px dashed #f85149; background:transparent"></span> Invalidated</div>
      <div class="legend-item"><span class="legend-swatch" style="background:#fbbf24; clip-path:polygon(50% 0,100% 50%,50% 100%,0 50%)"></span> Golden</div>
      <div class="legend-item"><span class="legend-swatch" style="background:#f85149; opacity:0.5"></span> Poisoned</div>
    `;
  }

  // ── Boot ──
  wireControls();
  renderLegend(document.getElementById('legend'));

  // Initial load
  try {
    const seed = await fetchGraph();
    await ingestGraph(seed);
  } catch (e) {
    console.warn('initial graph load failed', e);
  }

  // Start polling
  pollTimer = setTimeout(poll, pollInterval);

  return cy;
}
