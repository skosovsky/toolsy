from pathlib import Path
import subprocess, tempfile, os, time, signal, json
base=Path(tempfile.mkdtemp(prefix='r05-wrapper-a-',dir='/tmp'))
repo=base/'repo'; (repo/'scripts').mkdir(parents=True)
(repo/'scripts/release.sh').write_bytes(Path('scripts/release.sh').read_bytes())
for args in [('init','-b','main'),('config','commit.gpgsign','false'),('config','user.name','Review fixture'),('config','user.email','fixture@example.invalid'),('add','.'),('commit','-m','fixture')]:
 subprocess.run(['git',*args],cwd=repo,check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
binpath=base/'bin';binpath.mkdir()
fake=binpath/'go';fake.write_text('''#!/bin/bash
trap '' TERM
printf '%s\\n' "$$" > "$PROBE_PARENT"
bash -c 'trap "" TERM; printf "%s\\n" "$$" > "$PROBE_CHILD"; while :; do sleep 1; done' &
wait
''');fake.chmod(0o755)
env=os.environ.copy();env.update(PATH=str(binpath)+':'+env['PATH'],PROBE_PARENT=str(base/'parent.pid'),PROBE_CHILD=str(base/'child.pid'),TMPDIR=str(base))
p=subprocess.Popen(['bash',str(repo/'scripts/release.sh'),'patch'],cwd=repo,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
for _ in range(100):
 if (base/'child.pid').exists(): break
 time.sleep(.05)
else: raise RuntimeError('fake go not reached')
pids=[int((base/n).read_text()) for n in ['parent.pid','child.pid']]
t=time.monotonic(); p.send_signal(signal.SIGTERM); out,err=p.communicate(timeout=10)
active=[]
for pid in pids:
 try: os.kill(pid,0)
 except ProcessLookupError: pass
 else: active.append(pid)

print(json.dumps({'exit':p.returncode,'elapsed':round(time.monotonic()-t,3),'pids':pids,'active':active,'remaining_runner_dirs':[str(x) for x in base.glob('toolsy-release-runner.*')],'stderr':err.decode()}))
if active or list(base.glob('toolsy-release-runner.*')): raise RuntimeError('owned resource remains')
