from pathlib import Path
import os,subprocess,re,json,statistics
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
result={}
for rep in range(1,6):
 for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
  p=subprocess.run(['go','test','-run=^$','-bench=BenchmarkMemoryRegistryBoundary$','-benchmem','-benchtime=1s','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(out/f'{role}-boundary-confirm-{rep}.log').write_text(p.stdout)
  if p.returncode:raise RuntimeError(p.stdout)
  for line in p.stdout.splitlines():
   m=re.match(r'Benchmark\S+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
   if m:result.setdefault(role,[]).append(list(map(float,m.groups())))
summary={role:{k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])} for role,values in result.items()}
(out/'boundary-confirm-summary.json').write_text(json.dumps(summary,indent=2));print('boundary confirmed',flush=True)
# Confirm the custom binder timing outlier from the repaired-candidate matrix.
result={}
for rep in range(1,6):
 for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
  p=subprocess.run(['go','test','-run=^$','-bench=BenchmarkPerformanceWrappers/binder$','-benchmem','-benchtime=1s','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(out/f'{role}-binder-confirm-{rep}.log').write_text(p.stdout)
  if p.returncode:raise RuntimeError(p.stdout)
  for line in p.stdout.splitlines():
   m=re.match(r'Benchmark\S+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
   if m:result.setdefault(role,[]).append(list(map(float,m.groups())))
summary={role:{k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])} for role,values in result.items()}
(out/'binder-confirm-summary.json').write_text(json.dumps(summary,indent=2));print('binder confirmed',flush=True)
