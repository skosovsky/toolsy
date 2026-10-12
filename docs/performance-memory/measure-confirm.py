from pathlib import Path
import subprocess,os
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
env=dict(os.environ,GOPATH='/tmp/toolsy-performance-gopath',GOCACHE='/tmp/toolsy-performance-gocache',GOWORK='off',GOMAXPROCS='2',GOGC='100',GOMEMLIMIT='off')
for role,path in [('baseline',Path('/tmp/toolsy-memory-baseline')),('candidate',root)]:
 for rep in [3,4]:
  folder=out/f'{role}-memory-{rep}';folder.mkdir(exist_ok=True)
  p=subprocess.run(['go','test','-run=^TestMemoryMeasurement$','-count=1','-timeout=5m','.'],cwd=path,env=dict(env,TOOLSY_MEMORY_OUT=str(folder)),text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT);(folder/'run.log').write_text(p.stdout)
  if p.returncode:raise RuntimeError(folder)
 print(role,'memory confirmation done',flush=True)
