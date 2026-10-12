from pathlib import Path
import subprocess,os
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
 folder=out/f'{role}-exact-profile';folder.mkdir(exist_ok=True)
 p=subprocess.run(['go','test','-run=^TestMemoryMeasurement$','-count=1','-memprofilerate=1','-timeout=10m','.'],cwd=path,env=dict(env,TOOLSY_MEMORY_OUT=str(folder)),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(folder/'run.log').write_text(p.stdout)
 if p.returncode:raise RuntimeError(folder)
 for name,extra in [('inuse-top',[]),('constructor',['-focus=NewProxyTool|compileResultContract']),('scratch',['-focus=valueCloner.*remember'])]:
  p=subprocess.run(['go','tool','pprof','-top','-nodecount=50','-inuse_space',*extra,str(folder/'live-registry-inuse.pprof')],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(folder/(name+'.txt')).write_text(p.stdout)
 print(role,'exact profile done',flush=True)
