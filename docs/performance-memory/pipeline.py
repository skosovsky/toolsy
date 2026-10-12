from pathlib import Path
import subprocess,time,json
root=Path(__file__).resolve().parents[2];out=root/'docs/performance-memory'
deadline=time.monotonic()+1200
while not (out/'bench-summary.json').exists():
 log=(out/'bench-progress.log').read_text() if (out/'bench-progress.log').exists() else ''
 if 'Traceback' in log or time.monotonic()>deadline:raise RuntimeError('benchmark matrix incomplete')
 time.sleep(5)
records=[]
for script in ['marginal.py','measure.py','measure-confirm.py','boundary-confirm.py','profile.py','consumer.py','checks.py','summarize.py']:
 print(script,'started',flush=True);start=time.time()
 with open(out/(script.removesuffix('.py')+'-progress.log'),'w') as log:
  p=subprocess.run(['python3',str(out/script)],cwd=root,stdout=log,stderr=subprocess.STDOUT)
 records.append({'script':script,'exit':p.returncode,'started_unix':start,'ended_unix':time.time()})
 (out/'pipeline-results.json').write_text(json.dumps(records,indent=2))
 print(script,'PASS' if p.returncode==0 else 'FAIL',flush=True)
 if p.returncode:raise RuntimeError(script)
