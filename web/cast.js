(function () {
  'use strict';

  var root = document.getElementById('cast');
  var video = document.getElementById('video');
  var note = document.getElementById('castNote');
  var waiting = null;
  var READY_POLL_MS = 15000;
  var filo = null;
  var hls = null;
  var item = null;
  var listeners = [];
  var saved = 0;
  var view = 'gallery';
  var pushed = false;
  var viewers = [];
  var loads = 0;

  function online() { return !window.host && !/[?&]preview\b/.test(location.search); }

  function onTv() { return !!window.aguatvReceiver; }

  function state() {
    return {
      title: item ? item.title : '',
      key: item ? item.key : '',
      position: video.currentTime || 0,
      duration: isFinite(video.duration) ? video.duration : 0,
      playing: !!item && !video.paused
    };
  }

  function tell() {
    var s = state();
    listeners.forEach(function (f) { f(s); });
  }

  function itemPath(key) {
    return '/_media/api/item/' + key.split('/').map(encodeURIComponent).join('/');
  }

  function savePosition(force) {
    if (!item || !online() || !isFinite(video.duration) || !video.duration) return;
    var now = Date.now();
    if (!force && now - saved < 10000) return;
    saved = now;
    fetch(itemPath(item.key), {
      method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ position: video.currentTime || 0, duration: video.duration })
    }).catch(function () {});
  }

  function unload() {
    loads++;
    if (hls) { hls.destroy(); hls = null; }
    video.pause();
    video.removeAttribute('src');
    video.removeAttribute('poster');
    video.load();
  }

  function unprepare() {
    if (waiting) { clearTimeout(waiting); waiting = null; }
    note.hidden = true;
    note.textContent = '';
  }

  function close() {
    unprepare();
    savePosition(true);
    if (filo) { filo.detach(); filo = null; }
    unload();
    item = null;
    root.hidden = true;
    document.body.classList.remove('casting');
    if (view !== 'gallery') setView('gallery', null, 'replace');
    tell();
  }

  function objectKey(key) {
    if (/[\x00-\x1f\x7f\\]/.test(key)) return false;
    return key.split('/').every(function (p) { return p && p !== '.' && p !== '..'; });
  }

  function routeOf(search) {
    var m = /[?&](w|a)=([^&]+)/.exec(search);
    if (!m) return { view: 'gallery' };
    var key;
    try { key = decodeURIComponent(m[2]); } catch (e) { return { view: 'gallery' }; }
    if (!objectKey(key)) return { view: 'gallery' };
    return { view: m[1] === 'w' ? 'watch' : 'about', key: key };
  }

  function urlFor(v, key) {
    if (v === 'gallery') return location.pathname;
    return location.pathname + '?' + (v === 'watch' ? 'w' : 'a') + '=' + encodeURIComponent(key);
  }

  function setView(v, key, how) {
    view = v;
    document.body.classList.toggle('watching', v === 'watch');
    document.body.classList.toggle('away', v !== 'gallery');
    if (how && online()) {
      history[how + 'State']({ view: v, key: key }, '', urlFor(v, key));
      if (how === 'push') pushed = v !== 'gallery';
    }
    if (v !== 'gallery') window.scrollTo(0, 0);
    viewers.forEach(function (f) { f(v, key); });
  }

  function back() {
    if (pushed && history.state && history.state.view === view) { history.back(); return; }
    if (filo && item && item.kind !== 'audio' && !filo.docked) filo.dock(true);
    setView('gallery', null, 'push');
  }

  function route(r) {
    if (r.view === 'gallery') {
      if (filo && !filo.docked && view === 'watch') filo.dock(true);
      setView('gallery', null, null);
      return Promise.resolve();
    }
    if (item && item.key === r.key) {
      if (r.view === 'watch' && filo && filo.docked) filo.dock(false);
      setView(r.view, r.key, null);
      return Promise.resolve();
    }
    return play({ key: r.key }, r.view === 'about' ? 'about' : 'load');
  }

  function rungLabel(r) {
    if (r.height) return r.height + 'p';
    if (r.name === 's') return 'Speech';
    return r.kbps + ' kbps';
  }

  function qualitiesOf(it) {
    if (it.state !== 'ready' || !it.rungs || !it.rungs.length) return null;
    return [{ value: 'auto', label: 'Auto' }]
      .concat(it.rungs.map(function (r) { return { value: r.name, label: rungLabel(r) }; }))
      .concat([{ value: 'original', label: 'Original' }]);
  }

  function loadHls() {
    return import('/_media/static/hls.light.min.mjs').then(function (mod) {
      var Hls = mod.default || mod.Hls;
      if (!Hls || !Hls.isSupported()) throw new Error('no hls');
      return Hls;
    });
  }

  function source(src, master, at, playing) {
    if (hls) { hls.destroy(); hls = null; }
    function resume() {
      if (at) {
        var seek = function () { video.removeEventListener('loadedmetadata', seek); try { video.currentTime = at; } catch (e) {} };
        video.addEventListener('loadedmetadata', seek);
      }
      if (playing) video.play().catch(function () { if (filo) filo.showBar(true); });
    }
    if (master && !window.aguatvReceiver && video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = master;
      resume();
      return Promise.resolve();
    }
    if (master && window.MediaSource) {
      var load = loads;
      return loadHls().then(function (Hls) {
        if (load !== loads) return;
        hls = new Hls({ startPosition: at || -1 });
        hls.loadSource(master);
        hls.attachMedia(video);
        at = 0;
        resume();
      }).catch(function () {
        if (load !== loads) return;
        video.src = src;
        resume();
      });
    }
    video.src = src;
    resume();
    return Promise.resolve();
  }

  function switchQuality(value) {
    if (!item || item.state !== 'ready') return;
    var at = video.currentTime || 0;
    var playing = !video.paused;
    if (value === 'original') { source(item.original, null, at, playing); return; }
    var rung = item.rungs.filter(function (r) { return r.name === value; })[0];
    if (hls && value === 'auto') { hls.currentLevel = -1; return; }
    if (hls && rung) { hls.currentLevel = item.rungs.indexOf(rung); return; }
    source(item.original, value === 'auto' || !rung ? item.master : rung.playlist, at, playing);
  }

  function prepare(it, how) {
    unprepare();
    if (filo) { filo.detach(); filo = null; }
    unload();
    item = it;
    root.hidden = false;
    document.body.classList.add('casting');
    if (it.poster) video.setAttribute('poster', it.poster);
    note.textContent = it.state === 'failed' ? 'This video could not be prepared for the TV' : 'Preparing this for the TV';
    note.hidden = false;
    tell();
    if (it.state === 'failed') return;
    waiting = setTimeout(function () {
      waiting = null;
      if (item !== it) return;
      describe({ key: it.key, title: it.title, poster: it.poster }).then(function (fresh) {
        if (item !== it) return;
        if (fresh.state === 'ready') show(fresh, how);
        else prepare(fresh, how);
      }, function () { if (item === it) prepare(it, how); });
    }, READY_POLL_MS);
  }

  function show(it, how) {
    unprepare();
    var wasDocked = !!(filo && filo.docked);
    if (filo) filo.detach();
    unload();
    item = it;
    root.hidden = false;
    var web = online();
    var tv = onTv();
    var audio = it.kind === 'audio';
    var docked = web && !tv && (audio || (wasDocked && how === 'cast'));
    if (!web || tv) document.body.classList.add('casting');
    if (it.poster) video.setAttribute('poster', it.poster);
    var chapters = (it.chapters || []).map(function (c) { return { start: c.start, title: c.title }; });
    var qualities = qualitiesOf(it);
    filo = window.zanni.filo.attach(video, {
      globalKeys: true,
      autohide: 4000,
      fill: !web || tv,
      bar: web && !tv,
      docked: docked,
      boil: false,
      kind: it.kind === 'audio' ? 'audio' : undefined,
      title: it.title,
      chapters: chapters,
      duration: it.duration || undefined,
      thumbnails: it.thumbnails || undefined,
      qualities: qualities || undefined,
      quality: qualities ? 'auto' : undefined
    });
    filo.focus();
    var resume = it.position && it.position.position > 5 && (!it.duration || it.position.position < it.duration - 10) ? it.position.position : 0;
    source(it.original, it.state === 'ready' ? it.master : null, resume, true);
    if (tv) setView('gallery', null, null);
    else if (web && how === 'about') setView('about', it.key, null);
    else if (web && !docked) setView('watch', it.key, how === 'load' ? null : 'push');
    tell();
  }

  function play(e, how) {
    how = how || (e.kind === 'cast' ? 'cast' : 'pick');
    if (e.url) {
      show({ key: e.url, title: e.title || '', original: e.url, kind: 'video', state: 'none' }, how);
      return Promise.resolve();
    }
    return describe(e).then(function (it) {
      if (onTv() && it.kind === 'video' && it.state !== 'ready') prepare(it, how);
      else show(it, how);
    });
  }

  function describe(e) {
    return fetch(itemPath(e.key), { credentials: 'same-origin' }).then(function (r) {
      if (!r.ok) throw new Error('that file is not yours to play (' + r.status + ')');
      return r.json();
    }).then(function (it) {
      it.title = it.title || e.title || e.key;
      it.kind = it.kind || (isVideo(e.key) ? 'video' : 'audio');
      if (e.poster) it.poster = e.poster;
      return it;
    });
  }

  function isVideo(key) { return /\.(mp4|m4v|mov|mkv|webm)$/i.test(key); }

  function prepareFor(key) {
    if (!isVideo(key) || !online()) return Promise.resolve();
    return fetch(itemPath(key), {
      method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ingest: true })
    }).then(function (r) { return r.ok ? r.json() : null; }, function () { return null; });
  }

  var REMOTE_KEYS = ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Enter'];

  function pressKey(key) {
    if (REMOTE_KEYS.indexOf(key) < 0) return;
    (document.activeElement || document.body).dispatchEvent(new KeyboardEvent('keydown', { key: key, bubbles: true, cancelable: true }));
  }

  function control(action, value) {
    if (action === 'stop') { stop(); return; }
    if (action === 'key') { pressKey(value); return; }
    if (!filo) return;
    switch (action) {
      case 'toggle': filo.toggle(); break;
      case 'skip': filo.skip(value); break;
      case 'seekTo': filo.seekTo(value); break;
      case 'seekFraction': filo.seekFraction(value); break;
      case 'chapter': filo.chapter(value < 0 ? -1 : 1); break;
      case 'bar': filo.showBar(value < 0 ? !filo.shown : value > 0); break;
    }
  }

  function stop() {
    if (filo) filo.stop();
    else close();
  }

  function preparing() { return !!item && !filo && !note.hidden; }

  root.addEventListener('filo:stop', close);
  root.addEventListener('filo:expand', function () {
    if (!item) return;
    if (item.kind === 'audio') { setView('about', item.key, 'push'); return; }
    filo.dock(false);
    setView('watch', item.key, 'push');
  });
  video.addEventListener('enterpictureinpicture', function () { if (view === 'watch') back(); });
  video.addEventListener('webkitpresentationmodechanged', function () { if (video.webkitPresentationMode === 'picture-in-picture' && view === 'watch') back(); });
  window.addEventListener('popstate', function () { if (online()) route(routeOf(location.search)).catch(function () {}); });
  root.addEventListener('filo:seek', function () { tell(); savePosition(true); });
  root.addEventListener('filo:quality', function (e) { switchQuality(e.detail && e.detail.value); });
  video.addEventListener('play', tell);
  video.addEventListener('pause', function () { tell(); savePosition(true); });
  video.addEventListener('timeupdate', function () { savePosition(false); });
  video.addEventListener('ended', function () { savePosition(true); stop(); });
  document.addEventListener('visibilitychange', function () { if (document.hidden) savePosition(true); });
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape' || (filo && filo.open) || (!filo && !preparing())) return;
    if (!online() || onTv() || preparing()) stop();
    else if (view !== 'gallery') back();
    else return;
    e.preventDefault();
    e.stopImmediatePropagation();
  }, true);

  window.addEventListener('message', function (e) {
    if (window.aguatvReceiver || e.data !== 'aguatv-receiver' || (e.origin && e.origin !== location.origin) || !e.ports || !e.ports[0]) return;
    var port = e.ports[0];
    var waiting = {};
    var next = 1;
    port.onmessage = function (m) {
      var r;
      try { r = JSON.parse(m.data); } catch (err) { return; }
      if (r && r.event === 'back' && r.id === undefined) { window.dispatchEvent(new Event('aguatv:back')); return; }
      var w = waiting[r.id];
      if (!w) return;
      delete waiting[r.id];
      if (r.status >= 200 && r.status < 300) {
        var body = {};
        try { body = r.body ? JSON.parse(r.body) : {}; } catch (err) {}
        w.resolve(body);
      } else {
        w.reject(new Error('the TV could not save that (' + r.status + ')'));
      }
    };
    window.aguatvReceiver = {
      call: function (op, body) {
        var id = next++;
        return new Promise(function (resolve, reject) {
          waiting[id] = { resolve: resolve, reject: reject };
          port.postMessage(JSON.stringify({ id: id, op: op, body: body || {} }));
          setTimeout(function () { if (waiting[id]) { delete waiting[id]; reject(new Error('the TV did not answer')); } }, 15000);
        });
      }
    };
    window.dispatchEvent(new Event('aguatv:receiver'));
  });

  window.aguatvCast = {
    play: play,
    control: control,
    stop: stop,
    state: state,
    open: function () { return !!filo || preparing(); },
    covering: function () { return view !== 'gallery' || (!!filo && !filo.docked) || preparing(); },
    prepareFor: prepareFor,
    item: function () { return item; },
    view: function () { return view; },
    back: back,
    start: function () { return online() ? route(routeOf(location.search)) : Promise.resolve(); },
    onView: function (f) { viewers.push(f); },
    onChange: function (f) { listeners.push(f); }
  };
})();
