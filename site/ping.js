// Visit ping, current contract: anonymous POST to the ping API, no token.
// v2 replaces ENDPOINT with a same-origin Pages Function.
(function () {
  'use strict';

  var ENDPOINT = 'https://wvp0iszsrh.execute-api.us-east-1.amazonaws.com/ping';
  var FLAG = 'costgate_ping_sent';
  var DWELL_MS = 5000;
  var BOT_UA = /bot|crawl|spider|slurp|headless|lighthouse|pagespeed|prerender|preview|facebookexternalhit|embedly|whatsapp|telegram|discord|slack|curl|wget|python|go-http|java\//i;

  var ua = navigator.userAgent || '';

  if (navigator.webdriver === true) return;
  if (BOT_UA.test(ua)) return;
  if (document.prerendering === true || document.visibilityState === 'prerender') return;

  var storageOk = true;
  try {
    if (sessionStorage.getItem(FLAG)) return;
  } catch (e) {
    storageOk = false;
  }

  // Fallback when sessionStorage is unavailable: at most once per page load.
  var sent = false;

  function service() {
    if (new URLSearchParams(location.search).get('ref') === 'readme') return 'costgate-readme';
    if (document.referrer.indexOf('https://github.com/leosavio/costgate') === 0) return 'costgate-github';
    return 'costgate-site';
  }

  function sessionId() {
    if (window.crypto && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
    var bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    var hex = '';
    for (var i = 0; i < bytes.length; i++) {
      hex += (bytes[i] < 16 ? '0' : '') + bytes[i].toString(16);
    }
    return hex;
  }

  function device() {
    var coarse = typeof window.matchMedia === 'function' && window.matchMedia('(pointer: coarse)').matches;
    return coarse || /Mobi|Android/i.test(ua) ? 'mobile' : 'desktop';
  }

  var timer = null;
  var visibleSince = null;
  var accumulated = 0;

  function cleanup() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    document.removeEventListener('visibilitychange', onVisibility);
    window.removeEventListener('pointerdown', fire);
    window.removeEventListener('keydown', fire);
  }

  function fire() {
    if (sent) return;
    sent = true;
    cleanup();

    var body = JSON.stringify({
      service: service(),
      type: 'session_start',
      session_id: sessionId(),
      timestamp: new Date().toISOString(),
      metadata: { device: device() }
    });

    var queued = false;
    if (typeof navigator.sendBeacon === 'function') {
      try {
        queued = navigator.sendBeacon(ENDPOINT, body);
      } catch (e) {
        queued = false;
      }
    }
    if (!queued && typeof window.fetch === 'function') {
      fetch(ENDPOINT, {
        method: 'POST',
        body: body,
        keepalive: true,
        mode: 'no-cors',
        credentials: 'omit'
      }).catch(function () {});
    }

    if (storageOk) {
      try {
        sessionStorage.setItem(FLAG, '1');
      } catch (e) {
        // ignore: the local `sent` flag still limits this page load
      }
    }
  }

  function startDwell() {
    if (sent || timer !== null) return;
    visibleSince = Date.now();
    timer = setTimeout(fire, Math.max(0, DWELL_MS - accumulated));
  }

  function pauseDwell() {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
    if (visibleSince !== null) {
      accumulated += Date.now() - visibleSince;
      visibleSince = null;
    }
  }

  function onVisibility() {
    if (document.visibilityState === 'visible') {
      startDwell();
    } else {
      pauseDwell();
    }
  }

  document.addEventListener('visibilitychange', onVisibility);
  window.addEventListener('pointerdown', fire, { once: true, passive: true });
  window.addEventListener('keydown', fire, { once: true, passive: true });

  if (document.visibilityState === 'visible') startDwell();
})();
