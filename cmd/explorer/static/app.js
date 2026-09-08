// ShadowForge block explorer — a thin, dependency-free client of a live
// validator's real pkg/query HTTP API. Every render function below only
// ever displays a field pkg/query itself already returns; nothing here
// decodes, decrypts, or infers anything pkg/query withholds (see the
// package doc on pkg/query for the exact boundary — shielded note
// contents and per-voter ballots are never exposed, by that server, at
// all, so there is nothing this client could show even if it tried).
//
// TX_KIND_NAMES mirrors pkg/types.TxKind's exact iota order
// (pkg/types/models.go) — the wire format carries a plain integer, not a
// string, so this mapping has to stay in sync with that file by hand.
'use strict';

const TX_KIND_NAMES = [
  'Transfer', 'Mint', 'Vote', 'BankDeposit', 'BankWithdraw',
  'NFTTrait', 'ContainerSync', 'VoteReveal', 'NFTMint', 'Unstake', 'NFTTransfer',
];

const ZERO_HASH = '0'.repeat(64);

function txKindName(k) {
  return TX_KIND_NAMES[k] || ('Unknown(' + k + ')');
}

function isZeroHex(h) {
  return !h || h === ZERO_HASH;
}

// --- node connection -------------------------------------------------

function defaultQueryBase() {
  return window.EXPLORER_DEFAULT_QUERY_BASE || 'http://127.0.0.1:8081';
}

function getQueryBase() {
  try {
    return localStorage.getItem('shadowforge-explorer-query-base') || defaultQueryBase();
  } catch (e) {
    return defaultQueryBase();
  }
}

function setQueryBase(url) {
  try {
    localStorage.setItem('shadowforge-explorer-query-base', url);
  } catch (e) { /* private-browsing / storage disabled: fine, just doesn't persist */ }
}

function setNodeStatus(ok) {
  const dot = document.getElementById('node-status');
  dot.classList.remove('status-ok', 'status-bad', 'status-unknown');
  dot.classList.add(ok === null ? 'status-unknown' : (ok ? 'status-ok' : 'status-bad'));
}

async function apiGet(path) {
  const base = getQueryBase().replace(/\/+$/, '');
  const resp = await fetch(base + path);
  let body = null;
  try { body = await resp.json(); } catch (e) { /* empty or non-JSON body */ }
  if (!resp.ok) {
    const err = new Error((body && body.error) || ('HTTP ' + resp.status));
    err.status = resp.status;
    throw err;
  }
  return body;
}

// --- small render helpers ---------------------------------------------

function el(tag, attrs, children) {
  const n = document.createElement(tag);
  for (const k in (attrs || {})) {
    if (k === 'class') n.className = attrs[k];
    else if (k === 'html') n.innerHTML = attrs[k];
    else n.setAttribute(k, attrs[k]);
  }
  for (const c of (children || [])) {
    n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return n;
}

function shortHash(h) {
  if (!h) return '—';
  return h.length > 16 ? h.slice(0, 8) + '…' + h.slice(-6) : h;
}

function hashSpan(h) {
  if (isZeroHex(h)) return el('span', { class: 'mono' }, ['—']);
  const span = el('span', { class: 'hash-short mono', title: h + ' (click to copy)' }, [shortHash(h)]);
  span.addEventListener('click', () => {
    if (navigator.clipboard) navigator.clipboard.writeText(h).catch(() => {});
  });
  return span;
}

function b64Span(b64) {
  if (!b64) return el('span', { class: 'mono' }, ['—']);
  return el('span', { class: 'hash-short mono', title: b64 }, [shortHash(b64)]);
}

function shieldedNote() {
  return el('span', { class: 'shielded-note' }, ['shielded — value not revealed by design']);
}

function kindPill(kind) {
  return el('span', { class: 'kind-pill kind-' + kind }, [kind]);
}

// formatRat renders a pkg/decimal.Decimal wire value (an exact rational
// string like "5/2" or "3", see pkg/decimal's own MarshalJSON doc) as a
// human-readable approximate decimal — display only, never fed back
// into anything.
function formatRat(s) {
  if (s === undefined || s === null || s === '') return '—';
  const parts = String(s).split('/');
  const num = Number(parts[0]);
  const den = parts.length > 1 ? Number(parts[1]) : 1;
  if (!isFinite(num) || !isFinite(den) || den === 0) return String(s);
  return (num / den).toLocaleString(undefined, { maximumFractionDigits: 6 });
}

function fieldRow(label, value) {
  const row = el('div', { class: 'field-row' }, [el('span', { class: 'field-label' }, [label])]);
  const val = el('span', { class: 'field-value' });
  if (value instanceof Node) val.appendChild(value); else val.textContent = value;
  row.appendChild(val);
  return row;
}

function proposalLink(id) {
  const a = el('a', { href: '#/proposal/' + encodeURIComponent(id) }, [id]);
  return a;
}

function nftLink(id) {
  const a = el('a', { href: '#/lookup?nft=' + encodeURIComponent(id) }, [shortHash(id)]);
  a.classList.add('mono');
  a.title = id;
  return a;
}

function addrSpan(addr) {
  return hashSpan(addr);
}

// --- view switching (hash-based routing) ------------------------------

const VIEWS = ['home', 'block', 'tx', 'governance', 'proposal', 'lookup', 'stats'];
const TAB_FOR_VIEW = { home: 'home', block: 'home', tx: 'home', governance: 'governance', proposal: 'governance', lookup: 'lookup', stats: 'stats' };

function showView(name) {
  for (const v of VIEWS) {
    document.getElementById('view-' + v).classList.toggle('active', v === name);
  }
  const tab = TAB_FOR_VIEW[name] || 'home';
  document.querySelectorAll('.tab-btn').forEach((btn) => {
    btn.classList.toggle('active', btn.dataset.view === tab);
  });
}

async function route() {
  const hash = location.hash.replace(/^#\/?/, '');
  const [pathPart, queryPart] = hash.split('?');
  const segs = pathPart.split('/').filter(Boolean);
  const params = new URLSearchParams(queryPart || '');

  if (segs.length === 0) {
    showView('home');
    await renderBlocksList(true);
    return;
  }
  switch (segs[0]) {
    case 'block':
      showView('block');
      await renderBlockDetail(segs[1]);
      return;
    case 'tx':
      showView('tx');
      await renderTxDetail(segs[1]);
      return;
    case 'governance':
      showView('governance');
      await renderProposalsList();
      return;
    case 'proposal':
      showView('proposal');
      await renderProposalDetail(decodeURIComponent(segs[1] || ''));
      return;
    case 'lookup':
      showView('lookup');
      prefillLookup(params);
      return;
    case 'stats':
      showView('stats');
      await renderStats();
      return;
    default:
      location.hash = '#/';
  }
}

// --- status strip ------------------------------------------------------

async function refreshStatus() {
  try {
    const status = await apiGet('/v1/status');
    setNodeStatus(true);
    const strip = document.getElementById('status-strip');
    strip.innerHTML = '';
    strip.appendChild(el('span', {}, ['height ', el('b', {}, [String(status.height)])]));
    strip.appendChild(el('span', {}, ['head ', el('span', { class: 'mono' }, [shortHash(status.head_hash)])]));
    strip.appendChild(el('span', {}, ['genesis ', new Date(status.genesis_ms).toISOString()]));
    return status;
  } catch (e) {
    setNodeStatus(false);
    const strip = document.getElementById('status-strip');
    strip.innerHTML = '';
    strip.appendChild(el('span', { class: 'pill-error' }, ['could not reach node: ' + e.message]));
    return null;
  }
}

// --- blocks -------------------------------------------------------------

let lastBlockCursor = null;
// pagedBeyondFirstPage tracks whether the viewer has clicked "load
// older" — auto-refresh only replaces the feed while they're still
// looking at just the first page, so it never yanks their place in a
// deeper scroll-back.
let pagedBeyondFirstPage = false;

function blockRow(b) {
  const row = el('div', { class: 'block-row' });
  row.appendChild(el('span', { class: 'block-height' }, ['#' + b.Height]));
  row.appendChild(el('span', { class: 'block-meta' }, [b.Batch ? (b.Batch.length + ' tx') : '0 tx']));
  row.appendChild(el('span', { class: 'hash-short mono' }, [shortHash(b.PrevHash)]));
  row.appendChild(el('span', { class: 'block-meta' }, [new Date(b.Timestamp).toLocaleString()]));
  row.addEventListener('click', () => { location.hash = '#/block/' + b.Height; });
  return row;
}

async function renderBlocksList(reset) {
  const list = document.getElementById('blocks-list');
  const moreBtn = document.getElementById('blocks-more');
  if (reset) {
    list.innerHTML = '';
    lastBlockCursor = null;
    pagedBeyondFirstPage = false;
  } else {
    pagedBeyondFirstPage = true;
  }
  try {
    let path = '/v1/blocks?limit=20';
    if (lastBlockCursor !== null) path += '&before=' + lastBlockCursor;
    const page = await apiGet(path);
    setNodeStatus(true);
    if (page.blocks.length === 0 && reset) {
      list.appendChild(el('div', { class: 'empty' }, ['no blocks yet']));
    }
    for (const b of page.blocks) list.appendChild(blockRow(b));
    if (page.blocks.length > 0) lastBlockCursor = page.blocks[page.blocks.length - 1].Height;
    moreBtn.hidden = !page.has_more;
  } catch (e) {
    setNodeStatus(false);
    list.innerHTML = '';
    list.appendChild(el('div', { class: 'error-box' }, ['could not load blocks: ' + e.message]));
    moreBtn.hidden = true;
  }
}

// --- block detail --------------------------------------------------------

function txRow(t) {
  const kind = txKindName(t.Kind);
  const row = el('div', { class: 'tx-row' }, [
    kindPill(kind), ' ', el('span', { class: 'mono' }, [shortHash(t.TxID)]),
  ]);
  row.addEventListener('click', () => { location.hash = '#/tx/' + t.TxID; });
  return row;
}

async function renderBlockDetail(heightStr) {
  const container = document.getElementById('block-detail');
  container.innerHTML = 'loading…';
  try {
    const b = await apiGet('/v1/blocks/' + encodeURIComponent(heightStr));
    setNodeStatus(true);
    container.innerHTML = '';
    const head = el('div', { class: 'card' });
    head.appendChild(el('h2', {}, ['Block #' + b.Height]));
    head.appendChild(fieldRow('Epoch', String(b.Epoch)));
    head.appendChild(fieldRow('Timestamp', new Date(b.Timestamp).toLocaleString()));
    head.appendChild(fieldRow('Previous hash', hashSpan(b.PrevHash)));
    head.appendChild(fieldRow('Tx root', hashSpan(b.TxRoot)));
    head.appendChild(fieldRow('State root', hashSpan(b.StateRoot)));
    head.appendChild(fieldRow('DA root', hashSpan(b.DARoot)));
    head.appendChild(fieldRow('Proposer', hashSpan(b.Proposer)));
    head.appendChild(fieldRow('Votes', String((b.Votes || []).length)));
    head.appendChild(fieldRow('Dual-track (outage recovery)', b.DualTrack ? 'yes' : 'no'));
    if (b.TalliedMintCommits && b.TalliedMintCommits.length > 0) {
      head.appendChild(fieldRow('Real epoch-mint notes minted this block',
        String(b.TalliedMintCommits.length) + ' (' +
        b.TalliedMintCommits.map(shortHash).join(', ') + ')'));
    }
    container.appendChild(head);

    const txHead = el('h3', {}, ['Transactions (' + (b.Batch || []).length + ')']);
    container.appendChild(txHead);
    if (!b.Batch || b.Batch.length === 0) {
      container.appendChild(el('div', { class: 'empty' }, ['no transactions in this block']));
    } else {
      for (const t of b.Batch) container.appendChild(txRow(t));
    }
  } catch (e) {
    setNodeStatus(e.status ? true : false);
    container.innerHTML = '';
    container.appendChild(el('div', { class: 'error-box' },
      [e.status === 404 ? ('no block at height ' + heightStr) : ('could not load block: ' + e.message)]));
  }
}

// --- tx-kind-aware field rendering (the privacy-critical part) ----------

function renderTxFields(tx) {
  const kind = txKindName(tx.Kind);
  const rows = [];
  rows.push(fieldRow('Kind', kindPill(kind)));
  rows.push(fieldRow('TxID', hashSpan(tx.TxID)));
  if (tx.ContainerID) rows.push(fieldRow('Container', String(tx.ContainerID)));

  switch (kind) {
    case 'Transfer': {
      const p = tx.TransferPublicInputs;
      rows.push(fieldRow('Amount', shieldedNote()));
      rows.push(fieldRow('Fee (public — only the fee, never the transferred value)', p ? String(p.FeeAmount) : '—'));
      rows.push(fieldRow('Merkle root', p ? hashSpan(p.MerkleRoot) : '—'));
      rows.push(fieldRow('Nullifiers', p && p.Nullifiers ? p.Nullifiers.map(shortHash).join('  ') : '—'));
      rows.push(fieldRow('Output commitments', p && p.OutCommits ? p.OutCommits.map(shortHash).join('  ') : '—'));
      break;
    }
    case 'BankDeposit':
    case 'BankWithdraw': {
      const p = tx.BankPublicInputs;
      rows.push(fieldRow('Amount', shieldedNote()));
      rows.push(fieldRow('Asset', p ? p.Asset : '—'));
      rows.push(fieldRow('Oracle price (USD)', p ? formatRat(p.OraclePriceUSD) : '—'));
      rows.push(fieldRow('ATR (USD)', p ? formatRat(p.ATRUSD) : '—'));
      rows.push(fieldRow('Buffer (USD)', p ? formatRat(p.BufferUSD) : '—'));
      rows.push(fieldRow('Fee commitment', hashSpan(tx.FeeCommit)));
      break;
    }
    case 'Vote': {
      const p = tx.VotePublicInputs;
      if (p) {
        rows.push(fieldRow('Proposal', proposalLink(p.ProposalID)));
        rows.push(fieldRow('Sealed ballot commitment', hashSpan(p.Commitment)));
        if (p.ParamKey) rows.push(fieldRow('Param change requested', p.ParamKey + ' → ' + p.NewValue));
        if (p.MintAmount) {
          rows.push(fieldRow(p.MintStaked ? 'Staked mint requested' : 'Direct mint requested', String(p.MintAmount)));
          rows.push(fieldRow(p.MintStaked ? 'Stake position commitment' : 'Mint output commitment',
            hashSpan(p.MintStaked ? p.StakePositionCommit : p.MintOutCommit)));
        }
        if (!isZeroHex(p.SlashTargetNFT)) {
          rows.push(fieldRow('Slash proposal target', nftLink(p.SlashTargetNFT)));
          rows.push(fieldRow('Slash outcome requested', p.SlashBurn ? 'burn' : 'freeze'));
        }
        if (!isZeroHex(p.UnlockTransferTarget)) rows.push(fieldRow('Unlock-transfer target', nftLink(p.UnlockTransferTarget)));
        if (p.ContainerAssetTarget) rows.push(fieldRow('Bank asset authorization requested', p.ContainerAssetTarget));
        if (p.UnwindDualSign) rows.push(fieldRow('Dual-sign retirement requested', 'yes'));
      }
      break;
    }
    case 'VoteReveal': {
      const p = tx.VoteRevealPublicInputs;
      if (p) {
        rows.push(fieldRow('Proposal', proposalLink(p.ProposalID)));
        rows.push(fieldRow('Revealed ballot', p.Approve ? 'approve' : 'reject'));
      }
      break;
    }
    case 'NFTMint': {
      const p = tx.NFTMintPublicInputs;
      if (p) {
        rows.push(fieldRow('Owner', addrSpan(p.Owner)));
        rows.push(fieldRow('Nonce', String(p.Nonce)));
        rows.push(fieldRow('Attestor pubkey', b64Span(p.Attestor)));
        rows.push(fieldRow('Attested at', new Date(p.AttestationIssuedAtMs).toLocaleString()));
      }
      break;
    }
    case 'NFTTransfer': {
      const p = tx.NFTTransferPublicInputs;
      if (p) {
        rows.push(fieldRow('Target NFT', nftLink(p.Target)));
        rows.push(fieldRow('New owner', addrSpan(p.NewOwner)));
      }
      break;
    }
    case 'NFTTrait': {
      const p = tx.TraitPublicInputs;
      if (p) {
        rows.push(fieldRow('Trait key', p.Key));
        rows.push(fieldRow('Delta commitment (value hidden)', hashSpan(p.DeltaCommitment)));
      }
      break;
    }
    case 'Unstake': {
      const p = tx.UnstakePublicInputs;
      if (p) {
        rows.push(fieldRow('Principal', String(p.Principal)));
        rows.push(fieldRow('Start epoch', String(p.StartEpoch)));
        rows.push(fieldRow('Final amount (principal + real accrued yield)', String(p.FinalAmount)));
        rows.push(fieldRow('Merkle root', hashSpan(p.MerkleRoot)));
      }
      rows.push(fieldRow('Resulting note commitment', tx.Commitments && tx.Commitments[0] ? hashSpan(tx.Commitments[0]) : '—'));
      break;
    }
    case 'Mint':
      rows.push(fieldRow('Note', 'vestigial no-op kind — accepted but has no on-chain effect (see pkg/types.TxMint)'));
      break;
    case 'ContainerSync':
      rows.push(fieldRow('Note', 'enterprise container sync — no additional public fields exposed by this API'));
      break;
  }
  return rows;
}

async function renderTxDetail(txid) {
  const container = document.getElementById('tx-detail');
  container.innerHTML = 'loading…';
  try {
    const status = await apiGet('/v1/tx/' + encodeURIComponent(txid));
    setNodeStatus(true);
    container.innerHTML = '';
    const card = el('div', { class: 'card' });
    card.appendChild(el('h2', {}, ['Transaction']));
    card.appendChild(fieldRow('TxID', hashSpan(txid)));
    card.appendChild(fieldRow('Status', status.status));
    if (status.status === 'committed') {
      card.appendChild(fieldRow('Block height', el('a', { href: '#/block/' + status.height }, [String(status.height)])));
      if (status.tx) {
        for (const row of renderTxFields(status.tx)) card.appendChild(row);
      } else {
        card.appendChild(el('div', { class: 'empty' }, ['committed, but this node did not return the tx content']));
      }
    } else if (status.status === 'pending') {
      card.appendChild(el('div', { class: 'empty' }, ['still in the mempool — not yet committed to a block']));
    } else {
      card.appendChild(el('div', { class: 'empty' }, ['unknown to this node — never seen, or already evicted before committing']));
    }
    container.appendChild(card);
  } catch (e) {
    setNodeStatus(false);
    container.innerHTML = '';
    container.appendChild(el('div', { class: 'error-box' }, ['could not load tx: ' + e.message]));
  }
}

// --- governance -----------------------------------------------------------

function proposalRow(p) {
  const row = el('div', { class: 'proposal-row' });
  row.appendChild(el('span', {}, [el('b', {}, [p.proposal_id])]));
  row.appendChild(el('span', { class: 'block-meta' }, ['epoch ' + p.epoch]));
  const statusText = !p.tallied ? 'open' : (p.passed ? 'passed' : 'rejected');
  row.appendChild(el('span', { class: 'kind-pill ' + (p.passed ? 'kind-Vote' : '') }, [statusText]));
  row.appendChild(el('span', { class: 'block-meta' }, [p.approve + ' approve / ' + p.reject + ' reject']));
  row.addEventListener('click', () => { location.hash = '#/proposal/' + encodeURIComponent(p.proposal_id); });
  return row;
}

async function renderProposalsList() {
  const list = document.getElementById('proposals-list');
  list.innerHTML = 'loading…';
  try {
    const proposals = await apiGet('/v1/proposals');
    setNodeStatus(true);
    list.innerHTML = '';
    if (proposals.length === 0) {
      list.appendChild(el('div', { class: 'empty' }, ['no proposals yet']));
      return;
    }
    proposals.sort((a, b) => b.epoch - a.epoch || a.proposal_id.localeCompare(b.proposal_id));
    for (const p of proposals) list.appendChild(proposalRow(p));
  } catch (e) {
    setNodeStatus(false);
    list.innerHTML = '';
    list.appendChild(el('div', { class: 'error-box' }, ['could not load proposals: ' + e.message]));
  }
}

async function renderProposalDetail(id) {
  const container = document.getElementById('proposal-detail');
  container.innerHTML = 'loading…';
  try {
    const p = await apiGet('/v1/proposal/' + encodeURIComponent(id));
    setNodeStatus(true);
    container.innerHTML = '';
    const card = el('div', { class: 'card' });
    card.appendChild(el('h2', {}, ['Proposal ' + p.proposal_id]));
    card.appendChild(fieldRow('Epoch', String(p.epoch)));
    card.appendChild(fieldRow('Tallied', p.tallied ? 'yes' : 'no (still open)'));
    if (p.tallied) {
      card.appendChild(fieldRow('Approve / reject', p.approve + ' / ' + p.reject));
      card.appendChild(fieldRow('Passed', p.passed ? 'yes' : 'no'));
    }
    if (p.param_key) {
      card.appendChild(fieldRow('Param change', p.param_key + ' → ' + p.new_value));
      card.appendChild(fieldRow('Applied', p.applied ? 'yes' : 'no'));
    }
    if (p.mint_amount) {
      card.appendChild(fieldRow('Mint amount requested', String(p.mint_amount)));
      card.appendChild(fieldRow('Mint path', p.mint_staked ? 'staked (2% yield)' : 'direct (10% fee)'));
      card.appendChild(fieldRow('Mint applied', p.mint_applied ? 'yes' : 'no'));
      if (p.mint_out_commit) card.appendChild(fieldRow('Mint output commitment', hashSpan(p.mint_out_commit)));
      if (p.stake_position_commit) card.appendChild(fieldRow('Stake position commitment', hashSpan(p.stake_position_commit)));
    }
    if (p.slash_target_nft) {
      card.appendChild(fieldRow('Slash target NFT', nftLink(p.slash_target_nft)));
      card.appendChild(fieldRow('Slash outcome requested', p.slash_burn ? 'burn' : 'freeze'));
      card.appendChild(fieldRow('Slash applied', p.slash_applied ? 'yes' : 'no'));
    }
    if (p.unlock_transfer_target) {
      card.appendChild(fieldRow('Unlock-transfer target NFT', nftLink(p.unlock_transfer_target)));
      card.appendChild(fieldRow('Unlock applied', p.unlock_transfer_applied ? 'yes' : 'no'));
    }
    if (p.container_asset_target) {
      card.appendChild(fieldRow('Bank asset authorization requested', p.container_asset_target));
      card.appendChild(fieldRow('Authorization applied', p.container_asset_applied ? 'yes' : 'no'));
    }
    if (p.unwind_dual_sign) {
      card.appendChild(fieldRow('Dual-sign retirement requested', 'yes'));
      card.appendChild(fieldRow('Retirement applied', p.unwind_dual_sign_applied ? 'yes' : 'no'));
    }
    container.appendChild(card);
  } catch (e) {
    setNodeStatus(e.status ? true : false);
    container.innerHTML = '';
    container.appendChild(el('div', { class: 'error-box' },
      [e.status === 404 ? ('no proposal with id ' + id) : ('could not load proposal: ' + e.message)]));
  }
}

// --- direct lookups ---------------------------------------------------

async function runLookup(kind) {
  const input = document.querySelector('[data-lookup="' + kind + '"]');
  const out = document.querySelector('[data-lookup-result="' + kind + '"]');
  const value = input.value.trim();
  out.innerHTML = '';
  if (!value) return;
  try {
    if (kind === 'nullifier') {
      const r = await apiGet('/v1/nullifier/' + encodeURIComponent(value));
      out.appendChild(el('span', { class: r.spent ? 'pill-yes' : 'pill-no' }, [r.spent ? 'spent' : 'not spent']));
    } else if (kind === 'note') {
      const r = await apiGet('/v1/note/' + encodeURIComponent(value));
      out.appendChild(el('span', { class: r.exists ? 'pill-yes' : 'pill-no' }, [r.exists ? 'exists' : 'not found']));
    } else if (kind === 'nft') {
      const r = await apiGet('/v1/nft/' + encodeURIComponent(value));
      out.appendChild(fieldRow('Owner', addrSpan(r.Owner)));
      out.appendChild(fieldRow('Trust points', String(r.TP)));
      out.appendChild(fieldRow('Slashed', r.Slashed ? 'yes' : 'no'));
      out.appendChild(fieldRow('Minted at height', String(r.MintedAt)));
      const traits = r.Traits ? Object.entries(r.Traits).map(([k, v]) => k + '=' + v).join(', ') : '';
      out.appendChild(fieldRow('Traits', traits || '(none)'));
    } else if (kind === 'hold') {
      const r = await apiGet('/v1/hold/' + encodeURIComponent(value));
      out.appendChild(fieldRow('Owner', addrSpan(r.Owner)));
      out.appendChild(fieldRow('Asset', r.ExternalAsset));
      out.appendChild(fieldRow('SFG issued', String(r.SFGIssued)));
      out.appendChild(fieldRow('Status', String(r.Status)));
    }
    setNodeStatus(true);
  } catch (e) {
    setNodeStatus(e.status ? true : false);
    out.appendChild(el('span', { class: 'pill-error' },
      [e.status === 404 ? 'not found' : ('error: ' + e.message)]));
  }
}

function prefillLookup(params) {
  for (const kind of ['nullifier', 'note', 'nft', 'hold']) {
    if (params.has(kind)) {
      const input = document.querySelector('[data-lookup="' + kind + '"]');
      input.value = params.get(kind);
      runLookup(kind);
    }
  }
}

// --- supply stats -------------------------------------------------------

async function renderStats() {
  const body = document.getElementById('stats-body');
  body.innerHTML = 'loading…';
  try {
    const proposals = await apiGet('/v1/proposals');
    setNodeStatus(true);
    let directNet = 0, directFees = 0, staked = 0, mintedProposals = 0;
    for (const p of proposals) {
      if (!p.mint_applied) continue;
      mintedProposals++;
      if (p.mint_staked) {
        staked += p.mint_amount;
      } else {
        const fee = Math.floor(p.mint_amount / 10); // mirrors types.MintFeeAmount (1/10 floor)
        directNet += p.mint_amount - fee;
        directFees += fee;
      }
    }
    body.innerHTML = '';
    const stat = (label, value) => {
      const card = el('div', { class: 'stat-card' });
      card.appendChild(el('div', { class: 'label' }, [label]));
      card.appendChild(el('div', { class: 'value' }, [value]));
      return card;
    };
    body.appendChild(stat('Real minted supply (direct path, net of fee)', directNet.toLocaleString()));
    body.appendChild(stat('Vault fees collected (direct path)', directFees.toLocaleString()));
    body.appendChild(stat('Locked in staked positions (pre-yield)', staked.toLocaleString()));
    body.appendChild(stat('Passed & applied mint proposals', String(mintedProposals)));
  } catch (e) {
    setNodeStatus(false);
    body.innerHTML = '';
    body.appendChild(el('div', { class: 'error-box' }, ['could not compute supply: ' + e.message]));
  }
}

// --- search --------------------------------------------------------------

async function doSearch(raw) {
  const value = raw.trim();
  if (!value) return;
  // A 64-hex-char string must be checked before the plain-numeric case:
  // one made entirely of the digits 0-9 (no a-f) — a real txid or hash
  // like "0200...00" — would otherwise also match /^\d+$/ and get
  // misrouted to a nonsensical block-height lookup. No real block
  // height is ever remotely close to 64 digits long, so length alone
  // disambiguates safely.
  if (/^[0-9a-fA-F]{64}$/.test(value)) {
    const hex = value.toLowerCase();
    try {
      const tx = await apiGet('/v1/tx/' + hex);
      if (tx.status !== 'unknown') { location.hash = '#/tx/' + hex; return; }
    } catch (e) { /* fall through */ }
    try {
      await apiGet('/v1/nft/' + hex);
      location.hash = '#/lookup?nft=' + hex;
      return;
    } catch (e) { /* not an NFT id, fall through */ }
    try {
      await apiGet('/v1/hold/' + hex);
      location.hash = '#/lookup?hold=' + hex;
      return;
    } catch (e) { /* not a hold id, fall through */ }
    // Neither errors, so show both possibilities together — nullifier
    // and note lookups always answer 200 with a bool either way.
    location.hash = '#/lookup?nullifier=' + hex + '&note=' + hex;
    return;
  }
  if (/^\d+$/.test(value)) {
    location.hash = '#/block/' + value;
    return;
  }
  location.hash = '#/proposal/' + encodeURIComponent(value);
}

// --- wiring ---------------------------------------------------------------

function init() {
  const nodeInput = document.getElementById('node-input');
  nodeInput.value = getQueryBase();
  document.getElementById('node-apply').addEventListener('click', () => {
    setQueryBase(nodeInput.value.trim() || defaultQueryBase());
    refreshStatus();
    route();
  });
  nodeInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') document.getElementById('node-apply').click();
  });

  document.querySelectorAll('.tab-btn').forEach((btn) => {
    btn.addEventListener('click', () => { location.hash = '#/' + (btn.dataset.view === 'home' ? '' : btn.dataset.view); });
  });

  document.querySelectorAll('[data-back]').forEach((btn) => {
    btn.addEventListener('click', () => { location.hash = '#/' + btn.dataset.back; });
  });

  const searchInput = document.getElementById('search-input');
  document.getElementById('search-btn').addEventListener('click', () => doSearch(searchInput.value));
  searchInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') doSearch(searchInput.value); });

  document.getElementById('blocks-refresh').addEventListener('click', () => renderBlocksList(true));
  document.getElementById('blocks-more').addEventListener('click', () => renderBlocksList(false));

  document.querySelectorAll('[data-lookup-btn]').forEach((btn) => {
    const kind = btn.dataset.lookupBtn;
    btn.addEventListener('click', () => runLookup(kind));
    document.querySelector('[data-lookup="' + kind + '"]').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') runLookup(kind);
    });
  });

  window.addEventListener('hashchange', route);
  refreshStatus();
  route();

  // Auto-refresh: status always; the latest blocks page only while the
  // viewer is looking at the home view and hasn't paged further back —
  // a real, simple "new block arrived" feed, not a full live-diff.
  setInterval(() => {
    refreshStatus();
    const onHome = location.hash === '' || location.hash === '#/' || location.hash === '#';
    if (onHome && !pagedBeyondFirstPage) {
      renderBlocksList(true);
    }
  }, 5000);
}

document.addEventListener('DOMContentLoaded', init);
