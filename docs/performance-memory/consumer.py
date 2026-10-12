from pathlib import Path
import subprocess,os,json,hashlib,shutil,time
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory/consumer';out.mkdir(exist_ok=True)
work=Path('/tmp/toolsy-memory-consumer');work.mkdir(exist_ok=True)
original=Path('/tmp/zl-memory-20261012');evidence=root.parent/'ai-libs/reviews/zl-memory-2026-10-12'
if not (original/'candidate/go.mod').exists() or not (original/'eval-baseline').exists():
 subprocess.run(['python3',str(root/'docs/performance-memory/restore-consumer.py')],cwd=root,check=True)
env=dict(os.environ,GOWORK='off',GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/zl-memory-gocache-20261012',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
source=work/'toolsy';source.mkdir(exist_ok=True)
for p in root.rglob('*'):
 rel=p.relative_to(root)
 if any(s.startswith('.') for s in rel.parts) or rel.parts[0]=='docs':continue
 if p.is_file():q=source/rel;q.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,q)
app=work/'candidate';shutil.copytree(original/'candidate',app,dirs_exist_ok=True)
subprocess.run(['go','mod','edit','-replace=github.com/skosovsky/toolsy='+str(source)],cwd=app,env=env,check=True)
binary=work/'eval-optimized';p=subprocess.run(['go','build','-trimpath','-o',str(binary),'./cmd/zl-mcp-eval'],cwd=app,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(out/'build.log').write_text(p.stdout)
if p.returncode:raise RuntimeError('build failed')
manifest={'override':str(source),'candidate_sources':{str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in source.rglob('*') if p.is_file()},'fixture_sha256':hashlib.sha256((original/'candidate/docs/evals/fixtures.json').read_bytes()).hexdigest(),'env':{k:env[k] for k in ['GOWORK','GOMAXPROCS','GOGC','GOMEMLIMIT']},'binaries':{},'runs':[]}
roles={'old':original/'eval-baseline','official':original/'eval-candidate','optimized':binary}
for r,b in roles.items():manifest['binaries'][r]={'sha256':hashlib.sha256(b.read_bytes()).hexdigest(),'info':subprocess.check_output(['go','version','-m',str(b)],text=True,env=env)}
for role,rep in [('old',1),('official',1),('optimized',1),('optimized',2),('official',2),('old',2)]:
 jobs=[]
 for kind in ['read','search','context']:
  label=f'{role}-{kind}-r{rep}';state=work/label;state.mkdir(exist_ok=True)
  cmd=[str(roles[role]),'-root',str(original/'candidate'),'-measure-kind',kind,'-measure-calls','600','-state-dir',str(state)]
  stdout=open(out/(label+'.json'),'w');stderr=open(out/(label+'.stderr'),'w');p=subprocess.Popen(cmd,env=env,stdout=stdout,stderr=stderr)
  jobs.append((p,stdout,stderr,{'role':role,'replica':rep,'kind':kind,'command':cmd,'started_unix':time.time()}))
 for p,a,b,record in jobs:
  record['exit_status']=p.wait(timeout=150);record['ended_unix']=time.time();a.close();b.close();manifest['runs'].append(record)
 (out/'manifest.json').write_text(json.dumps(manifest,indent=2));print(role,rep,flush=True)
 if any(r['exit_status'] for r in manifest['runs']):raise RuntimeError('diagnostic failed')
