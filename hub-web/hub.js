(function () {
  'use strict';

  var statusEl = document.getElementById('status');
  var dot = document.getElementById('dot');
  var listEl = document.getElementById('devices');
  var castPanel = document.getElementById('cast');
  var adbPanel = document.getElementById('adb');
  var keysEl = document.getElementById('keys');
  var selected = null;
  var count = '';
  var pendingG = false;

  function say(text, bad) {
    statusEl.textContent = text;
    statusEl.classList.toggle('err', !!bad);
  }

  function api(method, path, body) {
    var opts = { method: method, credentials: 'same-origin', headers: {} };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    return fetch(path, opts).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.error || r.status + ' ' + r.statusText);
        return j;
      });
    });
  }

  function ago(iso) {
    var s = (Date.now() - new Date(iso).getTime()) / 1000;
    if (s < 90) return 'just now';
    if (s < 5400) return Math.round(s / 60) + ' min ago';
    if (s < 129600) return Math.round(s / 3600) + ' h ago';
    return Math.round(s / 86400) + ' days ago';
  }

  function clock(t) {
    t = Math.max(0, Math.round(t || 0));
    var m = Math.floor(t / 60), s = t % 60;
    return m + ':' + (s < 10 ? '0' : '') + s;
  }

  function render(data) {
    listEl.textContent = '';
    var items = [];
    (data.remotes || []).forEach(function (r) {
      items.push({ key: 'adb:' + r.name, kind: 'remote', name: r.tv || r.name, meta: r.connected ? 'adb remote, connected' : 'adb remote' + (r.error ? ': ' + r.error : ''), remote: r.name });
    });
    (data.devices || []).forEach(function (d) {
      var meta = d.kind + (d.session ? ' (this session)' : '') + ', seen ' + ago(d.lastSeen);
      if (d.state && d.state.title) meta = (d.state.playing ? 'playing ' : 'paused ') + d.state.title + ' ' + clock(d.state.position) + ' / ' + clock(d.state.duration);
      items.push({ key: 'dev:' + d.id, kind: d.kind, name: d.name, meta: meta, id: d.id });
    });
    if (!items.length) {
      var p = document.createElement('p');
      p.className = 'empty';
      p.textContent = 'No screens yet. Open tv.kelliher.info/receiver on a TV or browser to link one.';
      listEl.appendChild(p);
    }
    items.forEach(function (it) {
      var b = document.createElement('button');
      b.className = 'device' + (selected && selected.key === it.key ? ' sel' : '');
      b.dataset.key = it.key;
      var n = document.createElement('span'); n.className = 'name'; n.textContent = it.name;
      var k = document.createElement('span'); k.className = 'kind'; k.textContent = it.kind;
      var m = document.createElement('span'); m.className = 'meta'; m.textContent = it.meta;
      b.appendChild(n); b.appendChild(k); b.appendChild(m);
      b.addEventListener('click', function () { select(it); });
      listEl.appendChild(b);
    });
    if (!selected && items.length) select(items[0]);
    else if (selected) {
      var still = items.filter(function (i) { return i.key === selected.key; })[0];
      if (!still) select(items[0] || null);
    }
  }

  function select(it) {
    selected = it;
    castPanel.hidden = !it || !it.id;
    adbPanel.hidden = !it || !it.remote;
    if (it && it.id) document.getElementById('castTitle').textContent = it.name;
    if (it && it.remote) document.getElementById('adbTitle').textContent = it.name;
    Array.prototype.forEach.call(listEl.children, function (c) {
      c.classList.toggle('sel', !!it && c.dataset.key === it.key);
    });
  }

  function refresh() {
    api('GET', '/api/devices').then(function (d) { render(d); dot.className = 'dot on'; })
      .catch(function (e) { say(e.message, true); dot.className = 'dot err'; });
  }

  function light(sel) {
    var b = document.querySelector(sel);
    if (!b) return;
    b.classList.add('lit');
    setTimeout(function () { b.classList.remove('lit'); }, 140);
  }

  function press(action, times) {
    if (!selected || !selected.remote) return;
    light('#adb button[data-a="' + action + '"]');
    var n = times || 1;
    api('POST', '/api/adb/' + encodeURIComponent(selected.remote) + '/press', { action: action, times: n })
      .then(function () { say(action + (n > 1 ? ' ×' + n : '')); })
      .catch(function (e) { say(e.message, true); });
  }

  function control(action, value) {
    if (!selected || !selected.id) return;
    api('POST', '/api/devices/' + selected.id + '/control', { action: action, value: value || 0 })
      .then(function () { say(action + (value ? ' ' + value : '')); })
      .catch(function (e) { say(e.message, true); });
  }

  document.querySelectorAll('#adb button[data-a]').forEach(function (b) {
    b.addEventListener('click', function () { press(b.dataset.a); });
  });
  document.querySelectorAll('#cast button[data-c]').forEach(function (b) {
    b.addEventListener('click', function () { control(b.dataset.c, parseFloat(b.dataset.v || '0')); });
  });

  document.getElementById('castForm').addEventListener('submit', function (e) {
    e.preventDefault();
    var ref = document.getElementById('ref').value.trim();
    if (!ref || !selected || !selected.id) return;
    say('casting');
    api('POST', '/api/devices/' + selected.id + '/cast', { ref: ref })
      .then(function (r) { say('cast ' + r.title); })
      .catch(function (err) { say(err.message, true); });
  });

  document.getElementById('rename').addEventListener('click', function () {
    if (!selected || !selected.id) return;
    var name = prompt('Name this screen', selected.name);
    if (!name) return;
    api('POST', '/api/devices/' + selected.id + '/rename', { name: name }).then(refresh).catch(function (e) { say(e.message, true); });
  });

  document.getElementById('unlink').addEventListener('click', function () {
    if (!selected || !selected.id) return;
    if (!confirm('Unlink ' + selected.name + '? It will need a new code.')) return;
    api('DELETE', '/api/devices/' + selected.id).then(function () { selected = null; refresh(); }).catch(function (e) { say(e.message, true); });
  });

  document.getElementById('type').addEventListener('submit', function (e) {
    e.preventDefault();
    var input = document.getElementById('text');
    if (!selected || !selected.remote) return;
    if (!input.value) { press('enter'); return; }
    api('POST', '/api/adb/' + encodeURIComponent(selected.remote) + '/text', { text: input.value })
      .then(function () { say('typed'); input.value = ''; })
      .catch(function (err) { say(err.message, true); });
  });

  var keymap = {
    h: 'left', ArrowLeft: 'left', j: 'down', ArrowDown: 'down', k: 'up', ArrowUp: 'up', l: 'right', ArrowRight: 'right',
    Enter: 'ok', o: 'ok', ' ': 'ok', b: 'back', Escape: 'back', Backspace: 'back',
    H: 'home', K: 'menu', m: 'menu', p: 'play', '[': 'rewind', ']': 'forward',
    '+': 'volup', '=': 'volup', '-': 'voldown', M: 'mute'
  };
  var castKeys = { p: ['toggle'], ' ': ['toggle'], Enter: ['toggle'], '[': ['skip', -30], ']': ['skip', 30], h: ['skip', -10], l: ['skip', 10], ArrowLeft: ['skip', -10], ArrowRight: ['skip', 10] };

  document.addEventListener('keydown', function (e) {
    if (e.target.tagName === 'INPUT') {
      if (e.key === 'Escape') e.target.blur();
      return;
    }
    if (e.metaKey || e.altKey) return;
    if (e.ctrlKey) {
      if (selected && selected.remote && (e.key === 'u' || e.key === 'd')) {
        e.preventDefault();
        press(e.key === 'u' ? 'up' : 'down', 5 * (parseInt(count, 10) || 1));
        count = '';
      }
      return;
    }
    var k = e.key;
    if (k === '?') { keysEl.hidden = !keysEl.hidden; e.preventDefault(); return; }
    if (k === ':') { document.getElementById('ref').focus(); e.preventDefault(); return; }
    if (selected && selected.id) {
      var c = castKeys[k];
      if (c) { e.preventDefault(); control(c[0], c[1]); }
      return;
    }
    if (!selected || !selected.remote) return;
    if (pendingG) {
      pendingG = false;
      e.preventDefault();
      if (k === 'g') press('home');
      else if (k === 's') press('settings');
      else if (k === 'S') press('tvsettings');
      else if (k === 'h') press('quick');
      count = '';
      return;
    }
    if (/^[0-9]$/.test(k) && (k !== '0' || count)) {
      if (count.length < 2) count += k;
      e.preventDefault();
      return;
    }
    if (k === 'Escape' && count) { count = ''; e.preventDefault(); return; }
    if (k === 'g') { pendingG = true; e.preventDefault(); return; }
    if (k === 'i' || k === 'a') { document.getElementById('text').focus(); e.preventDefault(); return; }
    var action = keymap[k];
    if (!action) { count = ''; return; }
    e.preventDefault();
    press(action, Math.min(parseInt(count, 10) || 1, 50));
    count = '';
  });

  api('GET', '/api/me').then(function (m) { document.getElementById('who').textContent = m.user; }).catch(function () {});
  refresh();
  setInterval(refresh, 5000);
})();
