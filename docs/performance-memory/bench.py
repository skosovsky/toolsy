from pathlib import Path
import subprocess,os,json,re,statistics
root=Path(__file__).resolve().parents[2]; out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
pattern='Benchmark(PolicyManifestRich|ManifestPreDispatch|Performance(Clone|Execute|Wrappers|Profiles|Env|Stream|Cold|FreshSchema|Decode))$'
for rep in range(1,6):
 for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
  target=out/f'{role}-bench-{rep}.log'
  if target.exists():continue
  p=subprocess.run(['go','test','-run=^$','-bench='+pattern,'-benchmem','-benchtime=300ms','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  target.write_text(p.stdout)
  if p.returncode:raise RuntimeError(target)
  print(role,rep,flush=True)
for rep in range(1,6):
 for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline/mcp')),('candidate',root/'mcp')]:
  target=out/f'{role}-mcp-bench-{rep}.log'
  if target.exists():continue
  p=subprocess.run(['go','test','-run=^$','-bench=BenchmarkPerformanceExport$','-benchmem','-benchtime=300ms','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  target.write_text(p.stdout)
  if p.returncode:raise RuntimeError(target)
  print(role,'mcp',rep,flush=True)
rows={}
for role in ['baseline','candidate']:
 for rep in range(1,6):
  for line in ((out/f'{role}-bench-{rep}.log').read_text()+(out/f'{role}-mcp-bench-{rep}.log').read_text()).splitlines():
   m=re.match(r'(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
   if m:rows.setdefault(m[1],{}).setdefault(role,[]).append(list(map(float,m.groups()[1:])))
summary={}
for name,roles in rows.items():
 summary[name]={r:{k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])} for r,values in roles.items()}
 summary[name]['ratios']={k:summary[name]['candidate'][k]['median']/summary[name]['baseline'][k]['median'] for k in ['ns','bytes','allocs']}
(out/'bench-summary.json').write_text(json.dumps(summary,indent=2))
