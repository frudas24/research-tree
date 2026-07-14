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

  // ── In-memory state ──
  const state = {
    nodes: new Map(),
    edges: new Map(),
    relations: new Map(),
    lastRefresh: 0,
    paused: false,
    filterText: '',
    filterStatus: '',
    filterClaim: '',
    detailVisible: false,
  };

  function upsertNode(id, n) { state.nodes.set(String(id), n); }
  function upsertEdge(from, to, type, weight) {
    state.edges.set(`${type}:${from}->${to}`, { from, to, type, weight: weight || 1 });
  }

  function snapshotFromState() {
    return {
      nodes: Array.from(state.nodes.values()),
      edges: Array.from(state.edges.values()),
      relations: Array.from(state.relations.values()),
    };
  }

  // Filter + search: returns { filtered, firstMatch } so we can scroll to it
  function filterNodes(nodes) {
    const f = state.filterText.toLowerCase().trim();
    const fs = state.filterStatus;
    const fc = state.filterClaim;

    let first = null;
    const result = [];

    for (const n of nodes) {
      if (fs && fs !== 'all' && n.status !== fs) continue;
      if (fc && fc !== 'all' && n.claim_status !== fc) continue;

      if (f) {
        const title = (n.title || '').toLowerCase();
        const sid = String(n.id);
        const padId = String(n.id).padStart(4, '0');
        const tags = (n.tags || []).join(' ').toLowerCase();
        const scope = (n.scope || '').toLowerCase();
        const agent = (n.agent || '').toLowerCase();
        const match = title.includes(f) || sid === f || padId === f ||
          tags.includes(f) || scope.includes(f) || agent.includes(f);
        if (!match) continue;
      }

      result.push(n);
      if (!first) first = n;
    }

    return { filtered: result, firstMatch: first };
  }

  let firstLayoutDone = false;
  let lastLayoutAt = 0;
  let forceLayout = false;
  const LAYOUT_MIN_MS = 3000;

  async function refresh() {
    try {
      const raw = snapshotFromState();
      const { filtered, firstMatch } = filterNodes(raw.nodes);
      const filteredIds = new Set(filtered.map(n => String(n.id)));
      const filteredEdges = raw.edges.filter(e =>
        filteredIds.has(String(e.from)) && filteredIds.has(String(e.to)));
      const filteredRels = raw.relations.filter(r =>
        filteredIds.has(String(r.from)) && filteredIds.has(String(r.target)));

      const g = { nodes: filtered, edges: filteredEdges, relations: filteredRels };
      const els = toElements(g);

      const zoom = cy.zoom();
      const pan = cy.pan();

      cy.startBatch();
      const changed = applyDiff(cy, els, false);
      cy.endBatch();

      const now = Date.now();
      const hasNodes = cy.nodes().length > 0;
      const doLayout = forceLayout || (changed && (now - lastLayoutAt) >= LAYOUT_MIN_MS);
      forceLayout = false;

      if (hasNodes && !firstLayoutDone) {
        firstLayoutDone = true;
        lastLayoutAt = now;
        const layout = lay();
        layout.one('layoutstop', () => { try { cy.fit(cy.elements(), 50); } catch {} });
        layout.run();
      } else if (doLayout) {
        lastLayoutAt = now;
        const layout = lay();
        layout.one('layoutstop', () => {
          try { cy.fit(cy.elements(), 50); } catch {}
        });
        layout.run();
      } else {
        cy.zoom(zoom);
        cy.pan(pan);
      }

      // Scroll to first matching node (text search: precise, filter: fit done above)
      if (firstMatch && state.filterText.trim() && !doLayout) {
        const n = cy.getElementById(String(firstMatch.id));
        if (n && !n.empty()) {
          cy.animate({ center: { eles: n }, duration: 400 });
        }
      }

      const hint = document.getElementById('cyHint');
      if (hint) {
        hint.textContent = `nodes=${filtered.length} edges=${filteredEdges.length + filteredRels.length}`;
      }
    } catch (e) {
      console.warn('refresh failed', e);
    }
  }

  // ── Data ingestion ──
  async function ingestGraph(data) {
    state.nodes.clear();
    state.edges.clear();
    state.relations.clear();

    (data.nodes || []).forEach(n => upsertNode(n.id, n));
    (data.edges || []).forEach(e => upsertEdge(e.from, e.to, 'parent', 1));
    (data.relations || []).forEach(r => {
      state.relations.set(`rel:${r.from}->${r.target}:${r.type}`, {
        from: r.from, target: r.target, type: r.type, note: r.note || '',
      });
    });

    await refresh();
    renderSidebar(data.nodes || []);
  }

  // ── Polling ──
  let pollTimer = null;
  let pollInterval = 10000;

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

  // ── Sidebar: hotspots ──
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
      li.onclick = () => navigateToNode(n.id);
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

  // ── Node detail panel (independent of hotspot sidebar) ──
  async function showNodeDetail(node) {
    const id = typeof node === 'object' ? node.id() : String(node);
    const panel = document.getElementById('nodeDetail');
    const content = document.getElementById('nodeDetailContent');
    if (!panel || !content) return;

    content.innerHTML = '<div style="color:var(--muted)">loading...</div>';
    panel.style.display = 'block';

    try {
      const r = await fetch(`/node?id=${encodeURIComponent(id)}`);
      if (!r.ok) throw new Error(r.statusText);
      const d = await r.json();
      content.innerHTML = buildDetailHTML(d);
    } catch (e) {
      content.innerHTML = `<div style="color:var(--bad)">Error: ${e.message}</div>`;
    }
  }

  // Global: navigate to a node (center graph + show detail)
  // Exposed on window so inline onclick in detail panel can call it.
  async function navigateToNode(id) {
    const n = cy.getElementById(String(id));
    if (n && !n.empty()) {
      cy.animate({ center: { eles: n }, duration: 300 });
    } else {
      // Node is filtered out — reset filters so it becomes visible
      const filterEl = document.getElementById('txtFilter');
      if (filterEl) filterEl.value = '';
      const selStatus = document.getElementById('selStatus');
      if (selStatus) selStatus.value = 'all';
      const selClaim = document.getElementById('selClaim');
      if (selClaim) selClaim.value = 'all';
      state.filterText = '';
      state.filterStatus = 'all';
      state.filterClaim = 'all';
      forceLayout = true;
      await refresh();
      // Retry centering after reload
      const n2 = cy.getElementById(String(id));
      if (n2 && !n2.empty()) {
        cy.animate({ center: { eles: n2 }, duration: 300 });
      }
    }
    await showNodeDetail(id);
  }
  window.__navToNode = navigateToNode;

  function hideNodeDetail() {
    const panel = document.getElementById('nodeDetail');
    if (panel) panel.style.display = 'none';
  }

  function buildDetailHTML(d) {
    const statusPill = d.status === 'done' ? 'pill-ok' : d.status === 'active' ? 'pill-warn' : '';
    const evidenceBadge = d.evidence_status === 'poisoned'
      ? '<span class="pill pill-bad">poisoned</span>'
      : d.evidence_status === 'suspect'
        ? '<span class="pill pill-warn">suspect</span>'
        : d.evidence_status === 'revalidated'
          ? '<span class="pill pill-ok">revalidated</span>' : '';
    const idPad = String(d.id).padStart(4, '0');
    const milestoneIcon = d.milestone_class === 'golden' ? ' \u2605' : '';

    let html = `<strong>[${idPad}] ${esc(d.title)}${milestoneIcon}</strong>`;
    html += '<div style="margin-top:6px">';

    html += `<div>Status: <span class="pill ${statusPill}">${d.status}</span>`;
    html += ` Claim: <b>${d.claim_status}</b>`;
    html += ` Outcome: <b>${d.outcome}</b>`;
    html += ` Evidence: <b>${d.evidence_status}</b>${evidenceBadge}</div>`;

    if (d.evidence_cause) html += `<div>Cause: ${d.evidence_cause}${d.evidence_scope ? ' — ' + d.evidence_scope : ''}</div>`;
    if (d.poison_reason) html += `<div style="color:var(--bad)">\u2623 Poison: ${esc(d.poison_reason)}</div>`;
    if (d.invalidation_reason) html += `<div style="color:var(--bad)">\u2717 Invalidated: ${esc(d.invalidation_reason)}</div>`;

    if (d.milestone_class) {
      html += `<div>\u2605 <b>Golden</b> ${d.milestone_kind || ''}`;
      if (d.milestone_reason) html += `<br><span style="color:var(--muted)">${esc(d.milestone_reason).substring(0, 200)}</span>`;
      html += '</div>';
    }

    html += `<div>Children: <b>${(d.children||[]).length}</b> (${d.pending_children} pending) | Hotness: <b>${d.hotness}</b></div>`;

    if (d.parents && d.parents.length) {
      const pp = d.primary_parent;
      html += `<div>Parents: ${d.parents.map(p => {
        const star = pp === p ? ' \u2605' : '';
        return `<span class="pill pill-ok" style="cursor:pointer" onclick="event.stopPropagation();window.__navToNode(${p})">${String(p).padStart(4,'0')}${star}</span>`;
      }).join(' ')}</div>`;
    }
    if (d.children && d.children.length) {
      html += `<div>Children: ${d.children.map(c =>
        `<span class="pill pill-warn" style="cursor:pointer" onclick="event.stopPropagation();window.__navToNode(${c})">${String(c).padStart(4,'0')}</span>`
      ).join(' ')}</div>`;
    }
    if (d.continued_by && d.continued_by.length) html += `<div>Continued by: ${d.continued_by.map(c => String(c).padStart(4,'0')).join(', ')}</div>`;
    if (d.superseded_by && d.superseded_by.length) html += `<div>Superseded by: ${d.superseded_by.map(c => String(c).padStart(4,'0')).join(', ')}</div>`;
    if (d.invalidated_by && d.invalidated_by.length) html += `<div style="color:var(--bad)">Invalidated by: ${d.invalidated_by.map(c => String(c).padStart(4,'0')).join(', ')}</div>`;
    if (d.poisoned_by && d.poisoned_by.length) html += `<div style="color:var(--bad)">Poisoned by: ${d.poisoned_by.map(c => String(c).padStart(4,'0')).join(', ')}</div>`;
    if (d.revalidated_by && d.revalidated_by.length) html += `<div style="color:var(--ok)">Revalidated by: ${d.revalidated_by.map(c => String(c).padStart(4,'0')).join(', ')}</div>`;

    if (d.relations && d.relations.length) {
      html += '<div>Relations out: ' + d.relations.map(r =>
        `<span class="pill" style="background:var(--muted)">[${r.type}]\u2192${String(r.target).padStart(4,'0')}</span>`
      ).join(' ') + '</div>';
    }
    if (d.relation_of && d.relation_of.length) {
      html += '<div>Relations in: ' + d.relation_of.map(r =>
        `<span class="pill" style="background:var(--muted)">${String(r.target).padStart(4,'0')}\u2192[${r.type}]</span>`
      ).join(' ') + '</div>';
    }

    if (d.agent) html += `<div>Agent: <b>${esc(d.agent)}</b></div>`;
    if (d.scope) html += `<div>Scope: ${esc(d.scope)}</div>`;
    if (d.exit_criteria) html += `<div>Exit: ${esc(d.exit_criteria)}</div>`;
    if (d.tags && d.tags.length) {
      html += '<div>Tags: ' + d.tags.map(t => `<span class="pill">${esc(t)}</span>`).join(' ') + '</div>';
    }

    html += `<div>Runs: <b>${d.runs_count}</b> | Artifacts: <b>${d.artifacts_count}</b> | Commits: <b>${d.commits_count}</b> | Rev: <b>${d.revision}</b></div>`;
    if (d.created) html += `<div style="color:var(--muted)">Created: ${fmtDate(d.created)}</div>`;
    if (d.modified) html += `<div style="color:var(--muted)">Modified: ${fmtDate(d.modified)}</div>`;

    if (d.body) {
      html += `<div style="margin-top:6px; padding:6px; background:#0a0f14; border-radius:6px; white-space:pre-wrap; font-size:11px; color:#9ca3af">${esc(d.body)}</div>`;
    }

    html += '</div>';
    return html;
  }

  function esc(s) {
    if (!s) return '';
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  function fmtDate(iso) {
    if (!iso) return '';
    try { return new Date(iso).toLocaleString(); } catch { return iso; }
  }

  // ── Context menu ──
  function showContextMenu(x, y, node) {
    const old = document.querySelector('.cy-menu');
    if (old) old.remove();

    const menu = document.createElement('div');
    menu.className = 'cy-menu';
    menu.style.left = x + 'px';
    menu.style.top = y + 'px';

    const items = [
      { label: `Focus on ${node.id()}`, action: () => cy.fit(node, 80) },
      {
        label: 'Show neighbors (k=1)', action: () => {
          const nhood = node.closedNeighborhood();
          cy.elements().addClass('dimmed');
          nhood.removeClass('dimmed');
        },
      },
      {
        label: 'Reset view', action: () => {
          cy.elements().removeClass('dimmed');
          cy.fit(cy.elements(), 50);
        },
      },
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

  // ── Controls wiring ──
  function wireControls() {
    const pauseBtn = document.getElementById('btnPause');
    const txtFilter = document.getElementById('txtFilter');
    const selStatus = document.getElementById('selStatus');
    const selClaim = document.getElementById('selClaim');
    const pollSel = document.getElementById('selPollInterval');
    const btnSidebar = document.getElementById('btnSidebar');
    const btnDetail = document.getElementById('btnDetail');

    if (pauseBtn) {
      pauseBtn.onclick = () => {
        state.paused = !state.paused;
        pauseBtn.textContent = state.paused ? 'Resume' : 'Pause';
        if (!state.paused) pollTimer = setTimeout(poll, 500);
        else clearTimeout(pollTimer);
      };
    }

    if (txtFilter) {
      txtFilter.addEventListener('input', () => {
        state.filterText = txtFilter.value;
        forceLayout = true;
        refresh();
      });
    }

    if (selStatus) {
      selStatus.addEventListener('change', () => {
        state.filterStatus = selStatus.value;
        forceLayout = true;
        refresh();
      });
    }

    if (selClaim) {
      selClaim.addEventListener('change', () => {
        state.filterClaim = selClaim.value;
        forceLayout = true;
        refresh();
      });
    }

    if (pollSel) {
      pollSel.addEventListener('change', () => {
        pollInterval = parseInt(pollSel.value, 10) || 10000;
      });
    }

    // Sidebar toggle (hotspots only)
    if (btnSidebar) {
      btnSidebar.onclick = () => {
        const sidebar = document.getElementById('sidebar');
        if (sidebar) {
          sidebar.classList.toggle('collapsed');
          btnSidebar.textContent = sidebar.classList.contains('collapsed') ? '\u25C0' : '\u25B6';
        }
      };
    }

    // Detail panel toggle (independent floating panel)
    if (btnDetail) {
      btnDetail.onclick = () => {
        const panel = document.getElementById('nodeDetail');
        if (!panel) return;
        if (panel.style.display === 'block') {
          panel.style.display = 'none';
        } else {
          panel.style.display = 'block';
        }
      };
    }

    // Node interactions
    cy.on('tap', 'node', (evt) => {
      const node = evt.target;
      showNodeDetail(node);
      // If detail is pinned open, update it
      const panel = document.getElementById('nodeDetail');
      if (panel && panel.dataset.pinned === 'true') {
        panel.dataset.lastNode = node.id();
      }
    });

    cy.on('tap', (evt) => {
      if (evt.target === cy) {
        const panel = document.getElementById('nodeDetail');
        if (panel && panel.dataset.pinned !== 'true') {
          hideNodeDetail();
        }
      }
    });

    cy.on('cxttap', 'node', (evt) => {
      showContextMenu(evt.originalEvent.clientX, evt.originalEvent.clientY, evt.target);
    });
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

  try {
    const seed = await fetchGraph();
    await ingestGraph(seed);
  } catch (e) {
    console.warn('initial graph load failed', e);
  }

  // Re-apply current filter on poll refresh
  pollTimer = setTimeout(poll, pollInterval);

  return cy;
}
