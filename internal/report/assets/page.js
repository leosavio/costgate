// costgate HTML report. Renders the JSON report of the run (data.report) with the display values
// costgate computed with exact decimals (data.view): this script formats no amount and does no
// arithmetic on money. Strings from the plan reach the page only through textContent, never
// innerHTML. No network: the CSV and JSON Lines exports are Blob downloads.
(function () {
  "use strict";

  var body = document.body;
  function fail(msg) { body.setAttribute("data-status", "error: " + msg); }
  window.addEventListener("error", function (e) { fail(e.message + " (line " + e.lineno + ")"); });
  document.addEventListener("securitypolicyviolation", function (e) {
    fail("csp " + e.effectiveDirective + " " + (e.blockedURI || "inline"));
  });

  var ACTIONS = ["create", "update", "replace", "delete", "forget", "no-op"];
  var SHOWS = [["all", "all"], ["changed", "changed"], ["blocking", "blocks the gate"],
    ["unpriced", "unpriced"], ["premise", "premise missing"]];
  var METRICS = [["after", "monthly after"], ["delta", "delta"]];
  var KEYS = ["q", "action", "show", "why", "module", "type", "metric", "open"];
  var DEFAULTS = { q: "", action: "", show: "all", why: "", module: "", type: "", metric: "after", open: "" };
  var LIMIT = 8; // entries listed per group in the side panels; the rest is one click away in the table

  var R, V, rows, st, why = {};

  // ---- helpers
  function $(id) { return document.getElementById(id); }
  function el(tag, attrs, text) {
    var e = document.createElement(tag);
    if (attrs) Object.keys(attrs).forEach(function (k) {
      if (k === "class") e.className = attrs[k]; else e.setAttribute(k, attrs[k]);
    });
    if (text !== undefined && text !== null) e.textContent = String(text);
    return e;
  }
  function add(parent, tag, attrs, text) { var e = el(tag, attrs, text); parent.appendChild(e); return e; }
  function text(parent, s) { parent.appendChild(document.createTextNode(s)); }
  function plural(n, one, many) { return n + " " + (n === 1 ? one : many); }
  function signClass(s) { return s.charAt(0) === "+" ? "up" : s.charAt(0) === "-" ? "down" : ""; }
  function shownSide(r) { return r.after || r.before; }
  function viewSide(i, side) {
    return V.resources[i].sides.filter(function (s) { return s.side === side; })[0];
  }
  function cardHead(box, title, note) {
    var h = add(box, "div", { class: "card-head" });
    add(h, "h2", null, title);
    if (note) add(h, "span", { class: "muted small" }, note);
  }
  function kv(dl, k, v, cls) {
    add(dl, "dt", null, k);
    add(dl, "dd", cls ? { class: cls } : null, v);
  }

  // ---- state in the URL hash, so a filtered view can be shared or reloaded
  function readHash() {
    var s = {};
    KEYS.forEach(function (k) { s[k] = DEFAULTS[k]; });
    location.hash.replace(/^#/, "").split("&").forEach(function (kv) {
      if (!kv) return;
      var i = kv.indexOf("="), k = i < 0 ? kv : kv.slice(0, i), v = i < 0 ? "" : kv.slice(i + 1);
      try { v = decodeURIComponent(v); } catch (e) { return; }
      if (KEYS.indexOf(k) >= 0) s[k] = v;
    });
    if (ACTIONS.indexOf(s.action) < 0) s.action = "";
    if (!SHOWS.some(function (x) { return x[0] === s.show; })) s.show = "all";
    if (s.metric !== "delta") s.metric = "after";
    if (!V.unpriced.some(function (g) { return g.category === s.why; })) s.why = "";
    return s;
  }
  function hashOf(s) {
    return KEYS.filter(function (k) { return s[k] !== DEFAULTS[k]; })
      .map(function (k) { return k + "=" + encodeURIComponent(s[k]); }).join("&");
  }
  function writeHash() {
    var h = hashOf(st);
    if (location.hash.replace(/^#/, "") !== h) location.replace("#" + h);
  }

  // ---- rows: one per resource of the report, in report order
  function buildRows() {
    var missing = {};
    V.unpriced.forEach(function (g) {
      g.resources.forEach(function (i) { why[i] = g.category; });
    });
    V.premises.forEach(function (p) {
      if (!R.resources[p.resource][p.side].lines[p.line].declared) missing[p.resource] = true;
    });
    return R.resources.map(function (r, i) {
      var words = [r.address, r.type];
      ["before", "after"].forEach(function (s) {
        if (!r[s]) return;
        words.push(r[s].unpriced);
        r[s].lines.forEach(function (l) {
          words.push(l.name, l.price_key, l.sku, l.usage_type, l.rate_code, l.billing_description);
        });
      });
      return {
        i: i, r: r, v: V.resources[i], missing: !!missing[i],
        reason: (r.after && r.after.unpriced) || (r.before && r.before.unpriced) || "",
        text: words.filter(Boolean).join("\n").toLowerCase()
      };
    });
  }

  // match applies the filters; skip leaves one out, to count the choices of that filter.
  function match(row, skip) {
    var r = row.r;
    if (st.module && row.v.module !== st.module) return false;
    if (st.type && r.type !== st.type) return false;
    if (st.why && why[row.i] !== st.why) return false;
    if (skip !== "action" && st.action && r.action !== st.action) return false;
    if (skip !== "show") {
      if (st.show === "changed" && r.action === "no-op") return false;
      if (st.show === "blocking" && !r.blocking) return false;
      if (st.show === "unpriced" && row.v.kind !== "unpriced") return false;
      if (st.show === "premise" && !row.missing) return false;
    }
    if (st.q && row.text.indexOf(st.q.toLowerCase()) < 0) return false;
    return true;
  }
  function filtered() { return rows.filter(function (row) { return match(row); }); }

  function focus(row) {
    KEYS.forEach(function (k) { st[k] = DEFAULTS[k]; });
    st.q = row.r.address;
    st.open = row.r.address;
    render();
    $("resources").scrollIntoView();
  }
  function resourceLink(parent, row) {
    var b = add(parent, "button", { type: "button", class: "link", title: "show in the resource table" }, row.r.address);
    b.addEventListener("click", function () { focus(row); });
  }
  // showAll adds a button that resets the filters, applies set and scrolls to the table.
  function showAll(parent, label, set) {
    var b = add(parent, "button", { type: "button", class: "link more" }, label);
    b.addEventListener("click", function () {
      KEYS.forEach(function (k) { st[k] = DEFAULTS[k]; });
      set(st);
      render();
      $("resources").scrollIntoView();
    });
  }

  // ---- (1) header: gate, totals, snapshot
  function header() {
    $("version").textContent = R.version;
    document.title = "costgate · " + V.status_text.headline + " · " + V.status_text.title;
    var g = $("gate");
    add(g, "span", { class: "badge st-" + R.status }, V.status_text.title + " · exit " + R.exit_code);
    add(g, "span", { class: "headline " + signClass(V.totals.delta) }, V.status_text.headline);
    var t = $("totals");
    [["before", V.totals.before], ["after", V.totals.after], ["delta", V.totals.delta]].forEach(function (x) {
      var d = add(t, "div", { class: "total" });
      add(d, "span", { class: "lbl" }, x[0] + " · USD/month");
      add(d, "span", { class: "val " + (x[0] === "delta" ? signClass(x[1]) : "") }, x[1]);
    });
    var s = $("snapshot");
    add(s, "div", null, "AWS Price List snapshot " + V.region.snapshot + " · as of " + R.pricing.as_of);
    add(s, "div", { class: "mono", title: R.pricing.sha256 }, "sha256 " + V.sha256_short + " · priced region " + R.region);
  }

  function checksCard() {
    var box = $("checks");
    cardHead(box, "Gate", "exit " + R.exit_code + (R.incomplete ? " · estimate incomplete" : ""));
    var t = add(box, "table", { class: "checks" }), h = add(t, "tr");
    ["Check", "Limit", "Result"].forEach(function (x) { add(h, "th", null, x); });
    V.checks.forEach(function (c) {
      var tr = add(t, "tr");
      add(add(tr, "td"), c.flag ? "code" : "span", null, c.name);
      add(tr, "td", { class: "num" }, c.limit);
      add(tr, "td", { class: c.ok ? "pass" : "failc" }, (c.ok ? "✓ " : "✗ ") + c.value);
    });
    if (R.reasons.length) {
      add(box, "p", { class: "small" }, "Exit " + R.exit_code + ":");
      var ul = add(box, "ul", { class: "reasons small" });
      R.reasons.forEach(function (x) { add(ul, "li", null, x); });
    }
  }

  // ---- (2) bars by module and by type
  function bars(box, groups, key) {
    box.textContent = "";
    var list = groups.map(function (g, i) { return { g: g, i: i }; });
    if (st.metric === "delta") list.sort(function (a, b) { return b.g.bar_delta - a.g.bar_delta || a.i - b.i; });
    list.forEach(function (x) {
      var g = x.g, on = st[key] === g.key, delta = st.metric === "delta";
      var b = add(box, "button", { type: "button", class: "bar" + (on ? " on" : ""), title: key + " " + g.key });
      add(b, "span", { class: "bar-label" }, g.key);
      var fill = add(add(b, "span", { class: "bar-track" }), "span", { class: "bar-fill " + (delta ? signClass(g.delta) : "") });
      fill.style.width = (delta ? g.bar_delta : g.bar_after) / 10 + "%";
      add(b, "span", { class: "bar-value num " + (delta ? signClass(g.delta) : "") }, delta ? g.delta : g.after);
      var meta = add(b, "span", { class: "bar-meta" },
        plural(g.count, "resource", "resources") + " · " + g.changed + " changed · before " + g.before + " · delta " + g.delta);
      if (g.unpriced) { text(meta, " · "); add(meta, "span", { class: "warn" }, g.unpriced + " unpriced"); }
      b.addEventListener("click", function () { st[key] = on ? "" : g.key; st.open = ""; render(); });
    });
  }

  // ---- (3) resource table
  function summary(td, row) {
    var r = row.r;
    if (row.v.kind === "unpriced") {
      add(td, "span", { class: "flag" }, "unpriced: " + row.reason);
      if (r.blocking) { text(td, " · "); add(td, "span", { class: "block" }, "blocks the gate"); }
      return;
    }
    if (row.v.kind === "no_charge") { add(td, "span", { class: "muted" }, "no charge"); return; }
    text(td, shownSide(r).lines.map(function (l) { return l.name; }).join(" · "));
    if (row.missing) { text(td, " · "); add(td, "span", { class: "flag" }, "premise missing"); }
  }

  function detail(tb, row) {
    var td = add(add(tb, "tr", { class: "detail" }), "td", { colspan: "6" });
    row.v.sides.forEach(function (s) {
      var side = row.r[s.side];
      add(td, "div", { class: "side-head" }, s.side + (side.unpriced ? " · unpriced" : " · US$ " + s.monthly + "/month"));
      if (side.unpriced) { add(td, "div", { class: "flag small" }, side.unpriced); return; }
      if (!side.lines.length) { add(td, "div", { class: "muted small" }, "no charge"); return; }
      var t = add(td, "table", { class: "lines" }), h = add(t, "tr");
      ["Line", "Formula", "Monthly", "Billing rule", "SKU", "Usage type", "Rate code", "Invoice description"]
        .forEach(function (x) { add(h, "th", null, x); });
      side.lines.forEach(function (l, j) {
        var tr = add(t, "tr"), hv = s.lines[j];
        add(tr, "td", null, l.name);
        add(tr, "td", l.premise && !l.declared ? { class: "flag" } : null, hv.formula);
        add(tr, "td", { class: "num" }, hv.monthly);
        add(tr, "td", null, l.billing_rule);
        add(tr, "td", { class: "mono" }, l.sku);
        add(tr, "td", { class: "mono" }, l.usage_type);
        add(tr, "td", { class: "mono" }, l.rate_code);
        add(tr, "td", null, l.billing_description);
      });
    });
  }

  function table() {
    var tb = $("rows"), shown = 0, lines = 0;
    tb.textContent = "";
    rows.forEach(function (row) {
      if (!match(row)) return;
      shown++;
      lines += row.v.export.length;
      var open = st.open === row.r.address;
      var tr = add(tb, "tr", { class: "res" + (open ? " open" : "") }), td = add(tr, "td");
      add(td, "span", { class: "addr" }, row.r.address);
      add(td, "span", { class: "type" }, row.r.type);
      add(add(tr, "td"), "span", { class: "act act-" + row.r.action }, row.r.action);
      add(tr, "td", { class: "num" }, row.v.before);
      add(tr, "td", { class: "num" }, row.v.after);
      add(tr, "td", { class: "num " + signClass(row.v.delta) }, row.v.delta);
      summary(add(tr, "td", { class: "small" }), row);
      tr.addEventListener("click", function () { st.open = open ? "" : row.r.address; render(); });
      if (open) detail(tb, row);
    });
    if (!shown) add(add(tb, "tr"), "td", { colspan: "6", class: "muted" }, "No resource matches the filters.");
    $("count").textContent = shown + " of " + plural(rows.length, "resource", "resources") + " · " +
      plural(lines, "cost line", "cost lines") + " to export";
    $("exportCSV").disabled = $("exportJSONL").disabled = lines === 0;
    body.setAttribute("data-rows", String(shown));
    body.setAttribute("data-lines", String(lines));
  }

  // ---- (4) unpriced, grouped by reason
  function unpricedCard() {
    var box = $("unpriced"), n = 0, blocking = 0;
    V.unpriced.forEach(function (g) { n += g.resources.length; blocking += g.blocking; });
    cardHead(box, "Unpriced resources", n ? plural(n, "resource", "resources") + " · " + blocking + " blocking" : "none");
    if (!n) { add(box, "p", { class: "muted" }, "Every resource has a price or no charge of its own."); return; }
    add(box, "p", { class: "muted small" }, "Fail-closed: an unpriced resource counts as 0 in the totals, and a created, " +
      "updated or replaced one blocks the gate (exit 2).");
    V.unpriced.forEach(function (g) {
      var d = add(box, "div", { class: "group" }), gh = add(d, "div", { class: "group-head" });
      add(gh, "h3", null, g.title);
      add(gh, "span", { class: "muted small" }, g.resources.length + (g.blocking ? " · " + g.blocking + " blocking" : ""));
      add(d, "p", { class: "muted small" }, g.hint);
      var ul = add(d, "ul", { class: "list" });
      g.resources.slice(0, LIMIT).forEach(function (i) {
        var row = rows[i], li = add(ul, "li");
        resourceLink(li, row);
        var m = add(li, "div", { class: "small" });
        add(m, "span", { class: "act act-" + row.r.action }, row.r.action);
        text(m, " " + row.reason + " · ");
        add(m, "span", { class: row.r.blocking ? "block" : "muted" },
          row.r.blocking ? "blocks the gate" : "does not block (unchanged or deleted)");
      });
      showAll(d, (g.resources.length > LIMIT ? "show all " + g.resources.length : "show") + " in the table",
        function (s) { s.why = g.category; });
    });
  }

  // ---- (5) usage premises, declared and missing
  function premisesCard() {
    var box = $("premises"), miss = [], decl = [];
    V.premises.forEach(function (p) {
      var x = { row: rows[p.resource], p: p, l: R.resources[p.resource][p.side].lines[p.line], v: viewSide(p.resource, p.side).lines[p.line] };
      (x.l.declared ? decl : miss).push(x);
    });
    cardHead(box, "Usage premises", V.premises.length ? miss.length + " missing · " + decl.length + " declared" : "none");
    if (!V.premises.length) { add(box, "p", { class: "muted" }, "No cost line depends on a usage premise."); return; }
    if (miss.length) {
      var d = add(box, "div", { class: "group" });
      add(d, "h3", null, "Not declared: counted as 0");
      add(d, "p", { class: "muted small" }, "The totals leave these lines out. Declare the premise in the usage file (--usage).");
      var ul = add(d, "ul", { class: "list" });
      miss.slice(0, LIMIT).forEach(function (x) {
        var li = add(ul, "li");
        resourceLink(li, x.row);
        var m = add(li, "div", { class: "small" });
        text(m, x.l.name + " · set ");
        add(m, "code", null, x.l.premise);
        add(li, "div", { class: "small flag" }, "effect: " + x.p.per_unit + " declared");
      });
      showAll(d, (miss.length > LIMIT ? "show all " + miss.length : "show") + " in the table",
        function (s) { s.show = "premise"; });
    }
    if (decl.length) {
      var d2 = add(box, "div", { class: "group" });
      add(d2, "h3", null, "Declared");
      var ul2 = add(d2, "ul", { class: "list" });
      if (decl.length > LIMIT) add(d2, "p", { class: "muted small" }, "and " + (decl.length - LIMIT) + " more; open a resource to see its lines");
      decl.slice(0, LIMIT).forEach(function (x) {
        var li = add(ul2, "li");
        resourceLink(li, x.row);
        var m = add(li, "div", { class: "small" });
        text(m, x.l.name + " · ");
        add(m, "code", null, x.l.premise);
        add(li, "div", { class: "small muted" }, x.v.formula + " → US$ " + x.v.monthly + "/month in the total");
      });
    }
  }

  // ---- (0) warnings: part of the estimate is probably not what the caller wants
  function warning(parent, w) {
    var d = add(parent, "div", { class: "warning" });
    add(d, "code", { class: "warn-code" }, w.code);
    text(d, " ");
    add(d, "span", null, w.message);
    var f = add(d, "div", { class: "small" });
    text(f, "fix: ");
    add(f, "code", null, w.fix);
  }
  function warningsCard() {
    var list = R.warnings || [];
    body.setAttribute("data-warnings", String(list.length));
    if (!list.length) return;
    var box = $("warnings");
    box.hidden = false;
    cardHead(box, plural(list.length, "warning", "warnings"), "also at the top of the Markdown report");
    list.forEach(function (w) { warning(box, w); });
  }

  // ---- (6) pricing region
  function regionCard() {
    var box = $("region");
    var ws = (R.warnings || []).filter(function (w) { return w.code === "region_mismatch"; });
    cardHead(box, "Pricing region", ws.length ? plural(ws.length, "warning", "warnings") : "");
    var dl = add(box, "dl", { class: "kv" });
    kv(dl, "priced region (--region)", V.region.priced, "mono");
    kv(dl, "snapshot region", V.region.snapshot, "mono");
    add(box, "p", { class: "muted small" }, "costgate takes the region of a resource from the plan: its region attribute, " +
      "the region of its provider configuration, or its ARN. A resource in another region stays unpriced; a resource " +
      "with no known region is priced with the rates and bill codes of this region.");
    ws.forEach(function (w) { warning(box, w); });
    if (!V.region.mismatch.length) {
      add(box, "p", { class: "muted" }, "No resource in the plan is in a region different from the priced one.");
      return;
    }
    var ul = add(box, "ul", { class: "list" });
    V.region.mismatch.slice(0, LIMIT).forEach(function (i) {
      var li = add(ul, "li");
      resourceLink(li, rows[i]);
      add(li, "div", { class: "small flag" }, rows[i].reason);
    });
    var n = V.region.mismatch.length;
    showAll(box, (n > LIMIT ? "show all " + n : "show") + " in the table", function (s) { s.why = "region"; });
  }

  // ---- (6b) resources of the providers left out with --ignore-provider
  function ignoredCard() {
    var list = R.ignored || [], providers = R.ignored_providers || [];
    body.setAttribute("data-ignored", String(list.length));
    if (!providers.length) return;
    var box = $("ignored");
    box.hidden = false;
    cardHead(box, "Ignored providers", plural(list.length, "resource", "resources"));
    var p = add(box, "p", { class: "muted small" });
    text(p, "Left out of the estimate, of the totals and of the gate, and not in the resource table: ");
    providers.forEach(function (x, i) {
      if (i) text(p, ", ");
      add(p, "code", null, x);
    });
    text(p, " (--ignore-provider).");
    if (!list.length) { add(box, "p", { class: "muted" }, "No resource of the plan matched these providers."); return; }
    var ul = add(box, "ul", { class: "list" });
    list.slice(0, LIMIT).forEach(function (x) {
      var li = add(ul, "li");
      add(li, "span", { class: "mono small" }, x.address);
      var m = add(li, "div", { class: "small" });
      add(m, "span", { class: "act act-" + x.action }, x.action);
      text(m, " " + x.type + " · ");
      add(m, "span", { class: "muted" }, x.provider);
    });
    if (list.length > LIMIT) add(box, "p", { class: "muted small" }, "and " + (list.length - LIMIT) + " more in the JSON report (ignored)");
  }

  function sourcesCard() {
    var box = $("sources");
    cardHead(box, "Price snapshot", plural(R.pricing.sources.length, "offer file", "offer files"));
    var dl = add(box, "dl", { class: "kv" });
    kv(dl, "region", V.region.snapshot, "mono");
    kv(dl, "as of", R.pricing.as_of, "mono");
    kv(dl, "sha256", R.pricing.sha256, "mono");
    kv(dl, "costgate", R.version, "mono");
    var t = add(box, "table", { class: "lines sources" }), h = add(t, "tr");
    ["Offer file", "Scope", "Published"].forEach(function (x) { add(h, "th", null, x); });
    R.pricing.sources.forEach(function (s) {
      var tr = add(t, "tr", { title: s.path });
      add(tr, "td", { class: "mono" }, s.service + " " + s.version);
      add(tr, "td", null, s.scope);
      add(tr, "td", { class: "mono" }, s.publication_date);
    });
  }

  // ---- (7) export of the filtered cost lines
  function csvCell(s) {
    s = String(s);
    // A spreadsheet runs a cell that starts with = + - @ as a formula; numbers stay as they are.
    if (/^[=+\-@\t\r]/.test(s) && !/^[+-]?\d+(\.\d+)?$/.test(s)) s = "'" + s;
    return /[",\r\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
  }
  function exportCSV() {
    var out = [V.csv_header.map(csvCell).join(",")];
    filtered().forEach(function (row) {
      row.v.export.forEach(function (x) { out.push(x.csv.map(csvCell).join(",")); });
    });
    return out.join("\r\n") + "\r\n";
  }
  function exportJSONL() {
    var out = [];
    filtered().forEach(function (row) {
      row.v.export.forEach(function (x) { out.push(x.jsonl + "\n"); });
    });
    return out.join("");
  }
  function download(name, type, content) {
    var url = URL.createObjectURL(new Blob([content], { type: type }));
    var a = add(body, "a", { href: url, download: name });
    a.click();
    body.removeChild(a);
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
  }

  // ---- controls
  function seg(box, items, key) {
    items.forEach(function (x) {
      var b = add(box, "button", { type: "button", "data-v": x[0] }, x[1]);
      b.addEventListener("click", function () { st[key] = x[0]; render(); });
    });
  }
  function controls() {
    var q = $("q");
    q.addEventListener("input", function () { st.q = q.value; st.open = ""; render(); });
    $("action").addEventListener("change", function (e) { st.action = e.target.value; st.open = ""; render(); });
    seg($("metric"), METRICS, "metric");
    seg($("show"), SHOWS, "show");
    $("exportCSV").addEventListener("click", function () { download("costgate-lines.csv", "text/csv;charset=utf-8", exportCSV()); });
    $("exportJSONL").addEventListener("click", function () { download("costgate-lines.jsonl", "application/jsonl", exportJSONL()); });
    window.addEventListener("hashchange", function () {
      var s = readHash();
      if (hashOf(s) === hashOf(st)) return;
      st = s;
      render();
    });
  }
  function syncControls() {
    if ($("q").value !== st.q) $("q").value = st.q;
    var sel = $("action"), counts = {}, all = 0;
    rows.forEach(function (row) {
      if (!match(row, "action")) return;
      all++;
      counts[row.r.action] = (counts[row.r.action] || 0) + 1;
    });
    sel.textContent = "";
    add(sel, "option", { value: "" }, "all actions (" + all + ")");
    ACTIONS.forEach(function (a) {
      if (counts[a] || st.action === a) add(sel, "option", { value: a }, a + " (" + (counts[a] || 0) + ")");
    });
    sel.value = st.action;
    var shows = $("show").children;
    for (var i = 0; i < shows.length; i++) {
      var v = shows[i].getAttribute("data-v");
      var n = rows.filter(function (row) { return match(row, "show") && (v === "all" ||
        (v === "changed" && row.r.action !== "no-op") || (v === "blocking" && row.r.blocking) ||
        (v === "unpriced" && row.v.kind === "unpriced") || (v === "premise" && row.missing)); }).length;
      shows[i].textContent = SHOWS[i][1] + " (" + n + ")";
      shows[i].className = st.show === v ? "on" : "";
    }
    var metrics = $("metric").children;
    for (var j = 0; j < metrics.length; j++) metrics[j].className = st.metric === metrics[j].getAttribute("data-v") ? "on" : "";
    var pills = $("pills");
    pills.textContent = "";
    var reason = V.unpriced.filter(function (g) { return g.category === st.why; })[0];
    [["module", st.module], ["type", st.type], ["why", reason ? reason.title : ""]].forEach(function (p) {
      if (!p[1]) return;
      var s = add(pills, "span", { class: "pill" });
      text(s, (p[0] === "why" ? "unpriced" : p[0]) + ": ");
      add(s, p[0] === "why" ? "span" : "code", null, p[1]);
      var x = add(s, "button", { type: "button", title: "remove this filter" }, "×");
      x.addEventListener("click", function () { st[p[0]] = ""; render(); });
    });
  }

  function render() {
    syncControls();
    bars($("byModule"), V.modules, "module");
    bars($("byType"), V.types, "type");
    table();
    writeHash();
  }

  function start(data) {
    R = data.report;
    V = data.view;
    rows = buildRows();
    st = readHash();
    body.setAttribute("data-gate", R.status);
    body.setAttribute("data-exit", String(R.exit_code));
    body.setAttribute("data-total", String(rows.length));
    header();
    warningsCard();
    checksCard();
    unpricedCard();
    premisesCard();
    regionCard();
    ignoredCard();
    sourcesCard();
    var ign = R.ignored_providers && R.ignored_providers.length ? " · --ignore-provider " + R.ignored_providers.join(",") +
      ": " + plural((R.ignored || []).length, "resource", "resources") + " left out of the gate" : "";
    $("foot").textContent = "costgate " + R.version + " · AWS Price List Bulk API snapshot " + V.region.snapshot +
      " as of " + R.pricing.as_of + " (sha256 " + V.sha256_short + ")" + ign + " · " + V.footer +
      " This page makes no network request.";
    controls();
    render();
  }

  document.addEventListener("DOMContentLoaded", function () {
    try {
      start(JSON.parse($("costgate-data").textContent));
    } catch (err) {
      fail(String((err && err.message) || err));
      return;
    }
    if (body.getAttribute("data-status") === "loading") body.setAttribute("data-status", "ok");
  });
})();
