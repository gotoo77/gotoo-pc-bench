'use strict';
const $=id=>document.getElementById(id);
let runs=[];
const number=v=>new Intl.NumberFormat('fr-FR',{maximumFractionDigits:2}).format(v);
const text=(tag,value,cls='')=>{const el=document.createElement(tag);if(cls)el.className=cls;el.textContent=value;return el;};
function listOptions(select,items,prior){
  select.replaceChildren();
  for(const r of items){const opt=document.createElement('option');opt.value=r.id;opt.textContent=`${r.label} · ${r.cpu||'CPU inconnu'} · ${r.timestamp||'sans date'}`;select.append(opt);}
  if(items.some(r=>r.id===prior))select.value=prior;
}
function card(label,r){
  const container=text('div','', 'box');container.append(text('h3',label));
  for(const [caption,val] of [['CPU',r.cpu],['Cœurs',r.cores?`${r.cores} cœurs / ${r.threads||'?'} threads par cœur`:''],['RAM',r.ram],['Carte mère',r.board],['BIOS',r.bios],['Système',r.os],['Kernel',r.kernel],['Date',r.timestamp]]){
    if(!val)continue;const p=document.createElement('p');p.append(text('small',caption),text('span',val));container.append(p);
  }
  return container;
}
function renderComparison(cmp){
  $('machine').replaceChildren(card('AVANT · '+cmp.before.label,cmp.before),card('APRÈS · '+cmp.after.label,cmp.after));$('machine').hidden=false;
  const warnings=[...(cmp.before.warnings||[]),...(cmp.after.warnings||[]),...(cmp.warnings||[])];
  $('warnings').replaceChildren(...warnings.map(w=>text('p','⚠ '+w)));
  $('warnings').hidden=warnings.length===0;
  $('metrics').replaceChildren();
  for(const row of cmp.rows){
    const el=document.createElement('article');el.className='metric';
    const top=document.createElement('div');top.className='top';top.append(text('h4',row.label));
    const sign=row.improvement_percent>=0?'+':'';
    const change=text('span',row.comparable?`${sign}${number(row.improvement_percent)} %`:'NON COMPARABLE','change'+(!row.comparable?' na':row.improvement_percent<0?' negative':''));
    top.append(change);el.append(top);
    const vals=document.createElement('div');vals.className='nums';vals.append(text('strong',number(row.before)),text('span',' → ','arrow'),text('strong',number(row.after)),text('span',' '+row.unit));el.append(vals);
    if(row.comparable){const bar=document.createElement('div');bar.className='bar';const fill=document.createElement('div');fill.className='fill';fill.style.width=`${Math.min(100,Math.max(2,Math.abs(row.improvement_percent)))}%`;bar.append(fill);el.append(bar);}
    if(row.reason)el.append(text('div',row.reason,'reason'));
    $('metrics').append(el);
  }
  $('results').hidden=cmp.rows.length===0;$('empty').hidden=cmp.rows.length!==0;
  if(cmp.rows.length===0)$('empty').querySelector('p').textContent='Ces deux rapports ne contiennent aucune métrique commune. Choisis deux campagnes compatibles.';
  $('status').textContent=`${cmp.rows.length} mesures communes · ${cmp.rows.filter(r=>r.comparable).length} comparables`;
}
function renderSingle(run){
  $('machine').replaceChildren(card('CAMPAGNE · '+run.label,run));
  $('machine').hidden=false;
  const warnings=run.warnings||[];
  $('warnings').replaceChildren(...warnings.map(w=>text('p','⚠ '+w)));
  $('warnings').hidden=warnings.length===0;
  $('metrics').replaceChildren();
  for(const m of run.metrics){
    const el=document.createElement('article');el.className='metric';
    const top=document.createElement('div');top.className='top';top.append(text('h4',m.label),text('span','RÉFÉRENCE','change na'));el.append(top);
    const vals=document.createElement('div');vals.className='nums';vals.append(text('strong',number(m.value)),text('span',' '+m.unit));el.append(vals);
    $('metrics').append(el);
  }
  $('results').hidden=run.metrics.length===0;
  $('empty').hidden=run.metrics.length!==0;
  $('status').textContent=`${run.metrics.length} mesures de référence · ajoute une campagne après upgrade pour les comparer`;
}
async function compare(){
  if(runs.length===0){$('results').hidden=true;$('machine').hidden=true;$('empty').hidden=false;$('status').textContent='Aucun rapport reconnu dans ce dossier.';return;}
  if(runs.length===1){renderSingle(runs[0]);return;}
  const before=$('before').value,after=$('after').value;
  if(before===after){renderSingle(runs.find(r=>r.id===before)||runs[0]);return;}
  try{const r=await fetch(`/api/compare?before=${encodeURIComponent(before)}&after=${encodeURIComponent(after)}`);if(!r.ok)throw new Error(`HTTP ${r.status}`);renderComparison(await r.json());}
  catch(e){$('status').textContent=`Erreur comparaison : ${e.message}`;$('results').hidden=true;}
}
async function refresh(){
  $('status').textContent='Lecture des rapports…';
  try{const r=await fetch('/api/runs');if(!r.ok)throw new Error(`HTTP ${r.status}`);const prevBefore=$('before').value,prevAfter=$('after').value;runs=await r.json();$('summary').querySelector('strong').textContent=runs.length;
    listOptions($('before'),runs,prevBefore);listOptions($('after'),runs,prevAfter);
    if(!prevBefore&&runs.length>1)$('before').value=runs[runs.length-1].id;
    if(!prevAfter&&runs.length>1)$('after').value=runs[0].id;
    await compare();
  }catch(e){$('status').textContent=`Erreur lecture : ${e.message}`;}
}
$('refresh').addEventListener('click',refresh);$('before').addEventListener('change',compare);$('after').addEventListener('change',compare);refresh();
