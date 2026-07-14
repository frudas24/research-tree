// Visual encoding for research-tree nodes in Cytoscape

// Status → fill color
export function colorByStatus(status) {
  switch (status) {
    case 'done':    return '#3fb950'; // green
    case 'active':  return '#eab308'; // amber
    case 'paused':  return '#6b7280'; // gray
    default:        return '#6b7280';
  }
}

// Claim status → border style
export function borderByClaim(claim) {
  switch (claim) {
    case 'validated':   return { color: '#3fb950', width: 3, style: 'solid' };
    case 'invalidated': return { color: '#f85149', width: 3, style: 'dashed' };
    case 'superseded':  return { color: '#a855f7', width: 2, style: 'dotted' };
    case 'provisional':
    default:            return { color: '#374151', width: 1, style: 'solid' };
  }
}

// Evidence status → overlay tint
export function overlayByEvidence(ev) {
  switch (ev) {
    case 'poisoned':    return { color: '#f85149', opacity: 0.35 };
    case 'suspect':     return { color: '#eab308', opacity: 0.25 };
    case 'revalidated': return { color: '#3fb950', opacity: 0.15 };
    case 'clean':
    default:            return null;
  }
}

// Children count → node size: 18 + min(50, sqrt(children) * 10)
export function sizeByChildren(n) {
  if (!Number.isFinite(n) || n <= 0) return 22;
  return 18 + Math.min(50, Math.sqrt(n) * 10);
}

// Golden milestone → diamond shape
export function shapeByMilestone(cls) {
  return cls === 'golden' ? 'diamond' : 'ellipse';
}

// Relation type → edge style
export function edgeStyleByRelation(type) {
  switch (type) {
    case 'depends_on':       return { color: '#f97316', width: 2, style: 'dashed' };
    case 'compares_against': return { color: '#58a6ff', width: 2, style: 'dashed' };
    case 'inspired_by':      return { color: '#a855f7', width: 2, style: 'dotted' };
    case 'aggregates':       return { color: '#3fb950', width: 2, style: 'dotted' };
    default:                 return { color: '#6b7280', width: 2, style: 'dashed' };
  }
}

export function style() {
  return [
    // Parent edges (DAG structural)
    {
      selector: 'edge[type = "parent"]',
      style: {
        'line-color': '#94a3b8',
        'target-arrow-color': '#94a3b8',
        'target-arrow-shape': 'triangle',
        'width': ele => 1 + Math.min(4, Math.log2(1 + (ele.data('weight') || 1))),
        'curve-style': 'bezier',
      },
    },
    // Relation edges — colored by relation type
    {
      selector: 'edge[type = "depends_on"]',
      style: {
        'line-color': '#f97316',
        'target-arrow-color': '#f97316',
        'target-arrow-shape': 'triangle',
        'width': 2,
        'line-style': 'dashed',
        'curve-style': 'bezier',
      },
    },
    {
      selector: 'edge[type = "compares_against"]',
      style: {
        'line-color': '#58a6ff',
        'target-arrow-color': '#58a6ff',
        'target-arrow-shape': 'triangle',
        'width': 2,
        'line-style': 'dashed',
        'curve-style': 'bezier',
      },
    },
    {
      selector: 'edge[type = "inspired_by"]',
      style: {
        'line-color': '#a855f7',
        'target-arrow-color': '#a855f7',
        'target-arrow-shape': 'triangle',
        'width': 2,
        'line-style': 'dotted',
        'curve-style': 'bezier',
      },
    },
    {
      selector: 'edge[type = "aggregates"]',
      style: {
        'line-color': '#3fb950',
        'target-arrow-color': '#3fb950',
        'target-arrow-shape': 'triangle',
        'width': 2,
        'line-style': 'dotted',
        'curve-style': 'bezier',
      },
    },
    // Base node style
    {
      selector: 'node',
      style: {
        'label': 'data(label)',
        'text-wrap': 'wrap',
        'text-max-width': 160,
        'font-size': 10,
        'color': '#e6edf3',
        'text-valign': 'center',
        'text-halign': 'center',
        'background-color': ele => colorByStatus(ele.data('status')),
        'border-color': ele => borderByClaim(ele.data('claim_status')).color,
        'border-width': ele => borderByClaim(ele.data('claim_status')).width,
        'border-style': ele => borderByClaim(ele.data('claim_status')).style,
        'width': ele => sizeByChildren(ele.data('children') || 0),
        'height': ele => sizeByChildren(ele.data('children') || 0),
        'shape': ele => shapeByMilestone(ele.data('milestone_class')),
        'transition-property': 'background-color, border-color, border-width',
        'transition-duration': '300ms',
      },
    },
    // Evidence poisoned overlay
    {
      selector: 'node[evidence_status = "poisoned"]',
      style: {
        'overlay-color': '#f85149',
        'overlay-opacity': 0.3,
        'border-color': '#f85149',
        'border-width': 3,
      },
    },
    {
      selector: 'node[evidence_status = "suspect"]',
      style: {
        'overlay-color': '#eab308',
        'overlay-opacity': 0.25,
      },
    },
    // Golden milestone emphasis
    {
      selector: 'node[milestone_class = "golden"]',
      style: {
        'border-width': 4,
        'border-color': '#fbbf24',
        'text-outline-color': '#fbbf24',
        'text-outline-width': 1,
      },
    },
    // Hotspot pulse
    {
      selector: 'node.hot',
      style: {
        'overlay-color': '#14b8a6',
        'overlay-opacity': 0.35,
        'border-color': '#14b8a6',
        'border-width': 4,
      },
    },
    {
      selector: 'edge.hot',
      style: {
        'line-color': '#14b8a6',
        'target-arrow-color': '#14b8a6',
        'width': 5,
      },
    },
  ];
}
