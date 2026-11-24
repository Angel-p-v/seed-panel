'use strict';

const byId = (id) => document.getElementById(id);
const dateFormat = new Intl.DateTimeFormat('es-ES', {
  timeZone: 'Europe/Madrid', day: 'numeric', month: 'short',
  hour: '2-digit', minute: '2-digit',
});
const decimals = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 });
const percent = new Intl.NumberFormat('es-ES', { style: 'percent', maximumFractionDigits: 1 });
const nameOrder = new Intl.Collator('es-ES', { numeric: true, sensitivity: 'base' });
const stateLabels = {
  uploading: 'Compartiendo',
  forcedUP: 'Forzado',
  stalledUP: 'En espera',
  queuedUP: 'En cola',
  checkingUP: 'Verificando',
  error: 'Error',
  missingFiles: 'Archivos ausentes',
  pausedUP: 'Parado',
  pausedDL: 'Parado',
  stoppedUP: 'Parado',
  stoppedDL: 'Parado',
  downloading: 'Descargando',
  forcedDL: 'Descarga forzada',
  stalledDL: 'En espera',
  queuedDL: 'En cola',
  checkingDL: 'Verificando',
  metaDL: 'Obteniendo metadatos',
  forcedMetaDL: 'Obteniendo metadatos',
  allocating: 'Reservando espacio',
};
const emptyLabels = {
  sharing: 'No hay torrents compartiendo.',
  downloading: 'No hay torrents descargando.',
  stopped: 'No hay torrents parados.',
  error: 'No hay torrents con error.',
};
const torrentOrder = {
  'size-desc': (first, second) => second.size - first.size,
  'size-asc': (first, second) => first.size - second.size,
  'ratio-desc': (first, second) => second.ratio - first.ratio,
  name: (first, second) => nameOrder.compare(first.name, second.name),
};

let model;
let period = 'today';
let group = 'sharing';
let selected = null;
let expandedHash = null;

function bytes(value) {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return decimals.format(value) + ' ' + units[index];
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function chartDescription(bar, now) {
  if (bar.start >= now) return bar.label + ' · Pendiente';
  if (!bar.seconds) return bar.label + ' · Sin datos';
  const partial = bar.seconds < Math.min(bar.end, now) - bar.start - 90;
  return bar.label + ' · ' + bytes(bar.bytes) + (partial ? ' · Datos parciales' : '');
}

function renderChart() {
  const bars = model[period];
  const now = Math.floor(Date.now() / 1000);
  const hasData = bars.some((bar) => bar.seconds);
  const total = bars.reduce((sum, bar) => sum + bar.bytes, 0);
  const count = model.snapshot.torrent_count;
  const uploading = model.snapshot.torrents.filter((item) => item.group === 'sharing' && item.upspeed > 0).length;
  byId('chart-total-torrents').textContent = Number.isInteger(count)
    ? decimals.format(count) + (count === 1 ? ' torrent' : ' torrents') + ' · ' + decimals.format(uploading) + ' subiendo'
    : '— torrents';
  byId('total').textContent = hasData ? bytes(total) : '—';
  byId('period-label').textContent = period === 'today' ? 'Hoy, por hora' : 'Desde el lunes, por día';
  const maxValue = Math.max(0, ...bars.map((bar) => bar.bytes));
  const power = Math.pow(2, Math.ceil(Math.log2(maxValue || 1)));
  const scale = byId('scale');
  scale.replaceChildren(...(maxValue ? [power, power / 2, 0] : [0]).map(
    (value) => element('span', '', bytes(value)),
  ));
  const root = byId('bars');
  const focused = root.contains(document.activeElement) ? document.activeElement.dataset.start : null;
  root.replaceChildren(...bars.map((bar, index) => {
    const button = element('button', 'bar');
    button.type = 'button';
    button.dataset.start = bar.start;
    const label = chartDescription(bar, now);
    button.setAttribute('aria-label', label);
    button.setAttribute('aria-pressed', String(bar.start === selected));
    const track = element('span', 'track');
    const fill = element('span', 'fill');
    if (bar.seconds) {
      fill.style.height = Math.max(2, bar.bytes / power * 100) + '%';
      if (label.endsWith('Datos parciales')) button.classList.add('partial');
    } else if (bar.start < now) {
      button.classList.add('gap');
    }
    track.append(fill);
    const tick = period === 'week' ? bar.label : (index % 4 === 0 ? bar.label.slice(0, 2) : '');
    button.append(track, element('span', 'bar-label', tick));
    button.addEventListener('click', () => {
      selected = bar.start;
      renderChart();
    });
    return button;
  }));
  if (focused) {
    [...root.children].find((button) => button.dataset.start === focused)?.focus({ preventScroll: true });
  }
  const selection = bars.find((bar) => bar.start === selected);
  byId('chart-detail').textContent = selection
    ? chartDescription(selection, now)
    : hasData ? 'Toca una barra para ver el detalle.' : 'Sin datos de subida para este periodo.';
}

function renderTorrents() {
  const { snapshot } = model;
  const query = searchText(byId('torrent-search').value.trim());
  const sort = torrentOrder[byId('torrent-sort').value];
  const groups = { sharing: [], downloading: [], stopped: [], error: [] };
  for (const item of snapshot.torrents) {
    if (query && !searchText(item.name).includes(query)) continue;
    const category = item.state === 'missingFiles' || item.state === 'error' ? 'error' : item.group;
    groups[category]?.push(item);
  }
  for (const [category, items] of Object.entries(groups)) {
    byId(category + '-count').textContent = items.length;
  }
  byId('rate').textContent = snapshot.at ? '↑ ' + bytes(snapshot.speed) + '/s' : '—';
  if (sort) groups[group].sort(sort);
  const list = byId('torrent-list');
  const focused = list.contains(document.activeElement) ? document.activeElement.dataset.hash : null;
  const rows = groups[group].map((item) => {
    const row = element('li', 'torrent');
    if (item.upspeed > 0) row.classList.add('active');
    if (group === 'error') row.classList.add('problem');
    const summary = element('button', 'torrent-summary');
    summary.type = 'button';
    summary.dataset.hash = item.hash || '';
    summary.disabled = !item.hash;
    const info = element('span', '');
    const progress = element('span', 'progress', progressText(item.progress));
    progress.title = 'Progreso de descarga';
    const meta = element('span', 'torrent-meta');
    const state = item.upspeed > 0 && item.group === 'sharing' ? 'Subiendo' : (stateLabels[item.state] || item.state);
    const sizeAndProgress = element('span', 'size-progress');
    sizeAndProgress.append(element('span', '', bytes(item.size)), progress);
    meta.append(element('span', 'state', state), sizeAndProgress);
    info.append(element('span', 'torrent-name', item.name), meta);
    const metrics = element('span', 'torrent-metrics');
    metrics.append(
      element('span', 'speed', '↑ ' + bytes(item.upspeed) + '/s'),
      element('span', 'ratio', 'Ratio ' + (item.ratio < 0 ? '—' : item.ratio >= 9999 ? '∞' : decimals.format(item.ratio))),
    );
    summary.append(info, metrics, element('span', 'disclosure'));
    const details = element('dl', 'torrent-detail');
    const wasted = element('dd', '', 'Pendiente');
    details.id = 'detail-' + item.hash;
    details.hidden = !item.hash || expandedHash !== item.hash;
    summary.setAttribute('aria-controls', details.id);
    summary.setAttribute('aria-expanded', String(!details.hidden));
    for (const [label, value] of [
      ['Tiempo compartiendo', element('dd', '', durationText(item.seeding_time))],
      ['Velocidad de subida', element('dd', '', bytes(item.upspeed) + '/s')],
      ['Desperdiciado en la sesión', wasted],
    ]) {
      const metric = element('div', '');
      metric.append(element('dt', '', label), value);
      details.append(metric);
    }
    summary.addEventListener('click', () => {
      expandedHash = expandedHash === item.hash ? null : item.hash;
      renderTorrents();
    });
    if (!details.hidden) refreshWasted(item.hash, wasted);
    row.append(summary, details);
    return row;
  });
  list.replaceChildren(...rows);
  if (focused) {
    [...list.children].find((row) => row.firstChild.dataset.hash === focused)
      ?.firstChild.focus({ preventScroll: true });
  }
  const empty = byId('empty');
  empty.hidden = rows.length > 0;
  if (!snapshot.at) {
    empty.textContent = 'Esperando la primera lectura.';
  } else {
    empty.textContent = query ? 'No hay coincidencias en esta pestaña.' : emptyLabels[group];
  }
}

function durationText(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return '—';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor(seconds / 3600) % 24;
  const minutes = Math.floor(seconds / 60) % 60;
  if (days) return days + ' d ' + hours + ' h';
  if (hours) return hours + ' h ' + minutes + ' min';
  return minutes ? minutes + ' min' : Math.floor(seconds) + ' s';
}

async function refreshWasted(hash, value) {
  try {
    const response = await fetch('/api/torrent-detail?' + new URLSearchParams({ hash }), {
      signal: AbortSignal.timeout(8000),
    });
    if (!response.ok) throw new Error('HTTP ' + response.status);
    const detail = await response.json();
    if (Number.isFinite(detail.total_wasted) && detail.total_wasted >= 0) {
      value.textContent = bytes(detail.total_wasted);
      if (Date.now() / 1000 - detail.at > 150) value.textContent += ' · desactualizado';
      value.title = 'Última lectura: ' + dateFormat.format(detail.at * 1000);
    } else {
      value.textContent = detail.at ? 'Sin datos' : 'En próxima consulta';
    }
  } catch {
    value.textContent = 'Sin datos';
  }
}

function searchText(value) {
  return value.toLocaleLowerCase('es').normalize('NFD').replace(/\p{M}/gu, '');
}

function progressText(value) {
  if (!Number.isFinite(value) || value < 0) return '—';
  return percent.format(value >= 1 ? 1 : Math.min(value, 0.999));
}

function showNotice(message) {
  const notice = byId('notice');
  notice.hidden = !message;
  notice.textContent = message;
}

async function refresh() {
  try {
    const response = await fetch('/api/panel', { signal: AbortSignal.timeout(8000) });
    if (!response.ok) throw new Error('HTTP ' + response.status);
    model = await response.json();
    const updated = byId('updated');
    updated.textContent = model.snapshot.at ? dateFormat.format(model.snapshot.at * 1000) : 'Pendiente';
    updated.title = 'Última lectura válida';
    updated.classList.toggle('stale', model.stale);
    showNotice(model.status || (model.stale && model.snapshot.at ? 'Los datos están desactualizados.' : ''));
    renderChart();
    renderTorrents();
  } catch {
    byId('updated').classList.add('stale');
    showNotice('No se pudo actualizar el panel.');
  } finally {
    setTimeout(refresh, 10_000);
  }
}

for (const [key, onSelect] of [
  ['period', (value) => { period = value; selected = null; if (model) renderChart(); }],
  ['group', (value) => { group = value; if (model) renderTorrents(); }],
]) {
  const buttons = [...document.querySelectorAll('button[data-' + key + ']')];
  for (const button of buttons) {
    button.addEventListener('click', () => {
      buttons.forEach((item) => item.setAttribute('aria-pressed', String(item === button)));
      onSelect(button.dataset[key]);
    });
  }
}

byId('torrent-search').addEventListener('input', () => { if (model) renderTorrents(); });
byId('torrent-sort').addEventListener('change', () => { if (model) renderTorrents(); });
refresh();
