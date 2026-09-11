// app.js drives the three-step mint flow purely by calling this page's
// own real server endpoints (/api/generate, /api/identity, /api/attest,
// /api/mint) — see cmd/mintpage's Go handlers for what each one actually
// does. No cryptography happens in this file; it only collects form
// input, uploads it, and renders back whatever the server (which does
// the real signing) returns.

const state = {
  // The active identity's keystore, as a Blob/File — set either by
  // generating a fresh one (kept in memory, never written to this
  // browser's disk) or by picking an existing keystore.json file. The
  // Mint step reuses this automatically if no separate file is chosen
  // there, so a freshly generated identity can mint immediately without
  // manually re-uploading the file just downloaded.
  keystoreBlob: null,
  address: null,
};

function el(id) { return document.getElementById(id); }

function showResult(target, cls, html) {
  target.hidden = false;
  target.className = "result " + cls;
  target.innerHTML = html;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

async function apiJSON(path, body) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

async function apiForm(path, formData) {
  const res = await fetch(path, { method: "POST", body: formData });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function hexToBytes(hex) {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.substr(i * 2, 2), 16);
  return out;
}

function downloadLink(filename, blob, label) {
  const url = URL.createObjectURL(blob);
  return `<a href="${url}" download="${escapeHtml(filename)}">${escapeHtml(label)}</a>`;
}

function setActiveIdentity(address, keystoreBlob) {
  state.address = address;
  state.keystoreBlob = keystoreBlob;
  el("identity-banner").hidden = false;
  el("identity-address").textContent = address;
  if (!el("attest-owner").value) el("attest-owner").value = address;
}

// --- Step 1: tabs ---
document.querySelectorAll(".tab").forEach((btn) => {
  btn.addEventListener("click", () => {
    document.querySelectorAll(".tab").forEach((b) => b.classList.remove("active"));
    document.querySelectorAll(".tab-panel").forEach((p) => p.classList.add("hidden"));
    btn.classList.add("active");
    el("tab-" + btn.dataset.tab).classList.remove("hidden");
  });
});

// --- Step 1: generate ---
el("gen-btn").addEventListener("click", async () => {
  const passphrase = el("gen-passphrase").value;
  const btn = el("gen-btn");
  const result = el("gen-result");
  if (passphrase.length < 8) {
    showResult(result, "err", "Passphrase must be at least 8 characters.");
    return;
  }
  btn.disabled = true;
  showResult(result, "pending", "Generating a real Dilithium + X25519 identity…");
  try {
    const data = await apiJSON("/api/generate", { passphrase });
    const keystoreBlob = new Blob([data.keystore_json], { type: "application/json" });
    const nodeIdentityBlob = new Blob([hexToBytes(data.node_identity_hex)], { type: "application/octet-stream" });
    showResult(result, "ok", `
      <dl>
        <dt>Address</dt><dd>${escapeHtml(data.address)}</dd>
      </dl>
      <p>Save both files now — neither is recoverable if lost, and this server keeps no copy:</p>
      <div class="downloads">
        ${downloadLink("walletkey.json", keystoreBlob, "Download keystore (walletkey.json)")}
        ${downloadLink("node-identity.key", nodeIdentityBlob, "Download node identity (-key-file)")}
      </div>
      <p class="hint">Point your validator's <code>-key-file</code> at node-identity.key so it
      heartbeats as this same address.</p>
    `);
    setActiveIdentity(data.address, keystoreBlob);
  } catch (e) {
    showResult(result, "err", escapeHtml(e.message));
  } finally {
    btn.disabled = false;
  }
});

// --- Step 1: load existing ---
el("load-btn").addEventListener("click", async () => {
  const file = el("load-file").files[0];
  const result = el("load-result");
  if (!file) {
    showResult(result, "err", "Choose a keystore.json file first.");
    return;
  }
  const btn = el("load-btn");
  btn.disabled = true;
  showResult(result, "pending", "Reading keystore…");
  try {
    const fd = new FormData();
    fd.append("keystore", file);
    const data = await apiForm("/api/identity", fd);
    showResult(result, "ok", `<dl><dt>Address</dt><dd>${escapeHtml(data.address)}</dd></dl>`);
    setActiveIdentity(data.address, file);
  } catch (e) {
    showResult(result, "err", escapeHtml(e.message));
  } finally {
    btn.disabled = false;
  }
});

// --- Step 2: keep attest-nonce in sync with mint-nonce when opened ---
el("attestor-panel").addEventListener("toggle", () => {
  if (el("attestor-panel").open && !el("attest-nonce").value) {
    el("attest-nonce").value = el("mint-nonce").value;
  }
});

// --- Step 2: attest ---
el("attest-btn").addEventListener("click", async () => {
  const file = el("attest-keystore").files[0];
  const result = el("attest-result");
  if (!file) {
    showResult(result, "err", "Choose the attestor's own keystore file.");
    return;
  }
  const owner = el("attest-owner").value.trim();
  const nonce = el("attest-nonce").value;
  if (!owner || nonce === "") {
    showResult(result, "err", "Owner address and nonce are required.");
    return;
  }
  const btn = el("attest-btn");
  btn.disabled = true;
  showResult(result, "pending", "Signing attestation…");
  try {
    const fd = new FormData();
    fd.append("keystore", file);
    fd.append("passphrase", el("attest-passphrase").value);
    fd.append("owner", owner);
    fd.append("nonce", nonce);
    const data = await apiForm("/api/attest", fd);
    el("mint-nonce").value = data.nonce;
    el("mint-issued-at-ms").value = data.issued_at_ms;
    el("mint-attestor-pubkey").value = data.attestor_pubkey;
    el("mint-attestation-sig").value = data.attestation_sig;
    showResult(result, "ok", `Attestation signed, valid ${Math.round(data.valid_for_seconds / 60)} minutes — fields below filled in automatically.`);
  } catch (e) {
    showResult(result, "err", escapeHtml(e.message));
  } finally {
    btn.disabled = false;
  }
});

// --- Step 3: mint ---
el("mint-btn").addEventListener("click", async () => {
  const file = el("mint-keystore").files[0] || state.keystoreBlob;
  const result = el("mint-result");
  if (!file) {
    showResult(result, "err", "No identity selected — generate or load one in step 1, or choose a keystore file here.");
    return;
  }
  const btn = el("mint-btn");
  btn.disabled = true;
  showResult(result, "pending", "Submitting and waiting for confirmation — this can take up to a minute…");
  try {
    const fd = new FormData();
    fd.append("keystore", file, "keystore.json");
    fd.append("passphrase", el("mint-passphrase").value);
    fd.append("nonce", el("mint-nonce").value);
    fd.append("issued_at_ms", el("mint-issued-at-ms").value);
    fd.append("attestor_pubkey", el("mint-attestor-pubkey").value);
    fd.append("attestation_sig", el("mint-attestation-sig").value);
    fd.append("bootstrap", el("mint-bootstrap").value.trim());
    fd.append("query", el("mint-query").value.trim());
    const data = await apiForm("/api/mint", fd);
    const ok = data.status === "committed";
    const cls = ok ? "ok" : (data.status === "submit-failed" ? "err" : "pending");
    showResult(result, cls, `
      <dl>
        <dt>Transaction ID</dt><dd>${escapeHtml(data.txid)}</dd>
        <dt>Status</dt><dd>${escapeHtml(data.status)}${data.height ? " (height " + data.height + ")" : ""}</dd>
        ${data.error ? `<dt>Detail</dt><dd>${escapeHtml(data.error)}</dd>` : ""}
      </dl>
      ${ok ? "<p>NFT minted — this address can now run a node and vote.</p>" :
        `<p>Not confirmed yet. If this address already holds an NFT, or the attestation was
        wrong/expired/untrusted, the real pipeline silently drops the transaction rather than
        reporting a reason (see pkg/query's own doc) — check the node's own logs, or retry with
        a fresh attestation and a new nonce.</p>`}
    `);
  } catch (e) {
    showResult(result, "err", escapeHtml(e.message));
  } finally {
    btn.disabled = false;
  }
});

// --- prefill network defaults ---
window.addEventListener("DOMContentLoaded", () => {
  if (window.MINTPAGE_DEFAULT_BOOTSTRAP) el("mint-bootstrap").value = window.MINTPAGE_DEFAULT_BOOTSTRAP;
  if (window.MINTPAGE_DEFAULT_QUERY) el("mint-query").value = window.MINTPAGE_DEFAULT_QUERY;
});
