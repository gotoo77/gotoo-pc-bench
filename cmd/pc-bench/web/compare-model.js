'use strict';
// Pure presentation helpers. Performance index is always 100 for the reference.
// The API already computes improvement_percent in the correct direction for each metric.
(function (root) {
  const sections = [
    {key: 'cpu', label: 'CPU', description: 'Calcul synthétique Sysbench'},
    {key: 'ram', label: 'Mémoire', description: 'Débit d’écriture Sysbench'},
    {key: 'ffmpeg', label: 'FFmpeg', description: 'Encodage logiciel H.264'},
    {key: 'go', label: 'Go', description: 'Calcul SHA-256 parallèle'},
  ];
  function category(key) {
    const group = String(key || '').split('.')[0];
    return sections.find(s => s.key === group)?.key || 'other';
  }
  function grouped(metrics) {
    const groups = new Map();
    for (const metric of metrics || []) {
      const key = category(metric.key);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(metric);
    }
    return [...sections, {key: 'other', label: 'Autres', description: ''}]
      .filter(s => groups.has(s.key)).map(s => ({...s, rows: groups.get(s.key)}));
  }
  function campaignText(run) {
    const name = String(run.id || 'fichier inconnu');
    const date = run.timestamp ? String(run.timestamp).replace('T', ' ').replace(/([+-]\d\d:\d\d|Z)$/, '') : 'sans date';
    const cpu = String(run.cpu || 'CPU inconnu');
    const chip = cpu.match(/\b(?:i[3579]-\d+[A-Z]*|Ryzen\s+\d+\s+\w+|Apple\s+M\d+\w*)\b/i);
    return `${date} · ${run.label || 'sans nom'} · ${chip ? chip[0] : cpu} · ${(run.metrics || []).length} mesures · ${name}`;
  }
  function initialSelection(runs, before, after) {
    const ids = new Set(runs.map(r => r.id));
    const preferred = runs.find(r => /^(avant|before|pre)[-_ ]?upgrade\b/i.test(r.label || '')) || runs[0];
    const b = ids.has(before) ? before : (preferred?.id || '');
    const a = ids.has(after) && after !== b ? after : '';
    return {before: b, after: a};
  }
  function metricLabel(label) {
    return String(label || '').replace(/\b1 threads\b/g, '1 thread').replace(/\b1 workers\b/g, '1 worker');
  }
  function performanceBars(row) {
    if (!row || !row.comparable || !Number.isFinite(row.improvement_percent) || row.improvement_percent <= -100) return null;
    const baseline = 100;
    const after = baseline + row.improvement_percent;
    const scale = Math.max(baseline, after);
    return {beforeIndex: baseline, afterIndex: after,
      beforeWidth: baseline / scale * 100, afterWidth: after / scale * 100};
  }
  const result = {sections, category, grouped, campaignText, initialSelection, metricLabel, performanceBars};
  root.PCBenchModel = result;
  if (typeof module !== 'undefined' && module.exports) module.exports = result;
})(typeof globalThis === 'object' ? globalThis : this);
