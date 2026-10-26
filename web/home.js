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
  var bare = {
    apps: function () { return '[]'; },
    inputs: function () { return '[]'; },
    details: function () { return '{}'; },
    openApp: function () {}, openInput: function () {}, openAppInfo: function () {}, openSettings: function () {},
    ready: function () {}
  };
  var onDevice = !!window.host;
  var previewing = /[?&]preview\b/.test(location.search);
  var host = window.host || (previewing ? preview : bare);
  var online = !onDevice && !previewing;
  var cast = window.aguatvCast;

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
    panel: null,
    me: null,
    screens: [],
    remotes: []
  };

  var stage = document.querySelector('.stage');
  var shelf = document.getElementById('shelf');
  var hint = document.getElementById('hint');
  var drawer = document.getElementById('drawer');
  var here = document.getElementById('here');
  var padSheet = document.getElementById('pad');
  var padStatus = document.getElementById('padStatus');
  var padFor = null;

  function api(method, path, body) {
    var opts = { method: method, credentials: 'same-origin', headers: {} };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    return fetch(path, opts).then(function (r) {
      if (r.status === 204) return {};
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.error || r.status + ' ' + r.statusText);
        return j;
      });
    });
  }

  function say(text) { hint.textContent = text; }

  function clockOf(t) {
    t = Math.max(0, Math.round(t || 0));
    var h = Math.floor(t / 3600), m = Math.floor(t / 60) % 60, s = t % 60;
    return (h ? h + ':' + (m < 10 ? '0' : '') : '') + m + ':' + (s < 10 ? '0' : '') + s;
  }

  function ago(iso) {
    var s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 90) return 'just now';
    if (s < 5400) return Math.round(s / 60) + ' min ago';
    if (s < 129600) return Math.round(s / 3600) + ' h ago';
    return Math.round(s / 86400) + ' days ago';
  }

  function remote() { return !!(state.me && state.me.screen && state.me.screen.remote); }

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
    var tiles = [
      {
        kind: 'action', id: 'settings', label: 'aguatv',
        about: 'How this home screen looks and behaves.',
        open: function () { openSettings(); }
      }
    ];
    if (onDevice) {
      tiles.push({
        kind: 'action', id: 'tv-settings', label: 'TV settings',
        about: "The television's own settings: picture, sound, network, inputs.",
        open: function () { host.openSettings(); }
      });
    }
    return tiles;
  }

  function screenTiles() {
    if (!online || !state.me) return [];
    var tiles = [];
    if (!state.me.screen) {
      tiles.push({ kind: 'this', id: 'this', label: 'This browser', open: function () { openPanel('details', registerPanel()); } });
    }
    state.screens.forEach(function (s) {
      tiles.push({ kind: 'screen', id: s.id, label: s.name, live: !!(s.state && s.state.playing), screen: s, open: function () { openDetails(); } });
    });
    state.remotes.forEach(function (r) {
      tiles.push({ kind: 'adb', id: r.name, label: r.tv || r.name, live: !!r.connected, adb: r, open: function () { openPad(r); } });
    });
    return tiles;
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
    if (t.kind === 'input' || t.kind === 'screen' || t.kind === 'adb') n.appendChild(el('span', 'dot' + (t.live ? ' live' : '')));
    if (t.screen && t.screen.here) n.appendChild(el('span', 'tag fontpack-mono', 'here'));
    n.appendChild(el('span', 'ring boil-ui'));
    return n;
  }

  function build() {
    var keep = state.rows[state.row] && state.rows[state.row].tiles[state.col];
    var keepId = keep ? keep.kind + ':' + keep.id : null;
    var rows = [
      { title: 'Screens', tiles: screenTiles() },
      { title: 'Inputs', tiles: inputTiles() },
      { title: 'Apps', tiles: appTiles() },
      { title: 'Settings', tiles: actionTiles() }
    ].filter(function (r) { return r.tiles.length; });

    shelf.textContent = '';
    rows.forEach(function (r) {
      var sec = el('div', 'row');
      sec.appendChild(el('div', 'row-title', r.title));
      var strip = el('div', 'row-tiles');
      var ri = rows.indexOf(r);
      r.nodes = r.tiles.map(function (t, ci) {
        var n = tileNode(t);
        n.addEventListener('click', function () {
          state.row = ri;
          state.col = ci;
          focus();
          t.open();
        });
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
      if (t.screen && t.screen.state && t.screen.state.title) parts.push(playing(t.screen.state));
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

  function playing(st) {
    return (st.playing ? 'playing ' : 'paused ') + st.title + ' ' + clockOf(st.position) + ' / ' + clockOf(st.duration);
  }

  function fail(e) { say(e.message); }

  function registerPanel() {
    var current = state.me.screen;
    function save(asRemote) {
      return function (name) {
        name = (name || '').trim() || (current ? current.name : '');
        if (!name) { say('name this screen first'); return; }
        api('POST', '/api/me', { name: name, remote: asRemote })
          .then(function () { closePanel(); return refreshScreens(); })
          .then(listen)
          .catch(fail);
      };
    }
    return {
      title: current ? current.name : 'This browser',
      rows: [['', current
        ? 'This browser is one of your screens. As a remote it may also cast to and control your other screens.'
        : 'Make this browser one of your screens, so you can cast to it. As a remote it may also cast to and control your other screens.']],
      field: { placeholder: 'name this screen', value: current ? current.name : '' },
      actions: [
        { label: 'Screen only', run: save(false) },
        { label: 'Screen and remote', run: save(true) }
      ].concat(current ? [{ label: 'Forget this browser', run: function () {
        api('DELETE', '/api/me').then(function () { closePanel(); cast.stop(); return refreshScreens(); }).catch(fail);
      } }] : [])
    };
  }

  function screenPanel(t) {
    var s = t.screen;
    var rows = [['Kind', s.kind], ['Remote', s.remote ? 'yes' : 'no'], ['Seen', ago(s.lastSeen)]];
    if (s.state && s.state.title) rows.unshift(['Now', playing(s.state)]);
    var actions = [];
    function to(path, body, done) {
      return function () {
        api('POST', '/api/screens/' + encodeURIComponent(s.id) + path, body).then(function (r) { say(done(r)); refreshScreens(); }).catch(fail);
      };
    }
    var field = null;
    if (remote()) {
      field = { placeholder: 'a files path or URL to cast' };
      actions.push(
        { label: 'Cast', run: function (ref) {
          if (!ref) { say('which file? type a path first'); return; }
          api('POST', '/api/screens/' + encodeURIComponent(s.id) + '/cast', { url: ref }).then(function (r) { say('cast ' + r.title + ' to ' + s.name); }).catch(fail);
        } },
        { label: 'Play / pause', run: to('/control', { action: 'toggle' }, function () { return 'play / pause'; }) },
        { label: 'Back 30 s', run: to('/control', { action: 'skip', value: -30 }, function () { return '−30 s'; }) },
        { label: 'Forward 30 s', run: to('/control', { action: 'skip', value: 30 }, function () { return '+30 s'; }) },
        { label: 'Stop', run: to('/control', { action: 'stop' }, function () { return 'stopped'; }) },
        { label: 'Rename to what is typed', run: function (name) {
          if (!name) { say('type the new name first'); return; }
          to('/name', { name: name }, function () { return 'renamed'; })();
        } }
      );
      if (!s.here) {
        actions.push({ label: 'Remove', run: function () {
          if (!confirm('Remove ' + s.name + ' from your screens?')) return;
          api('DELETE', '/api/screens/' + encodeURIComponent(s.id)).then(function () { closePanel(); refreshScreens(); }).catch(fail);
        } });
      }
    }
    if (s.here) actions.push({ label: 'This browser…', run: function () { openPanel('details', registerPanel()); } });
    if (!actions.length) rows.push(['', 'This browser is not a remote, so it can only receive.']);
    return { title: s.name, rows: rows, field: field, actions: actions };
  }

  function detailsFor(t) {
    if (t.kind === 'this') return registerPanel();
    if (t.kind === 'screen') return screenPanel(t);
    if (t.kind === 'adb') return { title: t.label, rows: [['', 'An adb remote for this TV.']], actions: [{ label: 'Open the remote', run: t.open }] };
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
      rows: [['', onDevice ? 'Saved on this TV. Hidden apps: ' + state.hidden.length + '.' : 'Saved in this browser.']],
      actions: [
        flip('still', 'Boil: off', 'Boil: on'),
        flip('scanlines', 'Scanlines: on', 'Scanlines: off'),
        flip('clock24', 'Clock: 24 hour', 'Clock: 12 hour'),
        flip('showHidden', 'Hidden apps: shown', 'Hidden apps: kept hidden')
      ].concat(onDevice ? [{ label: 'TV settings', run: function () { closePanel(); host.openSettings(); } }] : [])
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
    var input = null;
    if (p.field) {
      input = el('input', 'drawer-field fontpack-mono');
      input.placeholder = p.field.placeholder;
      input.autocomplete = 'off';
      input.autocapitalize = 'off';
      input.spellcheck = false;
      input.value = p.text !== undefined ? p.text : (p.field.value || '');
      input.addEventListener('input', function () { p.text = input.value; });
      drawer.appendChild(input);
    }
    var list = el('div', 'drawer-actions');
    p.actions.forEach(function (a, i) {
      var b = el('button', 'drawer-action fontpack-mono' + (i === p.sel ? ' focus' : ''), a.label);
      b.type = 'button';
      b.addEventListener('click', function () { p.sel = i; runAction(); });
      list.appendChild(b);
    });
    drawer.appendChild(list);
    var close = el('button', 'drawer-close fontpack-mono', 'close');
    close.type = 'button';
    close.addEventListener('click', closePanel);
    drawer.appendChild(close);
    hint.textContent = 'up/down: choose  ·  ok: do it  ·  back: close' + (p.field ? '  ·  i: type' : '');
  }

  function runAction() {
    var p = state.panel;
    var a = p && p.actions[p.sel];
    if (!a) return;
    var text = p.text !== undefined ? p.text : (p.field && p.field.value) || '';
    a.run(text.trim());
  }

  function fieldOf() { return drawer.querySelector('.drawer-field'); }

  function openPanel(kind, p, keepSel) {
    var sel = keepSel && state.panel && state.panel.kind === kind ? state.panel.sel : 0;
    if (keepSel && state.panel && state.panel.text !== undefined) p.text = state.panel.text;
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
      case 'ArrowUp': case 'k': p.sel = Math.max(0, p.sel - 1); renderPanel(); return true;
      case 'ArrowDown': case 'j': p.sel = Math.min(p.actions.length - 1, p.sel + 1); renderPanel(); return true;
      case 'Enter': case ' ': runAction(); return true;
      case 'ArrowLeft': case 'Escape': case 'h': closePanel(); return true;
      case 'i': case ':': if (fieldOf()) { fieldOf().focus(); return true; } return false;
    }
    return false;
  }

  document.addEventListener('keydown', function (e) {
    if (cast.open() || !padSheet.hidden) return;
    if (e.target.tagName === 'INPUT') {
      if (e.key === 'Escape') { e.target.blur(); e.preventDefault(); }
      else if (e.key === 'Enter' && state.panel) { e.target.blur(); runAction(); e.preventDefault(); }
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (state.panel) {
      if (panelKey(e.key)) e.preventDefault();
      return;
    }
    switch (e.key) {
      case 'ArrowUp': case 'k': move(-1, 0); break;
      case 'ArrowDown': case 'j': move(1, 0); break;
      case 'ArrowLeft': case 'h': move(0, -1); break;
      case 'ArrowRight': case 'l': move(0, 1); break;
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

  var padKeys = {
    h: 'left', ArrowLeft: 'left', j: 'down', ArrowDown: 'down', k: 'up', ArrowUp: 'up', l: 'right', ArrowRight: 'right',
    Enter: 'ok', o: 'ok', ' ': 'ok', b: 'back', Backspace: 'back',
    H: 'home', K: 'menu', m: 'menu', p: 'play', '[': 'rewind', ']': 'forward',
    '+': 'volup', '=': 'volup', '-': 'voldown', M: 'mute'
  };
  var padCount = '';
  var padG = false;

  function press(action, times) {
    if (!padFor) return;
    var b = padSheet.querySelector('button[data-a="' + action + '"]');
    if (b) { b.classList.add('lit'); setTimeout(function () { b.classList.remove('lit'); }, 140); }
    var n = times || 1;
    api('POST', '/api/adb/' + encodeURIComponent(padFor.name) + '/press', { action: action, times: n })
      .then(function () { padStatus.textContent = action + (n > 1 ? ' ×' + n : ''); })
      .catch(function (e) { padStatus.textContent = e.message; });
  }

  function openPad(r) {
    padFor = r;
    closePanel();
    document.getElementById('padTitle').textContent = r.tv || r.name;
    padSheet.hidden = false;
  }

  function closePad() {
    padSheet.hidden = true;
    padFor = null;
    focus();
  }

  padSheet.querySelectorAll('button[data-a]').forEach(function (b) {
    b.addEventListener('click', function () { press(b.dataset.a); });
  });
  padSheet.querySelector('button[data-close]').addEventListener('click', closePad);
  document.getElementById('padText').addEventListener('submit', function (e) {
    e.preventDefault();
    var input = e.target.querySelector('input');
    if (!padFor) return;
    if (!input.value) { press('enter'); return; }
    api('POST', '/api/adb/' + encodeURIComponent(padFor.name) + '/text', { text: input.value })
      .then(function () { padStatus.textContent = 'typed'; input.value = ''; })
      .catch(function (err) { padStatus.textContent = err.message; });
  });

  document.addEventListener('keydown', function (e) {
    if (padSheet.hidden) return;
    if (e.target.tagName === 'INPUT') {
      if (e.key === 'Escape') { e.target.blur(); e.preventDefault(); }
      return;
    }
    if (e.metaKey || e.altKey) return;
    var k = e.key;
    if (e.ctrlKey) {
      if (k === 'u' || k === 'd') { e.preventDefault(); press(k === 'u' ? 'up' : 'down', 5 * (parseInt(padCount, 10) || 1)); padCount = ''; }
      return;
    }
    e.preventDefault();
    if (padG) {
      padG = false;
      padCount = '';
      if (k === 'g') press('home');
      else if (k === 's') press('settings');
      else if (k === 'S') press('tvsettings');
      return;
    }
    if (k === 'Escape' || k === 'q') { if (padCount) padCount = ''; else closePad(); return; }
    if (/^[0-9]$/.test(k) && (k !== '0' || padCount)) { if (padCount.length < 2) padCount += k; return; }
    if (k === 'g') { padG = true; return; }
    if (k === 'i' || k === 'a') { padSheet.querySelector('#padText input').focus(); return; }
    var action = padKeys[k];
    if (action) press(action, Math.min(parseInt(padCount, 10) || 1, 50));
    padCount = '';
  });

  function showHere() {
    if (!online || !state.me) { here.textContent = ''; return; }
    var s = state.me.screen;
    here.textContent = state.me.user + (s ? ' · ' + s.name + (s.remote ? ' · remote' : '') : ' · not a screen');
  }

  function refreshScreens() {
    if (!online) return Promise.resolve();
    return api('GET', '/api/me').then(function (me) {
      state.me = me;
      return api('GET', '/api/screens');
    }).then(function (l) {
      state.screens = l.screens || [];
      state.remotes = l.remotes || [];
      showHere();
      build();
      var p = state.panel;
      var t = current();
      if (p && p.kind === 'details' && t && t.screen && document.activeElement !== fieldOf()) openPanel('details', detailsFor(t), true);
    }).catch(function (e) { say(e.message); });
  }

  var listening = false;
  var reported = 0;

  function report(force) {
    if (!state.me || !state.me.screen) return;
    var now = Date.now();
    if (!force && now - reported < 5000) return;
    reported = now;
    api('POST', '/api/me/state', cast.state()).catch(function () {});
  }

  function sleep(ms) { return new Promise(function (r) { setTimeout(r, ms); }); }

  function listen() {
    if (listening || !state.me || !state.me.screen) return;
    listening = true;
    (function loop() {
      fetch('/api/me/next', { credentials: 'same-origin' }).then(function (r) {
        if (r.status === 401) { listening = false; cast.stop(); return refreshScreens(); }
        if (r.status === 204) return loop();
        if (!r.ok) return sleep(5000).then(loop);
        return r.json().then(function (e) {
          if (e.kind === 'cast') cast.play(e);
          else if (e.kind === 'control') cast.control(e.action, e.value);
          else if (e.kind === 'forgotten') { listening = false; cast.stop(); return refreshScreens(); }
          return loop();
        });
      }, function () { return sleep(5000).then(loop); });
    })();
  }

  cast.onChange(function () { report(true); });
  document.getElementById('video').addEventListener('timeupdate', function () { report(false); });

  window.aguatv = {
    refresh: function () { build(); if (state.panel && state.panel.kind === 'settings') openSettings(true); },
    resume: function () { tick(); build(); },
    pause: function () { clearTimeout(tickTimer); },
    home: function () { cast.stop(); closePad(); closePanel(); first(); },
    back: function () {
      if (cast.open()) cast.stop();
      else if (!padSheet.hidden) closePad();
      else if (state.panel) closePanel();
      else first();
    },
    details: openDetails,
    settings: function () { openSettings(); }
  };

  document.body.classList.toggle('online', online);
  applyPrefs();
  tick();
  build();
  if (online) {
    refreshScreens().then(listen);
    setInterval(function () { if (!document.hidden) refreshScreens(); }, 5000);
  }
  requestAnimationFrame(function () {
    requestAnimationFrame(function () { host.ready(); });
  });
})();
