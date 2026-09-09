// ShadowForge status page frontend — polls this same server's own
// /api/snapshot (never a node's query API directly: uptime history is
// this server's own observed state, computed by pkg/statuscheck from
// real, timestamped checks — see cmd/statuspage's own doc).
'use strict';

function el(tag, attrs, children) {
  const n = document.createElement(tag);
  for (const k in (attrs || {})) {
    if (k === 'class') n.className = attrs[k];
    else if (k === 'title') n.title = attrs[k];
    else n.setAttribute(k, attrs[k]);
  }
  for (const c of (children || [])) {
    n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return n;
}

function fmtTime(iso) {
  try { return new Date(iso).toLocaleString(); } catch (e) { return iso; }
}

function historyBar(check) {
  const up = !!check.up;
  const title = fmtTime(check.at) + ' — ' + (up ? ('up, height ' + check.height + ', ' + check.latency_ms + 'ms') : ('down' + (check.error ? (': ' + check.error) : '')));
  return el('div', { class: 'history-bar ' + (up ? 'up' : 'down'), title });
}

function nodeCard(node) {
  const latest = node.latest;
  const dotClass = latest ? (latest.up ? 'up' : 'down') : 'unknown';
  const card = el('div', { class: 'node-card' });

  const head = el('div', { class: 'node-head' });
  head.appendChild(el('div', { class: 'node-name' }, [
    el('span', { class: 'status-dot ' + dotClass }, []),
    node.name,
  ]));
  head.appendChild(el('div', { class: 'node-meta' }, [
    el('span', {}, ['uptime ', el('b', {}, [node.uptime_percent.toFixed(1) + '%']), ' (last ' + node.checks.length + ' checks)']),
  ]));
  card.appendChild(head);

  const meta = el('div', { class: 'node-meta' });
  if (latest && latest.up) {
    meta.appendChild(el('span', {}, ['height ', el('b', {}, [String(latest.height)])]));
    meta.appendChild(el('span', {}, ['latency ', el('b', {}, [latest.latency_ms + 'ms'])]));
    meta.appendChild(el('span', {}, ['last check ', fmtTime(latest.at)]));
  } else if (latest) {
    meta.appendChild(el('span', {}, ['last check ', fmtTime(latest.at)]));
  } else {
    meta.appendChild(el('span', {}, ['no checks recorded yet']));
  }
  meta.appendChild(el('span', {}, [node.query_base]));
  card.appendChild(meta);

  if (latest && !latest.up && latest.error) {
    card.appendChild(el('div', { class: 'error-note' }, [latest.error]));
  }

  const strip = el('div', { class: 'history-strip' });
  if (node.checks.length === 0) {
    strip.appendChild(el('div', { class: 'empty' }, ['(no history yet)']));
  } else {
    for (const c of node.checks) strip.appendChild(historyBar(c));
  }
  card.appendChild(strip);

  return card;
}

function renderOverallBanner(nodes) {
  const banner = document.getElementById('overall-banner');
  banner.className = 'overall-banner';
  if (nodes.length === 0) {
    banner.classList.add('banner-unknown');
    banner.textContent = 'no nodes configured';
    return;
  }
  const upCount = nodes.filter((n) => n.latest && n.latest.up).length;
  if (upCount === nodes.length) {
    banner.classList.add('banner-good');
    banner.textContent = 'All ' + nodes.length + ' monitored node(s) are up';
  } else if (upCount === 0) {
    banner.classList.add('banner-bad');
    banner.textContent = 'All ' + nodes.length + ' monitored node(s) are down';
  } else {
    banner.classList.add('banner-partial');
    banner.textContent = upCount + ' of ' + nodes.length + ' monitored node(s) are up';
  }
}

async function refresh() {
  try {
    const resp = await fetch('/api/snapshot');
    if (!resp.ok) throw new Error('HTTP ' + resp.status);
    const nodes = await resp.json();
    renderOverallBanner(nodes);
    const container = document.getElementById('nodes');
    container.innerHTML = '';
    for (const n of nodes) container.appendChild(nodeCard(n));
    document.getElementById('updated').textContent = 'updated ' + new Date().toLocaleTimeString();
  } catch (e) {
    document.getElementById('updated').textContent = 'could not reach statuspage server: ' + e.message;
  }
}

function init() {
  const seconds = window.STATUSPAGE_REFRESH_SECONDS || 30;
  const label = document.getElementById('interval-label');
  if (label) label.textContent = seconds + 's';
  refresh();
  setInterval(refresh, Math.max(5, Math.min(seconds, 30)) * 1000);
}

document.addEventListener('DOMContentLoaded', init);
