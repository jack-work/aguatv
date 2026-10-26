(function () {
  'use strict';
  var statusEl = document.getElementById('status');
  var codeEl = document.getElementById('code');
  var found = document.getElementById('found');
  var current = null;

  function say(t, bad) { statusEl.textContent = t; statusEl.classList.toggle('err', !!bad); }

  function api(method, path, body) {
    var opts = { method: method, credentials: 'same-origin', headers: {} };
    if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
    return fetch(path, opts).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (!r.ok) throw new Error(j.error || r.status + ' ' + r.statusText);
        return j;
      });
    });
  }

  function lookup(code) {
    found.hidden = true;
    api('GET', '/api/link?code=' + encodeURIComponent(code)).then(function (p) {
      current = p.code;
      document.getElementById('kind').textContent = p.kind === 'tv' ? 'a TV' : 'a browser';
      document.getElementById('asked').textContent = new Date(p.asked).toLocaleTimeString();
      document.getElementById('from').textContent = p.from;
      document.getElementById('lasts').textContent = p.session ? 'until that browser closes, 24 h at most' : '90 days after it was last used';
      document.getElementById('name').value = p.name;
      found.hidden = false;
      say('');
      document.getElementById('name').focus();
    }).catch(function (e) { say(e.message, true); });
  }

  document.getElementById('lookup').addEventListener('submit', function (e) {
    e.preventDefault();
    if (codeEl.value.trim()) lookup(codeEl.value.trim());
  });

  document.getElementById('approve').addEventListener('submit', function (e) {
    e.preventDefault();
    if (!current) return;
    api('POST', '/api/link', { code: current, name: document.getElementById('name').value.trim() }).then(function (d) {
      found.hidden = true;
      codeEl.value = '';
      current = null;
      say('Linked ' + d.name + '. It will show up in your screens in a moment.');
    }).catch(function (err) { say(err.message, true); });
  });

  api('GET', '/api/me').then(function (m) { document.getElementById('who').textContent = m.user; }).catch(function () {});
  var q = new URLSearchParams(location.search).get('code');
  if (q) { codeEl.value = q; lookup(q); }
})();
