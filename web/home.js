(function () {
  'use strict';

  var now = Date.now();
  var preview = {
    apps: function () {
      return JSON.stringify([
        { id: 'org.example.films', label: 'Films', leanback: true },
        { id: 'org.example.music', label: 'Music', leanback: true },
        { id: 'org.example.tool', label: 'A phone app', leanback: false }
      ]);
    },
    inputs: function () {
      return JSON.stringify([
        { id: 'hdmi1', label: 'HDMI 1', hdmi: true, connected: true },
        { id: 'hdmi2', label: 'HDMI 2', hdmi: true, connected: false },
        { id: 'hdmi3', label: 'HDMI 3', hdmi: true, connected: false }
      ]);
    },
    details: function (kind, id) {
      if (kind === 'input') {
        return JSON.stringify({ kind: 'input', id: id, label: id.toUpperCase(), type: 'HDMI', state: 'connected', service: 'org.example.tis' });
      }
      return JSON.stringify({ kind: 'app', id: id, label: id.split('.').pop(), version: '1.2.3', versionCode: 123,
        installed: now - 864e5 * 40, updated: now - 864e5 * 3, system: false, targetSdk: 28, tv: true });
    },
    openApp: function (id) { console.log('openApp', id); },
    openInput: function (id) { console.log('openInput', id); },
    openAppInfo: function (id) { console.log('openAppInfo', id); },
    openSettings: function () { console.log('openSettings'); },
    ready: function () {}
  };
  var host = window.host || preview;
  var onDevice = !!window.host;

  var store = {
    get: function (k, d) {
      try {
        var v = localStorage.getItem('aguatv.' + k);
        return v === null ? d : JSON.parse(v);
      } catch (e) { return d; }
    },
    set: function (k, v) {
      try { localStorage.setItem('aguatv.' + k, JSON.stringify(v)); } catch (e) {}
    }
  };

  var prefs = {
    still: store.get('still', false),
    scanlines: store.get('scanlines', true),
    clock24: store.get('clock24', false),
    showHidden: store.get('showHidden', false)
  };

  var state = {
    rows: [],
    row: 0,
    col: 0,
    hidden: store.get('hidden', []),
    panel: null
  };

  var stage = document.querySelector('.stage');
  var shelf = document.getElementById('shelf');
  var hint = document.getElementById('hint');
  var drawer = document.getElementById('drawer');

  function parse(s) {
    try { return JSON.parse(s) || []; } catch (e) { return []; }
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function isHidden(id) { return state.hidden.indexOf(id) >= 0; }

  function inputTiles() {
    return parse(host.inputs()).map(function (i) {
      return {
        kind: 'input', id: i.id, label: i.label, live: i.connected,
        open: function () { host.openInput(i.id); }
      };
    });
  }

  function appTiles() {
    var apps = parse(host.apps());
    var tv = apps.filter(function (a) { return a.leanback; });
    if (!tv.length) tv = apps;
    return tv.filter(function (a) { return prefs.showHidden || !isHidden(a.id); }).map(function (a) {
      return {
        kind: 'app', id: a.id, label: a.label, art: onDevice ? '/art/' + a.id : null,
        open: function () { host.openApp(a.id); }
      };
    });
  }

  function actionTiles() {
    return [
      {
        kind: 'action', id: 'settings', label: 'aguatv',
        about: 'How this home screen looks and behaves.',
        open: function () { openSettings(); }
      },
      {
        kind: 'action', id: 'tv-settings', label: 'TV settings',
        about: "The television's own settings: picture, sound, network, inputs.",
        open: function () { host.openSettings(); }
      }
    ];
  }

  function applyPrefs() {
    document.body.classList.toggle('still', prefs.still);
    stage.classList.toggle('phosphor-host', prefs.scanlines);
    stage.classList.toggle('phosphor-soft', prefs.scanlines);
  }

  function tileNode(t) {
    var n = el('div', 'tile' + (t.kind === 'action' ? ' action' : ''));
    if (t.kind === 'app' && isHidden(t.id)) n.classList.add('hidden-app');
    n.appendChild(el('span', 'label', t.label));
    if (t.art) {
      var img = el('img', 'art');
      img.alt = '';
      img.onload = function () { n.classList.add('has-art'); };
      img.onerror = function () { img.remove(); };
      img.src = t.art;
      n.appendChild(img);
    }
    if (t.kind === 'input') n.appendChild(el('span', 'dot' + (t.live ? ' live' : '')));
    n.appendChild(el('span', 'ring boil-ui'));
    return n;
  }

  function build() {
    var keep = state.rows[state.row] && state.rows[state.row].tiles[state.col];
    var keepId = keep ? keep.kind + ':' + keep.id : null;
    var rows = [
      { title: 'Inputs', tiles: inputTiles() },
      { title: 'Apps', tiles: appTiles() },
      { title: 'Settings', tiles: actionTiles() }
    ].filter(function (r) { return r.tiles.length; });

    shelf.textContent = '';
    rows.forEach(function (r) {
      var sec = el('div', 'row');
      sec.appendChild(el('div', 'row-title', r.title));
      var strip = el('div', 'row-tiles');
      r.nodes = r.tiles.map(function (t) {
        var n = tileNode(t);
        strip.appendChild(n);
        return n;
      });
      r.strip = strip;
      sec.appendChild(strip);
      shelf.appendChild(sec);
    });
    state.rows = rows;

    state.row = Math.min(state.row, rows.length - 1);
    state.col = 0;
    if (keepId) {
      rows.forEach(function (r, ri) {
        r.tiles.forEach(function (t, ci) {
          if (t.kind + ':' + t.id === keepId) { state.row = ri; state.col = ci; }
        });
      });
    }
    focus();
  }

  function current() {
    var r = state.rows[state.row];
    return r ? r.tiles[state.col] : null;
  }

  function focus() {
    state.rows.forEach(function (r, ri) {
      r.nodes.forEach(function (n, ci) {
        n.classList.toggle('focus', ri === state.row && ci === state.col);
      });
    });
    var r = state.rows[state.row];
    if (r) {
      var n = r.nodes[state.col];
      var over = n.offsetLeft + n.offsetWidth - r.strip.parentNode.clientWidth;
      r.strip.style.transform = over > 0 ? 'translateX(' + (-over - 16) + 'px)' : '';
    }
    if (state.panel) return;
    var t = current();
    var parts = [];
    if (t) {
      parts.push(t.label);
      if (t.kind === 'app' && isHidden(t.id)) parts.push('hidden');
      if (t.kind === 'input' && t.live) parts.push('connected');
    }
    parts.push('menu: details');
    hint.textContent = parts.join('  ·  ');
  }

  function move(dr, dc) {
    if (!state.rows.length) return;
    if (dr) {
      var next = state.row + dr;
      if (next < 0 || next >= state.rows.length) return;
      state.row = next;
      state.col = Math.min(state.col, state.rows[next].tiles.length - 1);
    } else {
      var len = state.rows[state.row].tiles.length;
      state.col = Math.max(0, Math.min(len - 1, state.col + dc));
    }
    focus();
  }

  function toggleHidden(id) {
    var i = state.hidden.indexOf(id);
    if (i >= 0) state.hidden.splice(i, 1); else state.hidden.push(id);
    store.set('hidden', state.hidden);
  }

  function first() {
    state.row = 0;
    state.col = 0;
    focus();
  }

  function date(ms) {
    if (!ms) return '';
    var d = new Date(ms);
    return d.getDate() + ' ' + months[d.getMonth()].slice(0, 3) + ' ' + d.getFullYear();
  }

  function detailsFor(t) {
    if (t.kind === 'action') {
      return { title: t.label, rows: [['', t.about]], actions: [{ label: 'Open', run: t.open }] };
    }
    var d = parse(host.details(t.kind, t.id));
    if (t.kind === 'input') {
      var rows = [['Type', d.type], ['State', d.state]];
      rows.push(['Service', d.service], ['Input', d.id]);
      return { title: d.label || t.label, rows: rows, actions: [{ label: 'Switch to it', run: t.open }] };
    }
    var hidden = isHidden(t.id);
    return {
      title: d.label || t.label,
      rows: [
        ['Package', d.id],
        ['Version', d.version + (d.versionCode ? ' (' + d.versionCode + ')' : '')],
        ['Installed', date(d.installed)],
        ['Updated', date(d.updated)],
        ['Kind', (d.system ? 'system' : 'sideloaded') + (d.tv ? ', TV app' : ', phone app')],
        ['Target SDK', String(d.targetSdk || '')]
      ],
      actions: [
        { label: 'Open', run: t.open },
        { label: hidden ? 'Unhide' : 'Hide', run: function () { toggleHidden(t.id); closePanel(); build(); } },
        { label: 'App info', run: function () { host.openAppInfo(t.id); } }
      ]
    };
  }

  function settingsPanel() {
    function flip(key, on, off) {
      return {
        label: (prefs[key] ? on : off),
        run: function () { prefs[key] = !prefs[key]; store.set(key, prefs[key]); applyPrefs(); tick(); build(); openSettings(true); }
      };
    }
    return {
      title: 'aguatv',
      rows: [['', 'Saved on this TV. Hidden apps: ' + state.hidden.length + '.']],
      actions: [
        flip('still', 'Boil: off', 'Boil: on'),
        flip('scanlines', 'Scanlines: on', 'Scanlines: off'),
        flip('clock24', 'Clock: 24 hour', 'Clock: 12 hour'),
        flip('showHidden', 'Hidden apps: shown', 'Hidden apps: kept hidden'),
        { label: 'TV settings', run: function () { closePanel(); host.openSettings(); } }
      ]
    };
  }

  function renderPanel() {
    var p = state.panel;
    drawer.textContent = '';
    drawer.classList.toggle('open', !!p);
    if (!p) return;
    drawer.appendChild(el('div', 'drawer-kind fontpack-mono', p.kind === 'settings' ? 'settings' : 'details'));
    drawer.appendChild(el('h2', 'drawer-title fontpack-display boil-text', p.title));
    var dl = el('dl', 'facts');
    p.rows.forEach(function (r) {
      if (r[0]) dl.appendChild(el('dt', 'fontpack-mono', r[0]));
      dl.appendChild(el('dd', r[0] ? '' : 'wide', r[1] || '—'));
    });
    drawer.appendChild(dl);
    var list = el('div', 'drawer-actions');
    p.actions.forEach(function (a, i) {
      var b = el('div', 'drawer-action fontpack-mono' + (i === p.sel ? ' focus' : ''), a.label);
      list.appendChild(b);
    });
    drawer.appendChild(list);
    hint.textContent = 'up/down: choose  ·  ok: do it  ·  back: close';
  }

  function openPanel(kind, p, keepSel) {
    var sel = keepSel && state.panel && state.panel.kind === kind ? state.panel.sel : 0;
    p.kind = kind;
    p.sel = Math.min(sel, p.actions.length - 1);
    state.panel = p;
    renderPanel();
  }

  function closePanel() {
    state.panel = null;
    renderPanel();
    focus();
  }

  function openDetails() {
    if (state.panel && state.panel.kind === 'details') { closePanel(); return; }
    var t = current();
    if (t) openPanel('details', detailsFor(t));
  }

  function openSettings(keepSel) {
    if (state.panel && state.panel.kind === 'settings' && !keepSel) { closePanel(); return; }
    openPanel('settings', settingsPanel(), keepSel);
  }

  function panelKey(key) {
    var p = state.panel;
    switch (key) {
      case 'ArrowUp': p.sel = Math.max(0, p.sel - 1); renderPanel(); return true;
      case 'ArrowDown': p.sel = Math.min(p.actions.length - 1, p.sel + 1); renderPanel(); return true;
      case 'Enter': case ' ': p.actions[p.sel].run(); return true;
      case 'ArrowLeft': case 'Escape': closePanel(); return true;
    }
    return false;
  }

  document.addEventListener('keydown', function (e) {
    if (state.panel) {
      if (panelKey(e.key)) e.preventDefault();
      return;
    }
    switch (e.key) {
      case 'ArrowUp': move(-1, 0); break;
      case 'ArrowDown': move(1, 0); break;
      case 'ArrowLeft': move(0, -1); break;
      case 'ArrowRight': move(0, 1); break;
      case 'Enter': case ' ':
        var t = current();
        if (t) t.open();
        break;
      case 'i': case 'ContextMenu': openDetails(); break;
      case 's': openSettings(); break;
      case 'Escape': first(); break;
      default: return;
    }
    e.preventDefault();
  });

  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  var days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
  var months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August',
    'September', 'October', 'November', 'December'];

  var tickTimer = null;
  function tick() {
    var d = new Date();
    var h = prefs.clock24 ? pad(d.getHours()) : (d.getHours() % 12 || 12);
    document.getElementById('clock').textContent = h + ':' + pad(d.getMinutes());
    document.getElementById('date').textContent = days[d.getDay()] + ' ' + d.getDate() + ' ' + months[d.getMonth()];
    clearTimeout(tickTimer);
    tickTimer = setTimeout(tick, 60000 - d.getSeconds() * 1000 - d.getMilliseconds() + 50);
  }

  window.aguatv = {
    refresh: function () { build(); if (state.panel && state.panel.kind === 'settings') openSettings(true); },
    resume: function () { tick(); build(); },
    pause: function () { clearTimeout(tickTimer); },
    home: function () { closePanel(); first(); },
    back: function () { if (state.panel) closePanel(); else first(); },
    details: openDetails,
    settings: function () { openSettings(); }
  };

  applyPrefs();
  tick();
  build();
  requestAnimationFrame(function () {
    requestAnimationFrame(function () { host.ready(); });
  });
})();
