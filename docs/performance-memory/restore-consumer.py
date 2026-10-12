from pathlib import Path
import subprocess,tarfile,io,hashlib,json,shutil,os
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory';repo=root.parent.parent/'mcp/zl-mcp';work=Path('/tmp/zl-memory-20261012');work.mkdir(exist_ok=True)
original=json.loads((out/'pre-pointer-fix/consumer/original-source-manifest.json').read_text())
for n,h in original['candidate_sources'].items():assert hashlib.sha256((repo/n).read_bytes()).hexdigest()==h,n
base=work/'baseline';base.mkdir(exist_ok=True)
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',original['baseline_sha']],cwd=repo))) as t:t.extractall(base,filter='data')
for n,h in original['overlay'].items():
 d=subprocess.check_output(['git','show',original['overlay_sha']+':'+n],cwd=repo);assert hashlib.sha256(d).hexdigest()==h
 p=base/n;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(d)
cand=work/'candidate';cand.mkdir(exist_ok=True)
for n,h in original['candidate_sources'].items():
 p=cand/n;p.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(repo/n,p)
# Same retained historical measurement instrumentation, replacing only in isolated copies.
instrument=(out/'pre-pointer-fix/consumer/instrumentation.go.txt').read_bytes()
for role in ['baseline','candidate']:
 p=work/role/'internal/evaluation/benchmark.go';p.write_bytes(instrument)
 subprocess.run(['gofmt','-w',str(p)],check=True)
 for q in (work/role).rglob('*'):
  if q.is_file():os.utime(q,None)
env=dict(os.environ,GOWORK='off',GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/zl-memory-gocache-20261012',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
record={'restored_from_verified_original_sources':True,'source_manifest':'pre-pointer-fix/consumer/original-source-manifest.json','override':'none for restored old/official roles','builds':[]}
for role in ['baseline','candidate']:
 b=work/('eval-'+role);p=subprocess.run(['go','build','-trimpath','-o',str(b),'./cmd/zl-mcp-eval'],cwd=work/role,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(out/f'restored-{role}-build.log').write_text(p.stdout)
 if p.returncode:raise RuntimeError(p.stdout)
 record['builds'].append({'role':role,'sha256':hashlib.sha256(b.read_bytes()).hexdigest(),'info':subprocess.check_output(['go','version','-m',str(b)],env=env,text=True)})
(out/'restored-consumer-manifest.json').write_text(json.dumps(record,indent=2));print('consumer fixture restored and old/official built')
