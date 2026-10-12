from pathlib import Path
import subprocess,os,json
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
 log=out/f'{role}-boundary.log'
 chunks=[]
 for repeat in range(5):
  p=subprocess.run(['go','test','-run=^$','-bench=BenchmarkMemoryRegistryBoundary$','-benchmem','-benchtime=300ms','-count=1','.'],cwd=path,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);chunks.append(p.stdout);log.write_text('\n'.join(chunks))
  if p.returncode:raise RuntimeError(log)
 for rep in range(1,3):
  folder=out/f'{role}-memory-{rep}';folder.mkdir(exist_ok=True)
  p=subprocess.run(['go','test','-run=^TestMemoryMeasurement$','-count=1','-timeout=5m','.'],cwd=path,env=dict(env,TOOLSY_MEMORY_OUT=str(folder)),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(folder/'run.log').write_text(p.stdout)
  if p.returncode:raise RuntimeError(folder)
  p=subprocess.run(['go','tool','pprof','-top','-inuse_space',str(folder/'live-registry-inuse.pprof')],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(folder/'inuse-top.txt').write_text(p.stdout)
 print(role,'retained done',flush=True)
