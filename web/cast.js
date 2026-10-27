(function () {
  'use strict';

  var root = document.getElementById('cast');
  var video = document.getElementById('video');
  var filo = null;
  var hls = null;
  var item = null;
  var listeners = [];
  var saved = 0;

  function online() { return !window.host && !/[?&]preview\b/.test(location.search); }

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
    if (hls) { hls.destroy(); hls = null; }
    video.pause();
    video.removeAttribute('src');
    video.removeAttribute('poster');
    video.load();
  }

  function close() {
    savePosition(true);
    if (filo) { filo.detach(); filo = null; }
    unload();
    item = null;
    root.hidden = true;
    document.body.classList.remove('casting');
    tell();
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
    if (master && video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = master;
      resume();
      return Promise.resolve();
    }
    if (master && window.MediaSource) {
      return loadHls().then(function (Hls) {
        hls = new Hls({ startPosition: at || -1 });
        hls.loadSource(master);
        hls.attachMedia(video);
        at = 0;
        resume();
      }).catch(function () {
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

  function show(it) {
    if (filo) filo.detach();
    unload();
    item = it;
    root.hidden = false;
    document.body.classList.add('casting');
    if (it.poster) video.setAttribute('poster', it.poster);
    var chapters = (it.chapters || []).map(function (c) { return { start: c.start, title: c.title }; });
    var qualities = qualitiesOf(it);
    filo = window.zanni.filo.attach(video, {
      globalKeys: true,
      autohide: 4000,
      fill: true,
      boil: !window.host,
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
    tell();
  }

  function play(e) {
    if (e.url) {
      show({ key: e.url, title: e.title || '', original: e.url, kind: 'video', state: 'none' });
      return Promise.resolve();
    }
    return fetch(itemPath(e.key), { credentials: 'same-origin' }).then(function (r) {
      if (!r.ok) throw new Error('that file is not yours to play (' + r.status + ')');
      return r.json();
    }).then(function (it) {
      it.title = it.title || e.title || e.key;
      it.kind = it.kind || (/\.(mp4|m4v|mov|mkv|webm)$/i.test(e.key) ? 'video' : 'audio');
      if (e.poster) it.poster = e.poster;
      show(it);
    });
  }

  function control(action, value) {
    if (action === 'stop') { stop(); return; }
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

  root.addEventListener('filo:stop', close);
  root.addEventListener('filo:seek', function () { tell(); savePosition(true); });
  root.addEventListener('filo:quality', function (e) { switchQuality(e.detail && e.detail.value); });
  video.addEventListener('play', tell);
  video.addEventListener('pause', function () { tell(); savePosition(true); });
  video.addEventListener('timeupdate', function () { savePosition(false); });
  video.addEventListener('ended', function () { savePosition(true); stop(); });
  document.addEventListener('visibilitychange', function () { if (document.hidden) savePosition(true); });
  document.addEventListener('keydown', function (e) {
    if (filo && e.key === 'Escape' && !filo.open) { e.preventDefault(); e.stopImmediatePropagation(); stop(); }
  }, true);

  window.aguatvCast = {
    play: play,
    control: control,
    stop: stop,
    state: state,
    open: function () { return !!filo; },
    onChange: function (f) { listeners.push(f); }
  };
})();
