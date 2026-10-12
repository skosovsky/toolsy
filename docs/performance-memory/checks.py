from pathlib import Path
import subprocess,os,json,time
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOLANGCI_LINT_CACHE='/tmp/toolsy-performance-lintcache')
results={}
for target in ['lint','test','test-integration','test-e2e']:
 started=time.time()
 with open(out/f'make-{target}.log','w') as log:p=subprocess.run(['make',target],cwd=root,env=env,stdout=log,stderr=subprocess.STDOUT)
 results[target]={'status':'PASS' if p.returncode==0 else 'FAIL','exit':p.returncode,'started_unix':started,'ended_unix':time.time(),'command':['make',target]}
 (out/'check-results.json').write_text(json.dumps(results,indent=2));print(target,results[target]['status'],flush=True)
results['live']={'status':'NOT RUN','reason':'External credential/provider checks are separate; no live-provider validation claim.'}
(out/'check-results.json').write_text(json.dumps(results,indent=2))
