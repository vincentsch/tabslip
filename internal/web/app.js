(() => {
 'use strict';
 const root=document.documentElement;
 try {const saved=localStorage.getItem('tabslip-theme');if(saved)root.dataset.theme=saved;}catch{}
 const page=document.body.dataset.page;
 const content=document.querySelector('#content-state');
 function contentStyle(){content.classList.toggle('stored',content.textContent!=='Content: not stored');}
 contentStyle();
 document.querySelector('#theme').addEventListener('click',()=>{
  const dark=root.dataset.theme ? root.dataset.theme==='dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  root.dataset.theme=dark?'light':'dark';try{localStorage.setItem('tabslip-theme',root.dataset.theme);}catch{}
 });

 document.addEventListener('click',async e=>{
  const button=e.target.closest('[data-copy]');if(!button){const row=e.target.closest('#call-rows tr');if(row&&!e.target.closest('a,button,input')&&!getSelection().toString())row.querySelector('a').click();return;}
  try{await navigator.clipboard.writeText(document.getElementById(button.dataset.copy).textContent);const original=button.textContent;button.textContent='Copied';setTimeout(()=>button.textContent=original,1500);}catch{button.textContent='Copy failed. Select the text to copy.';}
 });
 document.addEventListener('submit',e=>{if(e.target.dataset.confirm&&!confirm(e.target.dataset.confirm))e.preventDefault();});
 let capacity=200, newCalls=0;
 const rows=()=>Array.from(document.querySelectorAll('#call-rows tr'));
 const slow=new URLSearchParams(location.search).get('order')==='slowest';
 function compare(a,b){let c=slow?Number(b.dataset.total)-Number(a.dataset.total):b.dataset.started.localeCompare(a.dataset.started);return c||b.dataset.id.localeCompare(a.dataset.id);}
 function updateMore(){const more=document.querySelector('#load-more');const last=rows().at(-1);if(!more||!last)return;const u=new URL(location.href);u.searchParams.set('cursor_id',last.dataset.id);u.searchParams.set('cursor',slow?last.dataset.total:last.dataset.started);more.href=u.pathname+u.search;}
 let pending=false,again=false;
 async function refresh(){
  if(pending){again=true;return;}pending=true;
  try{
   const u=new URL(location.href);u.searchParams.set('fragment','1');
   const res=await fetch(u);if(!res.ok)throw new Error('refresh');const doc=new DOMParser().parseFromString(await res.text(),'text/html');
   if(page==='spend'){document.querySelector('#spend-results').replaceChildren(...doc.querySelector('#spend-results').childNodes);}
   else if(page==='calls'){document.querySelector('#call-results').replaceChildren(...doc.querySelector('#call-results').childNodes);capacity=200;}
   else if(page==='receipt'){const open=Array.from(document.querySelectorAll('details')).map(d=>d.open);const section=doc.querySelector('.receipt-layout');document.querySelector('.receipt-layout').replaceWith(section);section.querySelectorAll('details').forEach((d,i)=>{if(i<open.length)d.open=open[i];});}
  }catch{document.querySelector('#announcements').textContent='Live refresh failed. Reconnecting…';}
  finally{pending=false;if(again){again=false;refresh();}}
 }
 document.addEventListener('click',async e=>{
  const more=e.target.closest('#load-more');if(!more||e.ctrlKey||e.metaKey)return;e.preventDefault();if(more.dataset.loading)return;more.dataset.loading='1';more.textContent='Loading…';
  try{const u=new URL(more.href);u.searchParams.set('fragment','1');const res=await fetch(u);if(!res.ok)throw new Error('load');const doc=new DOMParser().parseFromString(await res.text(),'text/html');let added=0;doc.querySelectorAll('#call-rows tr').forEach(row=>{if(!rows().some(r=>r.dataset.id===row.dataset.id)){document.querySelector('#call-rows').append(row);added++;}});capacity+=added;const next=doc.querySelector('#load-more');if(next){more.href=next.href;more.textContent='Load more';}else{more.remove();}}
  catch{more.textContent='Retry loading more';}finally{delete more.dataset.loading;}
 });
 const events=new EventSource('/events'+location.search);
 events.onopen=()=>{const status=document.querySelector('#connection');if(status)status.textContent='connected';if(page!=='setup')refresh();};
 events.onerror=()=>{const status=document.querySelector('#connection');if(status)status.textContent='reconnecting';};
 events.addEventListener('counter',e=>{const n=JSON.parse(e.data),warning=document.querySelector('#recording-warning');warning.hidden=n===0;warning.textContent=`${n} ${n===1?'call could':'calls could'} not be recorded. API responses still passed through.`;});
 events.addEventListener('change',e=>{
  const data=JSON.parse(e.data);content.textContent=data.content;contentStyle();
  if(page==='spend'){refresh();return;}
  if(page==='receipt'){if(data.kind==='refresh'||location.pathname==='/r/'+data.id)refresh();return;}
  if(page!=='calls')return;
  if(pending){again=true;return;}
  if(data.kind==='refresh'){refresh();return;}
  const body=document.querySelector('#call-rows');const old=rows().find(row=>row.dataset.id===data.id);if(old)old.remove();
  if(data.row){const temp=document.createElement('tbody');temp.innerHTML=data.row;const row=temp.firstElementChild;
   const remaining=rows(),before=remaining.find(other=>compare(row,other)<0);if(before)body.insertBefore(row,before);else body.append(row);
   row.classList.add('fresh');setTimeout(()=>row.classList.remove('fresh'),1200);
   if(rows().length>capacity&&!document.querySelector('#load-more')){const more=document.createElement('a');more.id='load-more';more.className='button';more.textContent='Load more';more.href=location.href;document.querySelector('#call-results').append(more);}
   while(rows().length>capacity)body.lastElementChild.remove();
   document.querySelector('.empty')?.remove();
   if(!old&&data.kind==='receipt'){newCalls++;document.querySelector('#announcements').textContent=`${newCalls} new ${newCalls===1?'call':'calls'}`;}
  }
  updateMore();
  if(old&&!data.row&&rows().length===0)refresh();
 });
 let selected=-1;
 document.addEventListener('keydown',e=>{
  if(e.ctrlKey||e.altKey||e.metaKey||e.isComposing)return;
  const editing=e.target.matches('input,textarea,select,[contenteditable]');
  if(e.key==='Escape'&&page==='calls'){e.preventDefault();location.href='/';return;}
  if(editing)return;
  if(e.key==='/'&&document.querySelector('#search')){e.preventDefault();document.querySelector('#search').focus();}
  if(page==='calls'&&(e.key==='j'||e.key==='k')){e.preventDefault();const all=rows();if(!all.length)return;selected=Math.min(all.length-1,Math.max(0,selected+(e.key==='j'?1:-1)));all.forEach((row,i)=>row.classList.toggle('selected',i===selected));all[selected].querySelector('a').focus({preventScroll:true});all[selected].scrollIntoView({block:'nearest'});}
  if(e.key==='Enter'&&selected>=0&&page==='calls'&&e.target.matches('body,tr'))rows()[selected]?.querySelector('a').click();
 });
 window.addEventListener('pagehide',()=>events.close());
})();
