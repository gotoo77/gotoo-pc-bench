'use strict';
const $ = id => document.getElementById(id);
const model = globalThis.PCBenchModel;
let runs = [];
let requestID = 0;
const number = v => new Intl.NumberFormat('fr-FR', {maximumFractionDigits: 2}).format(v);
function element(tag, content, className = '') {
  const node = document.createElement(tag);
  if (className) node.className = className;
  node.textContent = content;
  return node;
}
function setOptions(select, items, selected) {
  select.replaceChildren();
  if (select.id === 'after') {
    const placeholder = element('option', 'Aucune : afficher uniquement la référence');
    placeholder.value = '';
    select.append(placeholder);
  }
  for (const run of items) {
    const option = element('option', model.campaignText(run));
    option.value = run.id;
    option.disabled = select.id === 'after' && run.id === $('before').value;
    select.append(option);
  }
  select.value = selected;
}
function updateAfterOptions(selected) {
  const after = $('after');
  const valid = selected && selected !== $('before').value && runs.some(run => run.id === selected);
  setOptions(after, runs, valid ? selected : '');
  after.disabled = runs.length < 2;
}
function machineCard(title, run) {
  const container = element('div', '', 'box');
  container.append(element('h3', title));
  const cores = Number(run.cores), threadsPerCore = Number(run.threads);
  const topology = Number.isInteger(cores) && cores > 0 && Number.isInteger(threadsPerCore) && threadsPerCore > 0
    ? `${cores} cœur${cores > 1 ? 's' : ''} / ${cores * threadsPerCore} threads logiques` : '';
  for (const [name, value] of [
    ['Processeur', run.cpu], ['Topologie', topology],
    ['RAM visible dans le rapport', run.ram], ['Carte mère', run.board], ['BIOS', run.bios],
    ['Système', run.os], ['Noyau', run.kernel], ['Date', run.timestamp], ['Rapport', run.id],
  ]) {
    if (!value) continue;
    const p = document.createElement('p');
    p.append(element('small', name), element('span', value));
    container.append(p);
  }
  return container;
}
function showWarnings(warnings) {
  $('warnings').replaceChildren(...warnings.map(w => element('p', '⚠ ' + w)));
  $('warnings').hidden = warnings.length === 0;
}
function metricValues(before, after, unit) {
  const values = element('div', '', 'nums');
  values.append(element('span', 'Avant : ', 'value-caption'), element('strong', number(before)),
    element('span', ' → ', 'arrow'), element('span', 'Après : ', 'value-caption'),
    element('strong', number(after)), element('span', ' ' + unit));
  return values;
}
function metricChart(row) {
  const data = model.performanceBars(row);
  if (!data) return null;
  const chart = element('div', '', 'comparison-chart');
  chart.setAttribute('role', 'img');
  chart.setAttribute('aria-label', `Indice de performance relatif : avant 100, après ${number(data.afterIndex)}. Valeurs normalisées et non valeurs brutes.`);
  for (const [label, value, width, cls] of [
    ['Avant', 100, data.beforeWidth, 'baseline'],
    ['Après', data.afterIndex, data.afterWidth, 'current'],
  ]) {
    const rowEl = element('div', '', 'chart-row');
    const track = element('div', '', 'chart-track');
    const bar = element('div', '', `chart-fill ${cls}${row.improvement_percent < 0 && cls === 'current' ? ' regression' : ''}`);
    bar.style.width = `${width}%`;
    track.append(bar);
    rowEl.append(element('span', label, 'chart-label'), track, element('span', number(value), 'chart-value'));
    chart.append(rowEl);
  }
  return chart;
}
function comparisonMetric(row) {
  const el = element('article', '', 'metric');
  const top = element('div', '', 'top');
  top.append(element('h4', model.metricLabel(row.label)));
  const change = !row.comparable ? element('span', 'NON COMPARABLE', 'change na')
    : element('span', `${row.improvement_percent > 0 ? '+' : ''}${number(row.improvement_percent)} %`,
      `change${row.improvement_percent < 0 ? ' negative' : row.improvement_percent === 0 ? ' neutral' : ''}`);
  top.append(change);
  el.append(top, metricValues(row.before, row.after, row.unit));
  const chart = metricChart(row);
  if (chart) el.append(chart);
  if (row.reason) el.append(element('p', row.reason, 'reason'));
  return el;
}
function referenceMetric(metric) {
  const el = element('article', '', 'metric');
  const top = element('div', '', 'top');
  top.append(element('h4', model.metricLabel(metric.label)), element('span', 'RÉFÉRENCE', 'change na'));
  const nums = element('div', '', 'nums');
  nums.append(element('strong', number(metric.value)), element('span', ' ' + metric.unit));
  el.append(top, nums);
  return el;
}
function showMetrics(metrics, render) {
  const result = $('metrics');
  result.replaceChildren();
  for (const group of model.grouped(metrics)) {
    const section = element('section', '', 'metric-group');
    const head = element('div', '', 'group-head');
    head.append(element('h4', group.label), element('span', `${group.rows.length} mesure${group.rows.length > 1 ? 's' : ''} · ${group.description}`));
    const grid = element('div', '', 'group-grid');
    for (const item of group.rows) grid.append(render(item));
    section.append(head, grid);
    result.append(section);
  }
  $('results').hidden = metrics.length === 0;
  $('empty').hidden = metrics.length !== 0;
}
function showSingle(run) {
  $('machine').replaceChildren(machineCard('RÉFÉRENCE · ' + run.label, run));
  $('machine').hidden = false;
  showWarnings(run.warnings || []);
  $('comparison-note').textContent = 'Vue d’une seule campagne. Sélectionne une mesure distincte pour afficher les gains.';
  $('chart-legend').hidden = true;
  $('empty').querySelector('p').textContent = 'Aucune mesure reconnue dans ce rapport.';
  showMetrics(run.metrics || [], referenceMetric);
  $('status').textContent = `${(run.metrics || []).length} mesures de référence · aucune comparaison active`;
}
function showComparison(cmp) {
  $('machine').replaceChildren(machineCard('AVANT · ' + cmp.before.label, cmp.before), machineCard('APRÈS · ' + cmp.after.label, cmp.after));
  $('machine').hidden = false;
  const warnings = [...(cmp.before.warnings || []), ...(cmp.after.warnings || []), ...(cmp.warnings || [])];
  if (cmp.before.label === cmp.after.label) warnings.push('Ces rapports portent la même étiquette : vérifie qu’il s’agit bien d’un avant et d’un après distincts.');
  showWarnings(warnings);
  $('comparison-note').textContent = 'Deux campagnes différentes. Les gains indiquent une amélioration de performance : positif = mieux.';
  $('chart-legend').hidden = false;
  $('empty').querySelector('p').textContent = 'Aucune mesure commune dans ces rapports.';
  showMetrics(cmp.rows || [], comparisonMetric);
  $('status').textContent = `${cmp.rows.length} mesures communes · ${cmp.rows.filter(r => r.comparable).length} comparables`;
}
async function compare() {
  const currentRequest = ++requestID;
  const b = $('before').value, a = $('after').value;
  if (runs.length === 0) {
    $('machine').hidden = true; $('warnings').hidden = true; $('results').hidden = true;
    $('chart-legend').hidden = true; $('empty').hidden = false;
    $('empty').querySelector('p').textContent = 'Place des rapports .txt ou .json dans le dossier choisi puis actualise.';
    $('status').textContent = 'Aucun rapport reconnu.';
    return;
  }
  if (!a || a === b) { showSingle(runs.find(r => r.id === b) || runs[0]); return; }
  $('status').textContent = 'Calcul de la comparaison…';
  try {
    const url = `/api/compare?before=${encodeURIComponent(b)}&after=${encodeURIComponent(a)}`;
    const response = await fetch(url);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const data = await response.json();
    if (currentRequest === requestID) showComparison(data);
  } catch (err) {
    if (currentRequest === requestID) {
      $('status').textContent = `Erreur de comparaison : ${err.message}`;
      $('results').hidden = true;
      $('chart-legend').hidden = true;
    }
  }
}
async function refresh() {
  $('status').textContent = 'Lecture des rapports…';
  try {
    const response = await fetch('/api/runs');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const previous = {before: $('before').value, after: $('after').value};
    const data = await response.json();
    if (!Array.isArray(data)) throw new Error('Format de la liste invalide');
    runs = data;
    $('summary').querySelector('strong').textContent = runs.length;
    const selected = model.initialSelection(runs, previous.before, previous.after);
    setOptions($('before'), runs, selected.before);
    $('before').disabled = runs.length === 0;
    updateAfterOptions(selected.after);
    await compare();
  } catch (err) {
    $('status').textContent = `Erreur de lecture : ${err.message}`;
  }
}
$('refresh').addEventListener('click', refresh);
$('before').addEventListener('change', () => { updateAfterOptions($('after').value); compare(); });
$('after').addEventListener('change', compare);
refresh();
