(function () {
  'use strict';

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
    openApp: function (id) { console.log('openApp', id); },
    openInput: function (id) { console.log('openInput', id); },
    openSettings: function () { console.log('openSettings'); },
    openAmazon: function () { console.log('openAmazon'); },
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

  var state = {
    rows: [],
    row: 0,
    col: 0,
    hidden: store.get('hidden', []),
    showHidden: false,
    still: store.get('still', false)
  };

  var shelf = document.getElementById('shelf');
  var hint = document.getElementById('hint');

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
    return tv.filter(function (a) { return state.showHidden || !isHidden(a.id); }).map(function (a) {
      return {
        kind: 'app', id: a.id, label: a.label, art: onDevice ? '/art/' + a.id : null,
        open: function () { host.openApp(a.id); }
      };
    });
  }

  function actionTiles() {
    return [
      { kind: 'action', id: 'settings', label: 'Settings', open: function () { host.openSettings(); } },
      {
        kind: 'action', id: 'boil', label: state.still ? 'Boil: off' : 'Boil: on',
        open: function () { state.still = !state.still; store.set('still', state.still); applyStill(); build(); }
      },
      {
        kind: 'action', id: 'hidden', label: state.showHidden ? 'Hide hidden' : 'Show hidden',
        open: function () { state.showHidden = !state.showHidden; build(); }
      },
      { kind: 'action', id: 'amazon', label: "Amazon's home", open: function () { host.openAmazon(); } }
    ];
  }

  function applyStill() {
    document.body.classList.toggle('still', state.still);
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
      { title: 'System', tiles: actionTiles() }
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
    var t = current();
    if (!t) { hint.textContent = ''; return; }
    if (t.kind === 'app') hint.textContent = t.label + (isHidden(t.id) ? '  ·  hidden' : '') + '  ·  menu: ' + (isHidden(t.id) ? 'unhide' : 'hide');
    else if (t.kind === 'input') hint.textContent = t.label + (t.live ? '  ·  connected' : '');
    else hint.textContent = '';
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

  function toggleHidden() {
    var t = current();
    if (!t || t.kind !== 'app') return;
    var i = state.hidden.indexOf(t.id);
    if (i >= 0) state.hidden.splice(i, 1); else state.hidden.push(t.id);
    store.set('hidden', state.hidden);
    build();
  }

  function first() {
    state.row = 0;
    state.col = 0;
    focus();
  }

  document.addEventListener('keydown', function (e) {
    switch (e.key) {
      case 'ArrowUp': move(-1, 0); break;
      case 'ArrowDown': move(1, 0); break;
      case 'ArrowLeft': move(0, -1); break;
      case 'ArrowRight': move(0, 1); break;
      case 'Enter': case ' ':
        var t = current();
        if (t) t.open();
        break;
      case 'ContextMenu': toggleHidden(); break;
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
    var h = d.getHours() % 12 || 12;
    document.getElementById('clock').textContent = h + ':' + pad(d.getMinutes());
    document.getElementById('date').textContent = days[d.getDay()] + ' ' + d.getDate() + ' ' + months[d.getMonth()];
    clearTimeout(tickTimer);
    tickTimer = setTimeout(tick, 60000 - d.getSeconds() * 1000 - d.getMilliseconds() + 50);
  }

  window.aguatv = {
    refresh: build,
    resume: function () { tick(); build(); },
    pause: function () { clearTimeout(tickTimer); },
    home: first,
    back: first,
    menu: toggleHidden
  };

  applyStill();
  tick();
  build();
  requestAnimationFrame(function () {
    requestAnimationFrame(function () { host.ready(); });
  });
})();
