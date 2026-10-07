from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed
import json, os, subprocess, time
root=Path.cwd();out=Path('/tmp/toolsy-task41/final/modules');out.mkdir(parents=True,exist_ok=True)
files=subprocess.check_output(['git','ls-files'],text=True).splitlines()
mods=sorted(str(Path(n).parent) for n in files if Path(n).name=='go.mod')
assert len(mods)==24,mods
work=[n.strip().removeprefix('./') for n in root.joinpath('go.work').read_text().splitlines() if n.startswith('\t')]
assert set(mods)==set(work),(mods,work)
env=os.environ.copy();env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache')
meta={'candidate':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'go':subprocess.check_output(['go','version'],text=True).strip(),'lint':subprocess.check_output(['/opt/homebrew/bin/golangci-lint','version'],text=True).strip(),'modules':mods,'worker_count':4}
out.joinpath('inventory.json').write_text(json.dumps(meta,indent=2)+'\n')
def check(module):
 d=out/('root' if module=='.' else module.replace('/','__'));d.mkdir(exist_ok=True)
 results=[]
 for label,cmd in [('tests',['go','test','-count=1','./...']),('race',['go','test','-race','-count=1','./...']),('lint',['/opt/homebrew/bin/golangci-lint','run','--allow-parallel-runners','./...'])]:
  taskenv=env.copy();taskenv['GOLANGCI_LINT_CACHE']=str(Path('/tmp/toolsy-task41/final/lint-cache')/d.name)
  start=time.time()
  with d.joinpath(label+'.log').open('w') as log:
   p=subprocess.run(cmd,cwd=root/module,env=taskenv,stdout=log,stderr=subprocess.STDOUT,timeout=1200)
  results.append({'stage':label,'command':cmd,'exit':p.returncode,'seconds':round(time.time()-start,3)})
  print(module,label,p.returncode,flush=True)
 d.joinpath('result.json').write_text(json.dumps({'module':module,'results':results},indent=2)+'\n')
 return {'module':module,'results':results}
allresults=[]
with ThreadPoolExecutor(max_workers=4) as pool:
 for future in as_completed([pool.submit(check,m) for m in mods]):
  allresults.append(future.result());out.joinpath('results.json').write_text(json.dumps(sorted(allresults,key=lambda r:r['module']),indent=2)+'\n')
assert len(allresults)==24
failed=[r for r in allresults if any(c['exit'] for c in r['results'])]
print('COMPLETE',len(allresults),'modules; failed',len(failed),flush=True)
raise SystemExit(bool(failed))
