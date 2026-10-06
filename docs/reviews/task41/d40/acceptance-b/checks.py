import concurrent.futures, os, pathlib, subprocess
root=pathlib.Path.cwd()
out=pathlib.Path('/tmp/toolsy-task41/d40-acceptance-b')
modules=['toolkits/fstool']+['adapters/sandbox/'+x for x in ['starlark','host','docker','wazero','e2b']]
def check(m):
 name=m.split('/')[-1]
 env=os.environ.copy(); env.update(GOWORK='off',GOCACHE='/tmp/toolsy-review-gocache',GOLANGCI_LINT_CACHE=str(out/'lint'/name))
 for mode,cmd in [('race',['go','test','-race','-count=3','./...']),('lint',['/opt/homebrew/bin/golangci-lint','run','--allow-parallel-runners','./...'])]:
  p=subprocess.run(cmd,cwd=root/m,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
  (out/(name+'-'+mode+'.log')).write_text(p.stdout)
  print(name,mode,p.returncode,flush=True)
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex: list(ex.map(check,modules))
