from pathlib import Path
import os,subprocess,json,re,statistics
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
series={}
for fixture,pattern,duration in [('env','BenchmarkPerformanceEnv/populated$','1s'),('cache','BenchmarkPerformanceProfiles/cache-miss$','10000x')]:
 for rep in range(1,6):
  for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
   p=subprocess.run(['go','test','-run=^$','-bench='+pattern,'-benchmem','-benchtime='+duration,'-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
   (out/f'{role}-{fixture}-confirmation-{rep}.log').write_text(p.stdout)
   if p.returncode:raise RuntimeError(p.stdout)
   for line in p.stdout.splitlines():
    m=re.match(r'(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',line)
    if m:series.setdefault(fixture,{}).setdefault(role,[]).append(list(map(float,m.groups()[1:])))
summary={}
for fixture,roles in series.items():
 summary[fixture]={role:{k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])} for role,values in roles.items()}
 summary[fixture]['ratios']={k:summary[fixture]['candidate'][k]['median']/summary[fixture]['baseline'][k]['median'] for k in ['ns','bytes','allocs']}
(out/'confirmation-summary.json').write_text(json.dumps(summary,indent=2));print(json.dumps(summary,indent=2))
