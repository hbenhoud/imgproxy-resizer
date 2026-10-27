(() => {
  const $ = (s, el = document) => el.querySelector(s);
  const list = $('#variants');
  const rowTpl = $('#row-tpl');
  const STORAGE = 'imgproxy-resizer:form';
  const KB = 1024;

  // Prefill sets come from the config file (presets:), rendered by the server.
  let PRESETS = [];
  try { PRESETS = JSON.parse($('#presets').textContent) || []; } catch (_) {}
  const DEFAULT_ROWS = PRESETS.length ? PRESETS[0].variants : [{ resizeType: 'force' }];

  let toastTimer;
  function toast(msg) {
    const t = $('#toast'); t.textContent = msg; t.classList.add('show');
    clearTimeout(toastTimer); toastTimer = setTimeout(() => t.classList.remove('show'), 1400);
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text); }
    catch (_) {
      const ta = document.createElement('textarea'); ta.value = text; document.body.appendChild(ta);
      ta.select(); document.execCommand('copy'); ta.remove();
    }
    toast('Copied');
  }

  function addRow(v = {}) {
    const li = rowTpl.content.firstElementChild.cloneNode(true);
    for (const [k, val] of Object.entries(v)) {
      const el = li.querySelector(`[name=${k}]`);
      if (!el) continue;
      if (el.type === 'checkbox') el.checked = !!val;
      else el.value = (val == null || val === 0) ? '' : val; // 0 means "unset" for numeric options
    }
    li.querySelector('.remove').addEventListener('click', () => { li.remove(); save(); });
    li.addEventListener('input', () => { clearError(li); save(); });
    list.appendChild(li);
    return li;
  }

  function clearError(li) {
    li.classList.remove('has-error');
    const e = li.querySelector('.v-error'); e.hidden = true; e.textContent = '';
  }

  function readRows() {
    return [...list.querySelectorAll('.variant')].map(li => {
      const g = n => li.querySelector(`[name=${n}]`);
      const num = n => { const x = g(n).value.trim(); return x === '' ? 0 : Number(x); };
      return {
        label: g('label').value.trim(),
        resizeType: g('resizeType').value,
        width: num('width'), height: num('height'),
        enlarge: g('enlarge').checked, extend: g('extend').checked,
        quality: num('quality'), maxKB: num('maxKB'), sharpen: num('sharpen'),
        extra: g('extra').value.trim(),
      };
    });
  }

  function formState() {
    return { profile: $('#profile').value, source: $('#source').value.trim(), ext: $('#ext').value,
      preview: $('#preview').checked, variants: readRows() };
  }

  function save() {
    try { localStorage.setItem(STORAGE, JSON.stringify(formState())); } catch (_) {}
  }

  function restore() {
    let st = null;
    try { st = JSON.parse(localStorage.getItem(STORAGE) || 'null'); } catch (_) {}
    if (!st) { DEFAULT_ROWS.forEach(addRow); return; }
    if (st.profile && $(`#profile option[value="${st.profile}"]`)) $('#profile').value = st.profile;
    $('#source').value = st.source || '';
    if (st.ext !== undefined) $('#ext').value = st.ext;
    $('#preview').checked = st.preview !== false;
    (st.variants && st.variants.length ? st.variants : DEFAULT_ROWS).forEach(addRow);
  }

  function updateHost() {
    const o = $('#profile').selectedOptions[0];
    $('#profile-host').textContent = o ? o.dataset.host : '';
  }

  function setStatus(msg, isErr) {
    const s = $('#status'); s.textContent = msg; s.className = 'status' + (isErr ? ' err' : '');
  }

  const fmtKB = b => `${Math.round(b / KB)} KB`;

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // Bytes of a cross-origin image are only readable when imgproxy sends
  // Timing-Allow-Origin; otherwise the entry reports 0 and we stay silent.
  function transferredBytes(url) {
    const e = performance.getEntriesByName(url).pop();
    return e ? (e.decodedBodySize || e.encodedBodySize || 0) : 0;
  }

  function renderResult(r, v, preview) {
    const div = el('div', 'result' + (r.error ? ' error' : ''));
    const head = el('div', 'head');
    head.append(el('span', 'label', r.label));
    if (!r.error) {
      head.append(el('span', 'spec', `${v.width}×${v.height}${v.maxKB ? ` · ≤ ${v.maxKB} KB` : ''}${v.quality ? ` · q ${v.quality}` : ''}`));
      head.append(el('span', 'opts', r.options));
    }
    div.append(head);

    if (r.error) {
      div.append(el('p', 'err', r.error));
      return div;
    }

    const foot = el('div', 'foot');
    const b = el('button', null, 'Copy URL'); b.type = 'button';
    b.addEventListener('click', () => copy(r.url));
    const meta = el('span', 'meta');
    foot.append(b, meta);

    if (preview) {
      const frame = el('div', 'frame');
      const scroller = el('div', 'scroller');
      const img = new Image();
      img.alt = `${r.label} ${v.width}×${v.height}`;
      img.decoding = 'async';
      img.addEventListener('load', () => {
        const dimsOk = img.naturalWidth === v.width && img.naturalHeight === v.height;
        let txt = `rendered ${img.naturalWidth}×${img.naturalHeight}`;
        if (!dimsOk && v.width && v.height) txt += ' (differs from target)';
        const bytes = transferredBytes(r.url);
        if (bytes) {
          const over = v.maxKB && bytes > v.maxKB * KB;
          txt += ` · ${fmtKB(bytes)}${over ? ', over the max' : ''}`;
          meta.classList.add(over ? 'err' : 'ok');
        } else if (!dimsOk) {
          meta.classList.add('err');
        }
        meta.textContent = txt;
      });
      img.addEventListener('error', () => {
        meta.textContent = 'imgproxy did not return an image: open the URL to see the error';
        meta.classList.add('err');
      });
      img.src = r.url;
      scroller.append(img); frame.append(scroller); div.append(frame);
    }

    const url = el('div', 'url');
    const a = el('a', null, r.url); a.href = r.url; a.target = '_blank'; a.rel = 'noopener';
    url.append(a);
    foot.append(url);
    div.append(foot);
    return div;
  }

  function renderResults(resp, st) {
    const box = $('#results'); box.innerHTML = '';
    const lines = [];
    resp.results.forEach((r, i) => {
      const v = st.variants[i] || {};
      box.append(renderResult(r, v, st.preview));
      if (!r.error) lines.push(`${r.label} — ${v.width}x${v.height} — ${r.url}`);
    });
    $('#export').value = lines.join('\n');
    $('#bench-title').textContent = `${resp.results.length} variant${resp.results.length > 1 ? 's' : ''} on ${resp.host}`;
    $('#empty').hidden = true;
    $('#results-wrap').hidden = false;
  }

  function showRowErrors(results) {
    const rows = [...list.querySelectorAll('.variant')];
    results.forEach((r, i) => {
      const li = rows[i]; if (!li) return;
      if (r.error) {
        li.classList.add('has-error');
        const e = li.querySelector('.v-error'); e.hidden = false; e.textContent = r.error;
      } else clearError(li);
    });
  }

  async function generate() {
    const st = formState();
    if (!st.source) { setStatus('Enter the source creative URL.', true); $('#source').focus(); return; }
    if (!st.variants.length) { setStatus('Add at least one variant.', true); return; }
    const body = { profile: st.profile, source: st.source, ext: st.ext,
      variants: st.variants.map(v => ({ label: v.label, resizeType: v.resizeType, width: v.width, height: v.height,
        enlarge: v.enlarge, extend: v.extend, quality: v.quality, maxBytes: Math.round(v.maxKB * KB), sharpen: v.sharpen, extra: v.extra })) };
    const btn = $('#generate'); btn.disabled = true; setStatus('Generating…');
    try {
      const res = await fetch('/api/generate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      const data = await res.json();
      if (!res.ok) { setStatus(data.error || `HTTP error ${res.status}`, true); return; }
      const nErr = data.results.filter(r => r.error).length;
      showRowErrors(data.results);
      setStatus(nErr ? `${nErr} variant${nErr > 1 ? 's' : ''} to fix` : '', nErr > 0);
      renderResults(data, st);
      if (window.innerWidth <= 900) $('#results-wrap').scrollIntoView({ behavior: 'smooth' });
    } catch (e) { setStatus(`The server is not responding: ${e.message}`, true); }
    finally { btn.disabled = false; }
  }

  document.querySelectorAll('[data-preset]').forEach(b => b.addEventListener('click', () => {
    const p = PRESETS[Number(b.dataset.preset)]; if (!p) return;
    list.innerHTML = ''; p.variants.forEach(addRow); save();
  }));
  document.querySelectorAll('[data-scale]').forEach(b => b.addEventListener('click', () => {
    document.querySelectorAll('[data-scale]').forEach(x => x.classList.toggle('on', x === b));
    $('#results').classList.toggle('fit', b.dataset.scale === 'fit');
  }));
  $('#add').addEventListener('click', () => { addRow({ resizeType: 'force' }).querySelector('[name=label]').focus(); save(); });
  $('#generate').addEventListener('click', generate);
  $('#copy-all').addEventListener('click', () => copy($('#export').value));
  $('#profile').addEventListener('change', () => { updateHost(); save(); });
  ['#source', '#ext', '#preview'].forEach(s => $(s).addEventListener('input', save));
  $('#source').addEventListener('keydown', e => { if (e.key === 'Enter') generate(); });
  document.addEventListener('keydown', e => { if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') generate(); });

  restore();
  updateHost();
})();
