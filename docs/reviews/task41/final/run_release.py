from pathlib import Path
import subprocess,os,json,hashlib,time
root=Path.cwd();d=Path('/tmp/toolsy-task41/final/release');d.mkdir(parents=True,exist_ok=True);source=d/'source';remote=d/'remote.git'
env=os.environ.copy();env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache',GIT_OPTIONAL_LOCKS='0',GOLANGCI_LINT_CACHE='/tmp/toolsy-task41/final/release-lint-cache')
def run(args,cwd=root):
 return subprocess.check_output(args,cwd=cwd,env=env,text=True,stderr=subprocess.STDOUT).strip()
assert not source.exists() and not remote.exists()
run(['git','clone','--no-local','--no-hardlinks','--quiet',str(root),str(source)])
run(['git','init','--bare',str(remote)])
for k,v in [('user.name','Release verification fixture'),('user.email','fixture@example.invalid'),('commit.gpgsign','false')]:run(['git','config',k,v],source)
run(['git','remote','set-url','origin',str(remote)],source)
run(['git','config','--unset-all','remote.origin.pushurl'],source) if subprocess.run(['git','config','--get-all','remote.origin.pushurl'],cwd=source,env=env,stdout=subprocess.DEVNULL).returncode==0 else None
(source/'private-note.txt').write_text('FAKE_FINAL_UNTRACKED_MARKER\n');(source/'.git/info/exclude').open('a').write('\nprivate-cache/\n');(source/'private-cache').mkdir();(source/'private-cache/marker.txt').write_text('FAKE_IGNORED_MARKER\n')
def snapshot():
 files={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for p in source.rglob('*') if p.is_file() and '.git' not in p.relative_to(source).parts}
 r=subprocess.run(['git','--git-dir',str(remote),'show-ref'],env=env,text=True,stdout=subprocess.PIPE)
 return {'head':run(['git','rev-parse','HEAD'],source),'branch':run(['git','branch','--show-current'],source),'refs':run(['git','show-ref'],source),'index':hashlib.sha256((source/'.git/index').read_bytes()).hexdigest(),'files':files,'remote_refs':r.stdout}
before=snapshot();d.joinpath('before.json').write_text(json.dumps(before,indent=2)+'\n')
with (d/'build.log').open('w') as log:subprocess.run(['go','build','-mod=readonly','-o',str(d/'toolsy-release'),'./cmd/toolsy-release'],cwd=source,env=env,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=600)
start=time.time()
with (d/'prepare-only.log').open('w') as log:p=subprocess.run([str(d/'toolsy-release'),'-prepare-only','break'],cwd=source,env=env,stdin=subprocess.DEVNULL,stdout=log,stderr=subprocess.STDOUT,timeout=2400)
after=snapshot();d.joinpath('after.json').write_text(json.dumps(after,indent=2)+'\n');log=(d/'prepare-only.log').read_text();verified=[line for line in log.splitlines() if line.startswith('Verify ')]
receipt={'candidate':before['head'],'exit':p.returncode,'seconds':round(time.time()-start,3),'verified_modules':verified,'module_count':len(verified),'source_unchanged':before==after,'remote_unchanged':before['remote_refs']==after['remote_refs'],'mode':'native CLI -prepare-only break; FullChecks=true','publication':'none; only local bare remote configured'}
d.joinpath('receipt.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt),flush=True)
assert p.returncode==0 and before==after and len(verified)==24 and not after['remote_refs'],receipt
