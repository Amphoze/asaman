package web

// indexHTML is the single-file observatory UI. No remote assets (offline):
// fonts degrade to system stacks; identity is carried by color, layout, and the
// sky-ribbon signature. {{.CSRF}} is injected server-side.
const indexHTML = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>asaman</title>
<meta name="csrf" content="{{.CSRF}}">
<style>
  :root{
    --ink:#0E1526; --ink2:#16203A; --haze:#8A97B4; --paper:#E9ECF5;
    --brass:#C8A24B; --claude:#B98CFF; --codex:#4FD6C0;
    --disp:"Bricolage Grotesque",Georgia,serif;
    --body:Inter,system-ui,sans-serif; --mono:"JetBrains Mono",ui-monospace,monospace;
  }
  *{box-sizing:border-box} body{margin:0;background:var(--ink);color:var(--paper);font-family:var(--body)}
  header{display:flex;align-items:center;gap:16px;padding:12px 16px;border-bottom:1px solid var(--ink2)}
  .word{font-family:var(--disp);font-weight:700;letter-spacing:.5px;font-size:20px}
  #q{flex:1;background:var(--ink2);border:1px solid #24304f;color:var(--paper);
     border-radius:8px;padding:9px 12px;font-family:var(--mono);font-size:13px}
  .layout{display:flex;min-height:calc(100vh - 56px)}
  aside{width:180px;padding:16px;border-right:1px solid var(--ink2);color:var(--haze);font-size:13px}
  aside h3{font-size:11px;letter-spacing:1px;text-transform:uppercase;color:#5f6d8c;margin:18px 0 8px}
  .dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:8px}
  .claude{background:var(--claude)} .codex{background:var(--codex)}
  main{flex:1;padding:16px 20px}
  .sky{height:56px;position:relative;border-bottom:1px solid var(--ink2);margin-bottom:16px}
  .node{position:absolute;bottom:8px;border-radius:50%;opacity:.85}
  .now{position:absolute;right:0;top:0;bottom:0;width:2px;background:var(--brass)}
  .eyebrow{font-family:var(--mono);font-size:11px;color:var(--haze);text-transform:uppercase;letter-spacing:1px}
  .row{display:flex;gap:12px;align-items:flex-start;padding:10px 8px;border-bottom:1px solid #131c31}
  .row:hover{background:#111a2e}
  .star{cursor:pointer;color:#3d4967} .star.on{color:var(--brass)}
  .meta{font-family:var(--mono);font-size:12px;color:var(--haze);white-space:nowrap}
  .title{font-weight:600} .snip{color:var(--haze);font-size:13px;margin-top:3px}
  a{color:var(--brass);text-decoration:none}
  .empty{color:var(--haze);padding:40px 8px;text-align:center}
  @media(prefers-reduced-motion:no-preference){.now{animation:pulse 2.4s ease-in-out infinite}}
  @keyframes pulse{0%,100%{opacity:.5}50%{opacity:1}}
  @media(max-width:640px){aside{display:none}}
</style></head>
<body>
<header>
  <span class="word">asaman</span>
  <input id="q" placeholder="search sessions + memory…  (⌘K)" autofocus>
</header>
<div class="layout">
  <aside>
    <h3>Agents</h3>
    <div><span class="dot claude"></span>claude</div>
    <div><span class="dot codex"></span>codex</div>
    <h3>Corpora</h3>
    <div>sessions</div><div>memory</div><div>feedback</div>
  </aside>
  <main>
    <div class="sky" id="sky"><div class="now"></div></div>
    <div class="eyebrow" id="head">The Sky · recent</div>
    <div id="results"></div>
  </main>
</div>
<script>
const CSRF = document.querySelector('meta[name=csrf]').content;
const results = document.getElementById('results');
const q = document.getElementById('q');
function esc(t){ const d=document.createElement('div'); d.textContent=t==null?'':t; return d; }
function render(hits){
  results.textContent='';
  if(!hits || !hits.length){ const e=esc('Nothing here yet — search, or star a session to pin it.'); e.className='empty'; results.appendChild(e); return; }
  for(const h of hits){
    const row=document.createElement('div'); row.className='row';
    const star=document.createElement('span'); star.className='star'; star.textContent='★';
    star.onclick=()=>fav(h.ID, star);
    const dot=document.createElement('span'); dot.className='dot '+(h.Agent||'');
    const body=document.createElement('div'); body.style.flex='1';
    const t=esc(h.Title||h.ID); t.className='title';
    const s=esc(h.Snippet||''); s.className='snip';
    const m=document.createElement('span'); m.className='meta'; m.textContent=(h.Agent||'')+' · '+(h.Kind||'');
    body.appendChild(t); body.appendChild(s);
    row.appendChild(star); row.appendChild(dot); row.appendChild(body); row.appendChild(m);
    results.appendChild(row);
  }
}
async function fav(id, el){
  const r=await fetch('/api/meta',{method:'POST',headers:{'X-CSRF-Token':CSRF,'Content-Type':'application/json'},
    body:JSON.stringify({Key:id,Field:'fav',Op:'set',Value:'1'})});
  if(r.ok) el.classList.add('on');
}
let timer;
q.addEventListener('input',()=>{clearTimeout(timer);timer=setTimeout(search,180)});
async function search(){
  const term=q.value.trim(); if(!term){render([]);return;}
  const r=await fetch('/api/search?q='+encodeURIComponent(term));
  render(await r.json());
}
document.addEventListener('keydown',e=>{if((e.metaKey||e.ctrlKey)&&e.key==='k'){e.preventDefault();q.focus();}});
render([]);
</script>
</body></html>`
