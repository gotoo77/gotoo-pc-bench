'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const {campaignText, category, grouped, initialSelection, metricLabel, performanceBars} = require('../cmd/pc-bench/web/compare-model.js');
const run = (id,label='avant_upgrade') => ({id,label, cpu:'Intel(R) Core(TM) i3-4130 CPU @ 3.40GHz',timestamp:'2026-11-15T14:23:31+01:00',metrics:[{key:'go.1'}]});
test('two before_upgrade reports never auto-create a before/after comparison', () => {
  const a=run('rapport_avant_upgrade_a.json');
  const b=run('rapport_avant_upgrade_b.txt');
  assert.deepEqual(initialSelection([a,b],'',''),{before:a.id,after:''});
  assert.deepEqual(initialSelection([a,b],a.id,a.id),{before:a.id,after:''});
  assert.deepEqual(initialSelection([a,b],a.id,b.id),{before:a.id,after:b.id});
});
test('selection survives refresh only for reports that still exist', () => {
  const a=run('rapport_a.txt');const b=run('rapport_b.txt');
  assert.deepEqual(initialSelection([a,b], 'missing.txt', b.id), {before:a.id,after:b.id});
  assert.deepEqual(initialSelection([a], a.id, b.id), {before:a.id,after:''});
  assert.deepEqual(initialSelection([], a.id, b.id), {before:'',after:''});
});
test('campaign labels distinguish identically named campaigns by file and date', () => {
  assert.match(campaignText(run('rapport_avant_upgrade_20261115_142331.json')),/2026-11-15 14:23:31/);
  assert.match(campaignText(run('rapport_avant_upgrade_20261115_142331.json')),/i3-4130/);
  assert.match(campaignText(run('rapport_avant_upgrade_20261115_142331.json')),/142331\.json/);
});
test('all core metrics are grouped, preserving order within a group', () => {
  const rows=[{key:'go.8'},{key:'cpu.1'},{key:'cpu.4'},{key:'ram.1'},{key:'ffmpeg.4'}];
  assert.deepEqual(grouped(rows).map(g=>[g.key,g.rows.map(x=>x.key)]),[
    ['cpu',['cpu.1','cpu.4']],['ram',['ram.1']],['ffmpeg',['ffmpeg.4']],['go',['go.8']]
  ]);
  assert.equal(category('unexpected.metric'),'other');
});
test('performance index uses server semantic gains, including inverse time metrics', () => {
  const twiceAsFast=performanceBars({comparable:true,improvement_percent:100});
  assert.deepEqual(twiceAsFast,{beforeIndex:100,afterIndex:200,beforeWidth:50,afterWidth:100});
  const halfSpeed=performanceBars({comparable:true,improvement_percent:-50});
  assert.deepEqual(halfSpeed,{beforeIndex:100,afterIndex:50,beforeWidth:100,afterWidth:50});
  assert.deepEqual(performanceBars({comparable:true,improvement_percent:0}), {beforeIndex:100,afterIndex:100,beforeWidth:100,afterWidth:100});
});
test('invalid or non-comparable metrics must never get a chart', () => {
  assert.equal(performanceBars({comparable:false,improvement_percent:250}),null);
  assert.equal(performanceBars({comparable:true,improvement_percent:NaN}),null);
  assert.equal(performanceBars({comparable:true,improvement_percent:-100}),null);
});

test('reference defaults to latest before-upgrade campaign and uses singular units', () => {
  const after=run('rapport_apres_upgrade.json','apres_upgrade');
  const before=run('rapport_avant_upgrade.json','avant_upgrade');
  assert.deepEqual(initialSelection([after,before], '', ''), {before:before.id,after:''});
  assert.equal(metricLabel('CPU · 1 threads'), 'CPU · 1 thread');
  assert.equal(metricLabel('Go SHA-256 · 1 workers'), 'Go SHA-256 · 1 worker');
});
