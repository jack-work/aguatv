(function () {
  'use strict';

  var root = document.getElementById('cast');
  var video = document.getElementById('video');
  var bar = document.getElementById('castBar');
  var filo = null;
  var title = '';
  var listeners = [];
  var barShown = false;

  function state() {
    return {
      title: title,
      position: video.currentTime || 0,
      duration: isFinite(video.duration) ? video.duration : 0,
      playing: !!title && !video.paused
    };
  }

  function tell() {
    var s = state();
    listeners.forEach(function (f) { f(s); });
  }

  function close() {
    if (filo) { filo.detach(); filo = null; }
    video.pause();
    video.removeAttribute('src');
    video.load();
    title = '';
    root.hidden = true;
    document.body.classList.remove('casting');
    tell();
  }

  function play(e) {
    if (filo) filo.detach();
    title = e.title || '';
    root.hidden = false;
    document.body.classList.add('casting');
    video.src = e.url;
    filo = window.zanni.filo.attach(bar, video, { globalKeys: true, autohide: 4000, title: title });
    video.play().catch(function () { filo.showBar(true); });
    tell();
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
      case 'bar': filo.showBar(value < 0 ? !barShown : value > 0); break;
    }
  }

  function stop() {
    if (filo) filo.stop();
    else close();
  }

  bar.addEventListener('filo:stop', close);
  bar.addEventListener('filo:bar', function (e) { barShown = !!(e.detail && e.detail.shown); });
  bar.addEventListener('filo:seek', tell);
  video.addEventListener('play', tell);
  video.addEventListener('pause', tell);
  video.addEventListener('ended', stop);
  video.addEventListener('click', function () { if (filo) filo.showBar(true); });
  document.addEventListener('keydown', function (e) {
    if (filo && e.key === 'Escape') { e.preventDefault(); e.stopImmediatePropagation(); stop(); }
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
