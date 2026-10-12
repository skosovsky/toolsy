from pathlib import Path
import shutil,subprocess,os,hashlib,json,re,statistics
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory';stage=Path('/tmp/toolsy-memory-stage1')
shutil.copytree('/tmp/toolsy-memory-baseline',stage,dirs_exist_ok=True)
# Reproduce first bounded-table design, before the inline refinement, from final source.
requirements=(root/'requirements.go').read_text()
requirements=re.sub(r'\n\s*inline\s+\[8\]cloneMemoEntry','',requirements)
requirements=re.sub(r'\n\s*inlineCount\s+int','',requirements)
(stage/'requirements.go').write_text(requirements)
memo=(root/'clone_memo.go').read_text();a=memo.index('\tfor _, entry := range c.inline',memo.index('func (c *valueCloner) lookup'));b=memo.index('\tslot :=',a)
memo=memo[:a]+'''\tif c.memo == nil {
        select { case c.memo = <-idleCloneMemos: default: c.memo = &cloneMemo{} }
    }
'''+memo[b:]
a=memo.index('\tif c.memo == nil {',memo.index('func (c *valueCloner) remember'));b=memo.index('\tmemo := c.memo',a);memo=memo[:a]+memo[b:]
memo=memo.replace('\tclear(c.inline[:c.inlineCount])\n\tc.inlineCount = 0\n','')
(stage/'clone_memo.go').write_text(memo)
subprocess.run(['gofmt','-w',str(stage/'requirements.go'),str(stage/'clone_memo.go')],check=True)
(out/'marginal-stage1-source.json').write_text(json.dumps({n:{'sha256':hashlib.sha256((stage/n).read_bytes()).hexdigest(),'source':(stage/n).read_text()} for n in ['requirements.go','clone_memo.go']},indent=2))
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
rows={}
for rep in range(1,6):
 for role,path in [('pooled',stage),('final',root)]:
  p=subprocess.run(['go','test','-run=^$','-bench=Benchmark(PolicyManifestRich|PerformanceClone)$','-benchmem','-benchtime=300ms','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/f'{role}-marginal-{rep}.log').write_text(p.stdout)
  if p.returncode:raise RuntimeError(p.stdout)
  for l in p.stdout.splitlines():
   m=re.match(r'(Benchmark\S+)-\d+\s+\d+\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',l)
   if m:rows.setdefault(m[1],{}).setdefault(role,[]).append(list(map(float,m.groups()[1:])))
summary={n:{r:{k:{'median':statistics.median(v[i] for v in values),'min':min(v[i] for v in values),'max':max(v[i] for v in values)} for i,k in enumerate(['ns','bytes','allocs'])} for r,values in roles.items()} for n,roles in rows.items()}
(out/'marginal-summary.json').write_text(json.dumps(summary,indent=2));print('marginal done',flush=True)
